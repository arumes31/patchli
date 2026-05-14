package main

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"time"
)

type WebhookPayload struct {
	Event   string `json:"event"`
	Message string `json:"message"`
	JobID   string `json:"job_id,omitempty"`
	NodeMac string `json:"node_mac,omitempty"`
}

// NotifyWebhooks sends an outward webhook to configured URLs.
func NotifyWebhooks(payload WebhookPayload) {
	// In production, these URLs would be fetched from the database based on group settings.
	urls := []string{
		// os.Getenv("SLACK_WEBHOOK_URL"),
		// os.Getenv("TEAMS_WEBHOOK_URL"),
		// os.Getenv("DISCORD_WEBHOOK_URL"),
	}

	for _, u := range urls {
		if u == "" {
			continue
		}
		go sendWebhook(u, payload)
	}
}

func sendWebhook(url string, payload WebhookPayload) {
	data, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(data))
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

// Specific formatters for different platforms (Optional, as many accept standard JSON)

func NotifySlack(url string, msg string) {
	payload := map[string]string{"text": msg}
	data, _ := json.Marshal(payload)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(data))
	if err != nil {
		log.Printf("Slack webhook error: %v", err)
		return
	}
	if resp != nil {
		defer resp.Body.Close()
	}
}

func NotifyDiscord(url string, msg string) {
	payload := map[string]string{"content": msg}
	data, _ := json.Marshal(payload)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(data))
	if err != nil {
		log.Printf("Discord webhook error: %v", err)
		return
	}
	if resp != nil {
		defer resp.Body.Close()
	}
}

func NotifyTeams(url string, title, text string) {
	payload := map[string]string{
		"@type":      "MessageCard",
		"@context":   "http://schema.org/extensions",
		"themeColor": "0076D7",
		"summary":    title,
		"text":       text,
	}
	data, _ := json.Marshal(payload)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(data))
	if err != nil {
		log.Printf("Teams webhook error: %v", err)
		return
	}
	if resp != nil {
		defer resp.Body.Close()
	}
}
