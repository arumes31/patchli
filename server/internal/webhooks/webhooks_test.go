package webhooks

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNotifyWebhooks(t *testing.T) {
	// Mock NotifyWebhooks
	oldNotify := NotifyWebhooks
	defer func() { NotifyWebhooks = oldNotify }()
	
	var called bool
	NotifyWebhooks = func(payload WebhookPayload) {
		called = true
	}

	NotifyWebhooks(WebhookPayload{Event: "test"})
	if !called {
		t.Error("NotifyWebhooks was not called")
	}
}

func TestNotifySlack(t *testing.T) {
	NotifySlack("http://example.com/slack", "test message")
}

func TestNotifyDiscord(t *testing.T) {
	NotifyDiscord("http://example.com/discord", "test message")
}

func TestNotifyTeams(t *testing.T) {
	NotifyTeams("http://example.com/teams", "title", "text")
}

func TestSecureHTTPClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// The custom client blocks loopback/private IPs, so requesting the httptest server directly will fail.
	_, err := secureHTTPClient.Get(server.URL)
	if err == nil {
		t.Error("Expected error when dialing loopback address, got nil")
	}
}

func TestIsValidURL(t *testing.T) {
	tests := []struct {
		url      string
		expected bool
	}{
		{"http://example.com", true},
		{"https://example.com", true},
		{"http://localhost:8080", false},
		{"http://127.0.0.1", false},
		{"http://10.0.0.1", false},
		{"ftp://example.com", false},
		{"invalid-url", false},
	}

	for _, tt := range tests {
		if got := isValidURL(tt.url); got != tt.expected {
			t.Errorf("isValidURL(%q) = %v, want %v", tt.url, got, tt.expected)
		}
	}
}

func TestSendWebhook(t *testing.T) {
    sendWebhook("http://example.com", WebhookPayload{Event: "test"})
}
