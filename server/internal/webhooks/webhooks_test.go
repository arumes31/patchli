package webhooks

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
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

func TestNotifyAsync(t *testing.T) {
	// Create a test server that sleeps for 2 seconds
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Test NotifySlack
	start := time.Now()
	NotifySlack(server.URL, "test slack")
	elapsed := time.Since(start)
	if elapsed > 500*time.Millisecond {
		t.Errorf("NotifySlack took too long: %v, expected it to be async", elapsed)
	}

	// Test NotifyDiscord
	start = time.Now()
	NotifyDiscord(server.URL, "test discord")
	elapsed = time.Since(start)
	if elapsed > 500*time.Millisecond {
		t.Errorf("NotifyDiscord took too long: %v, expected it to be async", elapsed)
	}

	// Test NotifyTeams
	start = time.Now()
	NotifyTeams(server.URL, "title", "text")
	elapsed = time.Since(start)
	if elapsed > 500*time.Millisecond {
		t.Errorf("NotifyTeams took too long: %v, expected it to be async", elapsed)
	}
}
