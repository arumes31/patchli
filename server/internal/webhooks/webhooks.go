package webhooks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"
)

var webhookClient = &http.Client{
	Timeout: 10 * time.Second,
	CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	},
	Transport: &http.Transport{
		DialContext: safeDialContext,
	},
}

func safeDialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("resolve webhook host: %w", err)
	}
	if len(addresses) == 0 {
		return nil, errors.New("webhook host has no addresses")
	}
	for _, resolved := range addresses {
		ip := resolved.IP
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() {
			return nil, errors.New("webhook host resolves to a non-public address")
		}
	}
	dialer := &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
	return dialer.DialContext(ctx, network, net.JoinHostPort(addresses[0].IP.String(), port))
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
			log.Printf("Rejected invalid or insecure webhook URL")
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

	// #nosec G704 -- isValidURL requires HTTPS and the client's dialer resolves once and rejects every non-public address.
	req, err := http.NewRequest("POST", targetURL, bytes.NewBuffer(data))
	if err != nil {
		log.Printf("Failed to create webhook request: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	// #nosec G704 -- safeDialContext dials only the already-validated public resolution and redirects are disabled.
	resp, err := webhookClient.Do(req)
	if err != nil {
		log.Printf("Webhook delivery failed: %v", err)
		return
	}
	defer func() { _ = resp.Body.Close() }()

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
	if err != nil || p.Scheme != "https" || p.Host == "" || p.User != nil {
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
