package webhooks

import (
	"bytes"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
)

type WebhookPayload struct {
	Event   string `json:"event"`
	Message string `json:"message"`
	JobID   string `json:"job_id,omitempty"`
	NodeMac string `json:"node_mac,omitempty"`
}

var NotifyWebhooks = func(payload WebhookPayload) {
	urls := []string{
		os.Getenv("SLACK_WEBHOOK_URL"),
		os.Getenv("TEAMS_WEBHOOK_URL"),
		os.Getenv("DISCORD_WEBHOOK_URL"),
	}

	for _, u := range urls {
		if u == "" {
			continue
		}
		if !isValidURL(u) {
			log.Print("Invalid or insecure webhook URL")
			continue
		}
		go sendWebhook(u, payload)
	}
}

func sendWebhook(targetURL string, payload WebhookPayload) {
	if !isValidURL(targetURL) {
		return
	}
	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Failed to marshal webhook payload: %v", err)
		return
	}

	req, err := http.NewRequest("POST", targetURL, bytes.NewBuffer(data)) // #nosec G704 -- webhookClient validates resolved IPs when dialing and rejects redirects.
	if err != nil {
		log.Printf("Failed to create webhook request: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := webhookClient.Do(req) // #nosec G704 -- Transport checks every resolved IP and dials it directly; redirects are disabled.
	if err != nil {
		log.Printf("Webhook delivery failed: %v", err)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		log.Printf("Webhook returned error status: %d", resp.StatusCode) // #nosec G706 -- StatusCode is an integer formatted with %d.
	}
}

func NotifySlack(webhookURL string, msg string) {
	if !isValidURL(webhookURL) {
		return
	}
	payload := map[string]string{"text": msg}
	go func() {
		data, err := json.Marshal(payload)
		if err != nil {
			log.Printf("Slack webhook error: %v", err)
			return
		}
		resp, err := webhookClient.Post(webhookURL, "application/json", bytes.NewBuffer(data))
		if err != nil {
			log.Printf("Slack webhook error: %v", err)
			return
		}
		if resp != nil {
			defer func() { _ = resp.Body.Close() }()
		}
	}()
}

func NotifyDiscord(webhookURL string, msg string) {
	if !isValidURL(webhookURL) {
		return
	}
	payload := map[string]string{"content": msg}
	go func() {
		data, err := json.Marshal(payload)
		if err != nil {
			log.Printf("Discord webhook error: %v", err)
			return
		}
		resp, err := webhookClient.Post(webhookURL, "application/json", bytes.NewBuffer(data))
		if err != nil {
			log.Printf("Discord webhook error: %v", err)
			return
		}
		if resp != nil {
			defer func() { _ = resp.Body.Close() }()
		}
	}()
}

func NotifyTeams(webhookURL string, title, text string) {
	if !isValidURL(webhookURL) {
		return
	}
	payload := map[string]string{
		"@type":      "MessageCard",
		"@context":   "http://schema.org/extensions",
		"themeColor": "0076D7",
		"summary":    title,
		"text":       text,
	}
	go func() {
		data, err := json.Marshal(payload)
		if err != nil {
			log.Printf("Teams webhook error: %v", err)
			return
		}
		resp, err := webhookClient.Post(webhookURL, "application/json", bytes.NewBuffer(data))
		if err != nil {
			log.Printf("Teams webhook error: %v", err)
			return
		}
		if resp != nil {
			defer func() { _ = resp.Body.Close() }()
		}
	}()
}

func isValidURL(u string) bool {
	p, err := url.Parse(u)
	if err != nil || (p.Scheme != "http" && p.Scheme != "https") {
		return false
	}

	host, _, err := net.SplitHostPort(p.Host)
	if err != nil {
		host = p.Host
	}

	// Block loopback addresses to prevent SSRF
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return false
	}

	// Block private/link-local IP ranges to prevent SSRF
	ip := net.ParseIP(host)
	if ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()) {
		return false
	}

	return true
}
