package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// controllerEndpoint uses only the operator's SERVER_URL. Remote controllers
// must authenticate with a verified TLS certificate; HTTP is for local agents.
func controllerEndpoint(raw, endpoint string) (*url.URL, error) {
	if raw == "" {
		raw = "localhost:8080"
	}
	explicitScheme := strings.Contains(raw, "://")
	if !explicitScheme {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid SERVER_URL")
	}
	if u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || (u.Scheme != "https" && u.Scheme != "http") {
		return nil, fmt.Errorf("SERVER_URL must be an HTTP(S) origin with an optional base path, without credentials, query or fragment")
	}
	loopback := strings.EqualFold(u.Hostname(), "localhost")
	if ip := net.ParseIP(u.Hostname()); ip != nil {
		loopback = ip.IsLoopback()
	}
	if !explicitScheme && loopback {
		u.Scheme = "http"
	}
	if u.Scheme == "http" && !loopback {
		return nil, fmt.Errorf("SERVER_URL requires HTTPS for non-loopback controllers")
	}
	u.RawPath = strings.TrimRight(u.EscapedPath(), "/") + endpoint
	u.Path, err = url.PathUnescape(u.RawPath)
	if err != nil {
		return nil, fmt.Errorf("invalid SERVER_URL path")
	}
	return u, nil
}

func controllerRequest(ctx context.Context, method, raw, endpoint string, body io.Reader) (*http.Request, error) {
	u, err := controllerEndpoint(raw, endpoint)
	if err != nil {
		return nil, err
	}
	// #nosec G704 -- The URL is operator configuration, validated above for TLS. Private controllers are supported intentionally.
	return http.NewRequestWithContext(ctx, method, u.String(), body)
}

func controllerClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		// Never forward credentials or accept commands from a redirect target.
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}
}
