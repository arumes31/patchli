// Package control configures the agent's authenticated control-plane transport.
package control

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

const maxResponseBytes = 4 << 20

// Client builds HTTP and WebSocket requests to one verified HTTPS origin.
type Client struct {
	base       *url.URL
	httpClient *http.Client
	dialer     *websocket.Dialer
}

// FromEnvironment loads SERVER_URL and an optional operator-mounted SERVER_CA_FILE.
func FromEnvironment(getenv func(string) string) (*Client, error) {
	raw := strings.TrimSpace(getenv("SERVER_URL"))
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return nil, errors.New("SERVER_URL must be an https origin")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return nil, errors.New("SERVER_URL must not contain credentials, a path, query, or fragment")
	}
	parsed.Path = ""

	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	if caFile := strings.TrimSpace(getenv("SERVER_CA_FILE")); caFile != "" {
		// #nosec G304 -- SERVER_CA_FILE is an operator-controlled startup setting, not remote input.
		pem, readErr := os.ReadFile(caFile)
		if readErr != nil {
			return nil, fmt.Errorf("read SERVER_CA_FILE: %w", readErr)
		}
		pool, poolErr := x509.SystemCertPool()
		if poolErr != nil {
			pool = x509.NewCertPool()
		}
		if !pool.AppendCertsFromPEM(pem) {
			return nil, errors.New("SERVER_CA_FILE contains no certificates")
		}
		tlsConfig.RootCAs = pool
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = tlsConfig.Clone()
	return &Client{
		base: parsed,
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   60 * time.Second,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		dialer: &websocket.Dialer{
			HandshakeTimeout: 15 * time.Second,
			TLSClientConfig:  tlsConfig.Clone(),
		},
	}, nil
}

func (c *Client) endpoint(path string) (*url.URL, error) {
	if !strings.HasPrefix(path, "/") || strings.Contains(path, "..") {
		return nil, errors.New("control endpoint must be an absolute clean path")
	}
	endpoint := *c.base
	endpoint.Path = path
	return &endpoint, nil
}

// NewRequest creates a request to the configured HTTPS origin.
func (c *Client) NewRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	endpoint, err := c.endpoint(path)
	if err != nil {
		return nil, err
	}
	return http.NewRequestWithContext(ctx, method, endpoint.String(), body)
}

// Do sends an HTTPS request without following redirects.
func (c *Client) Do(request *http.Request) (*http.Response, error) {
	if request == nil || request.URL == nil || request.URL.Scheme != "https" || request.URL.Host != c.base.Host || request.URL.User != nil {
		return nil, errors.New("control request escaped configured HTTPS origin")
	}
	// #nosec G704 -- the request is constrained above to the startup-validated HTTPS control origin.
	return c.httpClient.Do(request)
}

// LimitedBody caps responses before decoding or writing them to disk.
func LimitedBody(body io.Reader) io.Reader {
	return io.LimitReader(body, maxResponseBytes+1)
}

// DialWebSocket opens a verified WSS connection to the configured origin.
func (c *Client) DialWebSocket(ctx context.Context, path string, header http.Header) (*websocket.Conn, *http.Response, error) {
	endpoint, err := c.endpoint(path)
	if err != nil {
		return nil, nil, err
	}
	endpoint.Scheme = "wss"
	return c.dialer.DialContext(ctx, endpoint.String(), header)
}
