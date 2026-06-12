package webhooks

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIsValidURL(t *testing.T) {
	tests := []struct {
		url      string
		expected bool
	}{
		{"http://localhost", true},
		{"https://google.com", true},
		{"ftp://unsafe", false},
		{"invalid", false},
		{"http://127.0.0.1:8080", true},
	}

	for _, tt := range tests {
		if got := isValidURL(tt.url); got != tt.expected {
			t.Errorf("isValidURL(%s) = %v, want %v", tt.url, got, tt.expected)
		}
	}
}

func TestNotifyWebhooks_Empty(t *testing.T) {
	// Should not panic
	NotifyWebhooks(WebhookPayload{Event: "test"})
}

func TestWebhooks_SpecificPlatforms(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	t.Run("NotifySlack", func(t *testing.T) {
		NotifySlack(server.URL, "test message")
	})

	t.Run("NotifyDiscord", func(t *testing.T) {
		NotifyDiscord(server.URL, "test message")
	})

	t.Run("NotifyTeams", func(t *testing.T) {
		NotifyTeams(server.URL, "title", "text")
	})
}

func TestSendWebhook_Error(t *testing.T) {
	// Test with closed server to trigger delivery failure
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := server.URL
	server.Close()

	sendWebhook(url, WebhookPayload{Event: "fail"})
}
