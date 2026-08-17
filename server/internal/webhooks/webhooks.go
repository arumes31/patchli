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

// secureHTTPClient mitigates TOCTOU DNS Rebinding SSRF vulnerabilities
var secureHTTPClient = &http.Client{
	Timeout: 10 * time.Second,
	Transport: &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, err
			}
			if len(ips) == 0 {
				return nil, errors.New("no IP addresses found")
			}
			ip := ips[0].IP
			if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
				return nil, errors.New("blocked private/loopback IP")
			}
			ipStr := ip.String()
			if ip.To4() == nil {
				ipStr = "[" + ipStr + "]"
			}
			dialer := &net.Dialer{Timeout: 5 * time.Second}
			return dialer.DialContext(ctx, network, ipStr+":"+port)
		},
	},
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

	req, err := http.NewRequest("POST", targetURL, bytes.NewBuffer(data))
	if err != nil {
		log.Printf("Failed to create webhook request: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	client := secureHTTPClient
	resp, err := client.Do(req) // #nosec G704 -- mitigated by secureHTTPClient
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
		client := secureHTTPClient
		resp, err := client.Post(webhookURL, "application/json", bytes.NewBuffer(data)) // #nosec G704 -- mitigated by secureHTTPClient
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
		client := secureHTTPClient
		resp, err := client.Post(webhookURL, "application/json", bytes.NewBuffer(data)) // #nosec G704 -- mitigated by secureHTTPClient
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
		client := secureHTTPClient
		resp, err := client.Post(webhookURL, "application/json", bytes.NewBuffer(data)) // #nosec G704 -- mitigated by secureHTTPClient
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
