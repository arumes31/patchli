package webhooks

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"
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
			log.Printf("Invalid webhook URL: %s", u)
			continue
		}
		go sendWebhook(u, payload)
	}
}

func sendWebhook(targetURL string, payload WebhookPayload) {
	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Failed to marshal webhook payload: %v", err)
		return
	}

	req, err := http.NewRequest("POST", targetURL, bytes.NewBuffer(data))
	if err != nil {
		log.Printf("Failed to create webhook request: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Webhook delivery failed: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		log.Printf("Webhook returned error status: %d", resp.StatusCode)
	}
}

func NotifySlack(webhookURL string, msg string) {
	if !isValidURL(webhookURL) { return }
	payload := map[string]string{"text": msg}
	data, _ := json.Marshal(payload)
	resp, err := http.Post(webhookURL, "application/json", bytes.NewBuffer(data))
	if err != nil {
		log.Printf("Slack webhook error: %v", err)
		return
	}
	if resp != nil {
		defer resp.Body.Close()
	}
}

func NotifyDiscord(webhookURL string, msg string) {
	if !isValidURL(webhookURL) { return }
	payload := map[string]string{"content": msg}
	data, _ := json.Marshal(payload)
	resp, err := http.Post(webhookURL, "application/json", bytes.NewBuffer(data))
	if err != nil {
		log.Printf("Discord webhook error: %v", err)
		return
	}
	if resp != nil {
		defer resp.Body.Close()
	}
}

func NotifyTeams(webhookURL string, title, text string) {
	if !isValidURL(webhookURL) { return }
	payload := map[string]string{
		"@type":      "MessageCard",
		"@context":   "http://schema.org/extensions",
		"themeColor": "0076D7",
		"summary":    title,
		"text":       text,
	}
	data, _ := json.Marshal(payload)
	resp, err := http.Post(webhookURL, "application/json", bytes.NewBuffer(data))
	if err != nil {
		log.Printf("Teams webhook error: %v", err)
		return
	}
	if resp != nil {
		defer resp.Body.Close()
	}
}

func isValidURL(u string) bool {
	p, err := url.Parse(u)
	return err == nil && (p.Scheme == "http" || p.Scheme == "https")
}
