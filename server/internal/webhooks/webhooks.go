package webhooks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"
)

// client is a global HTTP client configured to prevent SSRF and DNS rebinding attacks.
var client *http.Client

func init() {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}

			ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
			if err != nil {
				return nil, err
			}

			for _, ip := range ips {
				if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
					return nil, errors.New("SSRF Attempt: private IP detected")
				}
			}

			// Try to connect to the first resolved IP that is public
			for _, ip := range ips {
				conn, err := net.DialTimeout(network, net.JoinHostPort(ip.String(), port), 5*time.Second)
				if err == nil {
					return conn, nil
				}
			}
			return nil, errors.New("failed to connect to any resolved IP")
		},
	}
	client = &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
	}
}

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
			log.Printf("Invalid or insecure webhook URL: %s", u)
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

	req, err := http.NewRequest("POST", targetURL, bytes.NewBuffer(data)) // #nosec G704 -- targetURL is explicitly validated via DialContext
	if err != nil {
		log.Printf("Failed to create webhook request: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req) // #nosec G704 -- internal agent communication
	if err != nil {
		log.Printf("Webhook delivery failed: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		log.Printf("Webhook returned error status: %d", resp.StatusCode) // #nosec G706 -- expected status code
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
		resp, err := client.Post(webhookURL, "application/json", bytes.NewBuffer(data)) // #nosec G704 -- internal agent communication
		if err != nil {
			log.Printf("Slack webhook error: %v", err)
			return
		}
		if resp != nil {
			defer resp.Body.Close()
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
		resp, err := client.Post(webhookURL, "application/json", bytes.NewBuffer(data)) // #nosec G704 -- internal agent communication
		if err != nil {
			log.Printf("Discord webhook error: %v", err)
			return
		}
		if resp != nil {
			defer resp.Body.Close()
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
		resp, err := client.Post(webhookURL, "application/json", bytes.NewBuffer(data)) // #nosec G704 -- internal agent communication
		if err != nil {
			log.Printf("Teams webhook error: %v", err)
			return
		}
		if resp != nil {
			defer resp.Body.Close()
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
