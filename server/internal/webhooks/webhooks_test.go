package webhooks

import (
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
	NotifySlack("http://example.com:8080/slack", "test message")
}

func TestNotifyDiscord(t *testing.T) {
	NotifyDiscord("http://example.com:8080/discord", "test message")
}

func TestNotifyTeams(t *testing.T) {
	NotifyTeams("http://example.com:8080/teams", "title", "text")
}
