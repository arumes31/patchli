package webhooks

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type deliveryTransport func(*http.Request) (*http.Response, error)

func (f deliveryTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type deliveryBody struct {
	io.Reader
	closed chan struct{}
}

func (b deliveryBody) Close() error { close(b.closed); return nil }

func TestProviderWebhookPayloads(t *testing.T) {
	cases := []struct {
		name     string
		send     func(string)
		expected map[string]string
	}{
		{"slack", func(u string) { NotifySlack(u, "hello") }, map[string]string{"text": "hello"}},
		{"discord", func(u string) { NotifyDiscord(u, "hello") }, map[string]string{"content": "hello"}},
		{"teams", func(u string) { NotifyTeams(u, "title", "hello") }, map[string]string{"@type": "MessageCard", "@context": "http://schema.org/extensions", "themeColor": "0076D7", "summary": "title", "text": "hello"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			previous := webhookClient
			t.Cleanup(func() { webhookClient = previous })
			closed := make(chan struct{})
			received := make(chan map[string]string, 1)
			webhookClient = &http.Client{Transport: deliveryTransport(func(r *http.Request) (*http.Response, error) {
				payload := map[string]string{}
				err := json.NewDecoder(r.Body).Decode(&payload)
				if err != nil {
					return nil, err
				}
				payload["method"] = r.Method
				payload["content-type"] = r.Header.Get("Content-Type")
				received <- payload
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: deliveryBody{strings.NewReader(""), closed}}, nil
			})}
			tc.send("https://hooks.example.invalid/hook")
			select {
			case <-closed:
			case <-time.After(time.Second):
				t.Fatal("delivery did not finish")
			}
			got := <-received
			if got["method"] != "POST" || got["content-type"] != "application/json" {
				t.Fatalf("incorrect request: %+v", got)
			}
			for key, value := range tc.expected {
				if got[key] != value {
					t.Errorf("%s=%q want %q", key, got[key], value)
				}
			}
		})
	}
}

func TestWebhookFailuresAndRejectedURLs(t *testing.T) {
	previous := webhookClient
	t.Cleanup(func() { webhookClient = previous })
	calls := make(chan struct{}, 10)
	webhookClient = &http.Client{Transport: deliveryTransport(func(*http.Request) (*http.Response, error) {
		calls <- struct{}{}
		return nil, errors.New("delivery unavailable")
	})}
	for _, send := range []func(string){func(u string) { sendWebhook(u, WebhookPayload{Event: "test"}) }, func(u string) { NotifySlack(u, "test") }, func(u string) { NotifyDiscord(u, "test") }, func(u string) { NotifyTeams(u, "title", "test") }} {
		send("file:///etc/passwd")
		select {
		case <-calls:
			t.Fatal("invalid URL reached transport")
		default:
		}
		send("https://hooks.example.invalid/hook")
		select {
		case <-calls:
		case <-time.After(time.Second):
			t.Fatal("valid delivery never attempted")
		}
	}
}

func TestGenericWebhookAndEnvironmentSelection(t *testing.T) {
	previous := webhookClient
	t.Cleanup(func() { webhookClient = previous })
	t.Setenv("SLACK_WEBHOOK_URL", "https://hooks.example.invalid/hook")
	t.Setenv("TEAMS_WEBHOOK_URL", "file:///bad")
	t.Setenv("DISCORD_WEBHOOK_URL", "")
	closed := make(chan struct{})
	received := make(chan WebhookPayload, 1)
	webhookClient = &http.Client{Transport: deliveryTransport(func(r *http.Request) (*http.Response, error) {
		var payload WebhookPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			return nil, err
		}
		received <- payload
		return &http.Response{StatusCode: 503, Header: make(http.Header), Body: deliveryBody{strings.NewReader("unavailable"), closed}}, nil
	})}
	expected := WebhookPayload{Event: "job_failed", JobID: "job", NodeMac: "node", Message: "failure"}
	NotifyWebhooks(expected)
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("notification did not finish")
	}
	if got := <-received; got != expected {
		t.Fatalf("payload=%+v want %+v", got, expected)
	}
}

func TestURLValidationRejectsLocalAndUnsupportedDestinations(t *testing.T) {
	for _, u := range []string{"%", "ftp://example.com", "http://localhost", "http://127.0.0.1:8080", "http://10.0.0.1", "http://169.254.169.254"} {
		if isValidURL(u) {
			t.Errorf("accepted %q", u)
		}
	}
	for _, u := range []string{"https://example.com", "https://example.com:8443/hook"} {
		if !isValidURL(u) {
			t.Errorf("rejected %q", u)
		}
	}
}
