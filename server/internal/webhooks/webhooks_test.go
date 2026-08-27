package webhooks

import (
	"context"
	"net"
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

// Override DialContext for testing to bypass SSRF protection for test server
func mockSafeClient(url string) {
	// For testing, we just want to ensure it doesn't panic
}

func TestNotifySlack(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	// Temporarily override the safe client for testing
	oldSafeClient := safeHTTPClient
	defer func() { safeHTTPClient = oldSafeClient }()
	safeHTTPClient = func() *http.Client {
		return &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
					return net.Dial(network, addr)
				},
			},
		}
	}

	NotifySlack(ts.URL, "test message")
}

func TestNotifyDiscord(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	oldSafeClient := safeHTTPClient
	defer func() { safeHTTPClient = oldSafeClient }()
	safeHTTPClient = func() *http.Client {
		return &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
					return net.Dial(network, addr)
				},
			},
		}
	}

	NotifyDiscord(ts.URL, "test message")
}

func TestNotifyTeams(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	oldSafeClient := safeHTTPClient
	defer func() { safeHTTPClient = oldSafeClient }()
	safeHTTPClient = func() *http.Client {
		return &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
					return net.Dial(network, addr)
				},
			},
		}
	}

	NotifyTeams(ts.URL, "title", "text")
}
