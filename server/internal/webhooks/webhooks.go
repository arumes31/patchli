package webhooks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
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

	req, err := http.NewRequest("POST", targetURL, bytes.NewBuffer(data))
	if err != nil {
		log.Printf("Failed to create webhook request: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := safeHTTPClient.Do(req)
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
		resp, err := safeHTTPClient.Post(webhookURL, "application/json", bytes.NewBuffer(data))
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
		resp, err := safeHTTPClient.Post(webhookURL, "application/json", bytes.NewBuffer(data))
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
		resp, err := safeHTTPClient.Post(webhookURL, "application/json", bytes.NewBuffer(data))
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

// safeHTTPClient is reused across requests to prevent resource leaks (goroutines/file descriptors)
var safeHTTPClient = &http.Client{
	Timeout: 10 * time.Second,
	Transport: &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
			if err != nil {
				return nil, err
			}

			if len(ips) == 0 {
				return nil, fmt.Errorf("no IP found")
			}

			var lastErr error
			dialer := &net.Dialer{Timeout: 5 * time.Second}

			// Iterate over all resolved IPs (Happy Eyeballs approach)
			for _, ip := range ips {
				if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
					return nil, fmt.Errorf("SSRF prevention: blocked %s", ip)
				}

				// 🛡️ Sentinel: Dial directly to the validated IP to prevent TOCTOU DNS rebinding SSRF
				conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
				if err == nil {
					return conn, nil
				}
				lastErr = err
			}

			return nil, fmt.Errorf("failed to dial: %w", lastErr)
		},
	},
}
