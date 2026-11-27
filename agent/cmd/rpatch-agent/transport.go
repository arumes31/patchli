package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// AllowInsecureLoopback permits plain HTTP to a loopback controller for local
// development (ALLOW_INSECURE_LOOPBACK=true). Plain HTTP cannot prove the
// listener is the controller, so any local process bound to that port would
// receive the agent's credentials.
var AllowInsecureLoopback = os.Getenv("ALLOW_INSECURE_LOOPBACK") == "true"

// controllerEndpoint uses only the operator's SERVER_URL. The controller must
// authenticate with a verified TLS certificate before the agent sends
// credentials or accepts commands. Plain HTTP is accepted only for an
// explicitly allowed loopback development controller, and only together with
// COMMAND_PUB_KEY so that commands are still authenticated.
func controllerEndpoint(raw, endpoint string) (*url.URL, error) {
	if raw == "" {
		raw = "localhost:8080"
	}
	if !strings.Contains(raw, "://") {
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
	if u.Scheme == "http" {
		switch {
		case !loopback:
			return nil, fmt.Errorf("SERVER_URL requires HTTPS for non-loopback controllers")
		case !AllowInsecureLoopback:
			return nil, fmt.Errorf("SERVER_URL uses plain HTTP, which cannot authenticate the controller; use HTTPS or set ALLOW_INSECURE_LOOPBACK=true for local development")
		case CommandPubKey == "":
			return nil, fmt.Errorf("plain HTTP controllers require COMMAND_PUB_KEY so that commands are authenticated")
		}
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
