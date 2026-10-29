package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// allowLoopbackHTTP opts into plain HTTP loopback controllers for tests that
// use httptest.NewServer; that requires a command verification key.
func allowLoopbackHTTP(t *testing.T) {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	oldAllow, oldKey := AllowInsecureLoopback, CommandPubKey
	t.Cleanup(func() { AllowInsecureLoopback, CommandPubKey = oldAllow, oldKey })
	AllowInsecureLoopback = true
	CommandPubKey = base64.StdEncoding.EncodeToString(pub)
}

func TestControllerEndpointPlainHTTPRequiresOptInAndSigning(t *testing.T) {
	oldAllow, oldKey := AllowInsecureLoopback, CommandPubKey
	t.Cleanup(func() { AllowInsecureLoopback, CommandPubKey = oldAllow, oldKey })
	for _, tc := range []struct {
		name  string
		allow bool
		key   string
		want  string
	}{
		{"default", false, "key", "ALLOW_INSECURE_LOOPBACK"},
		{"unsigned commands", true, "", "COMMAND_PUB_KEY"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			AllowInsecureLoopback, CommandPubKey = tc.allow, tc.key
			_, err := controllerEndpoint("http://127.0.0.1:8080", "/ws")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("plain HTTP accepted or wrong reason: %v", err)
			}
			// HTTPS is never affected by the loopback opt-in.
			if _, err := controllerEndpoint("https://127.0.0.1:8080", "/ws"); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestControllerEndpointPolicy(t *testing.T) {
	allowLoopbackHTTP(t)
	for _, tt := range []struct {
		input, path, want string
		fail              bool
	}{
		{"example.com:8443", "/api/v1/poll", "https://example.com:8443/api/v1/poll", false},
		{"https://example.com/patchli/", "/ws", "https://example.com/patchli/ws", false},
		{"https://example.com/a%2Fb/", "/ws", "https://example.com/a%2Fb/ws", false},
		{"localhost:8080", "/health", "https://localhost:8080/health", false},
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
	allowLoopbackHTTP(t)
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
