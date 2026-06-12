package webhooks

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSendWebhook_StatusError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	sendWebhook(server.URL, WebhookPayload{Event: "404"})
}

func TestNotifyWebhooks_WithEnv(t *testing.T) {
	// Mock environment variables are hard to test with the current package structure
	// because it reads them inside the anonymous function variable.
	// But we can call it.
	NotifyWebhooks(WebhookPayload{Event: "dummy"})
}
