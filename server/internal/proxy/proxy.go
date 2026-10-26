// Package proxy holds the reverse-proxy trust policy shared by the HTTP API
// and the WebSocket endpoint.
package proxy

import (
	"net/http"
	"os"
	"strings"
)

// IsTrusted checks if the remote address matches the TRUSTED_PROXY environment variable.
func IsTrusted(remoteAddr string) bool {
	trustedProxy := os.Getenv("TRUSTED_PROXY")
	if trustedProxy == "" {
		return false
	}
	// Strip port from remoteAddr if present
	host := remoteAddr
	if idx := strings.LastIndex(remoteAddr, ":"); idx != -1 {
		host = remoteAddr[:idx]
	}
	return host == trustedProxy
}

// Scheme returns the scheme the client used to reach the server. The
// X-Forwarded-Proto header is only honored when the request comes from
// TRUSTED_PROXY.
func Scheme(r *http.Request) string {
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" && IsTrusted(r.RemoteAddr) {
		if strings.ToLower(proto) == "https" {
			return "https"
		}
		return "http"
	}
	if r.TLS != nil {
		return "https"
	}
	return "http"
}
