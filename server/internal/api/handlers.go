package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"text/template"
	"time"

	"github.com/arumes31/patchli/server/internal/auth"
	"github.com/arumes31/patchli/server/internal/fleet"
	"github.com/arumes31/patchli/server/static"
)

func HandleNodes(w http.ResponseWriter, r *http.Request) {
	nodes := fleet.Registry.GetNodes()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(nodes); err != nil {
		log.Printf("Error encoding nodes: %v", err)
	}
}

func HandleStats(w http.ResponseWriter, r *http.Request) {
	nodes := fleet.Registry.GetNodes()
	online := 0
	rebootRequired := 0
	for _, n := range nodes {
		if strings.ToLower(n.Status) == "online" {
			online++
		}
		if strings.Contains(strings.ToLower(n.Status), "reboot") {
			rebootRequired++
		}
	}

	compliance := "0%"
	if len(nodes) > 0 {
		compliance = fmt.Sprintf("%d%%", (online * 100 / len(nodes)))
	}

	stats := struct {
		Vitality   int    `json:"vitality"`
		Immune     string `json:"immune"`
		Recovery   int    `json:"recovery"`
	}{
		Vitality: len(nodes),
		Immune:   compliance,
		Recovery: rebootRequired,
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(stats); err != nil {
		log.Printf("Error encoding stats: %v", err)
	}
}

func HandleSetup(w http.ResponseWriter, r *http.Request) {
	group := r.URL.Query().Get("group")
	if group == "" { group = "default" }
	osType := r.URL.Query().Get("os")
	if osType == "" { osType = "linux" }

	timestamp := time.Now().Format(time.RFC3339)
	signature := auth.GenerateRegistrationSignature(group, timestamp)

	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		scheme := "http"
		if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
			if strings.ToLower(proto) == "https" {
				scheme = "https"
			}
		} else if r.TLS != nil {
			scheme = "https"
		}
		baseURL = fmt.Sprintf("%s://%s", scheme, r.Host)
	}

	serverHost := baseURL
	if idx := strings.Index(baseURL, "://"); idx != -1 {
		serverHost = baseURL[idx+3:]
	}

	var script string
	switch osType {
	case "windows":
		script = generateWindowsScript(group, timestamp, signature, serverHost)
	case "alpine":
		script = generateAlpineScript(group, timestamp, signature, serverHost)
	default:
		script = generateLinuxScript(group, timestamp, signature, serverHost)
	}

	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprint(w, script)
}

func ServeSetupUI(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.ParseFS(static.FS, "setup.html")
	if err != nil {
		http.Error(w, "Failed to load setup page", http.StatusInternalServerError)
		return
	}
	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		scheme := "http"
		if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
			if strings.ToLower(proto) == "https" {
				scheme = "https"
			}
		} else if r.TLS != nil {
			scheme = "https"
		}
		baseURL = fmt.Sprintf("%s://%s", scheme, r.Host)
	}
	data := struct { BaseURL string }{ BaseURL: baseURL }
	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("Error executing template: %v", err)
	}
}

func generateLinuxScript(group, ts, sig, url string) string {
	return fmt.Sprintf(` + "`" + `#!/bin/bash
set -e
echo "--- Patchli Agent Setup (Linux) ---"
GROUP="%s"
TIMESTAMP="%s"
SIGNATURE="%s"
SERVER_URL="%s"

echo "1. Creating configuration..."
mkdir -p /etc/patchli
cat <<EOF > /etc/patchli/config.yaml
server_url: $SERVER_URL
group: $GROUP
