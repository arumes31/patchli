package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestControllerEndpointPolicy(t *testing.T) {
	for _, tt := range []struct {
		input, path, want string
		fail              bool
	}{
		{"example.com:8443", "/api/v1/poll", "https://example.com:8443/api/v1/poll", false},
		{"https://example.com/patchli/", "/ws", "https://example.com/patchli/ws", false},
		{"https://example.com/a%2Fb/", "/ws", "https://example.com/a%2Fb/ws", false},
		{"localhost:8080", "/health", "http://localhost:8080/health", false},
		{"http://127.0.0.1:8080/base", "/health", "http://127.0.0.1:8080/base/health", false},
		{"http://[::1]:8080", "/health", "http://[::1]:8080/health", false},
		{"http://example.com", "/health", "", true},
		{"http://192.168.1.10", "/health", "", true},
		{"https://user:password@example.com", "/health", "", true},
		{"https://example.com?token=secret", "/health", "", true},
		{"https://example.com/#fragment", "/health", "", true},
		{"file:///etc/passwd", "/health", "", true},
		{"https://", "/health", "", true},
	} {
		t.Run(tt.input, func(t *testing.T) {
			got, err := controllerEndpoint(tt.input, tt.path)
			if tt.fail {
				if err == nil {
					t.Fatal("unsafe endpoint accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.String() != tt.want {
				t.Fatalf("got %q, want %q", got.String(), tt.want)
			}
		})
	}
}

func TestRefreshDoesNotFollowRedirect(t *testing.T) {
	var received bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { received = true }))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	_, err := refreshTokens(context.Background(), redirect.URL, "node", "refresh-secret")
	if err == nil {
		t.Fatal("redirect accepted")
	}
	if received {
		t.Fatal("refresh credential forwarded to redirect destination")
	}
}
