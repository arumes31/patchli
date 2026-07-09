package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"text/template"
	"time"

	"github.com/arumes31/patchli/server/internal/auth"
	"github.com/arumes31/patchli/server/internal/fleet"
	"github.com/arumes31/patchli/server/static"
)

var validGroupRegex = regexp.MustCompile("^[a-zA-Z0-9._-]+$")

func isValidGroupName(group string) bool {
	return validGroupRegex.MatchString(group)
}

func escapeBash(s string) string {
	return strings.ReplaceAll(s, "'", "'\\''")
}

func escapePowerShell(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

func HandleNodes(w http.ResponseWriter, r *http.Request) {
	nodes := fleet.Registry.GetNodes()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(nodes); err != nil {
		log.Printf("Error encoding nodes: %v", err)
	}
}

// ⚡ Bolt: Using GetStats() instead of GetNodes() avoids O(N) allocation for aggregate calculations.
func HandleStats(w http.ResponseWriter, r *http.Request) {
	total, online, rebootRequired := fleet.Registry.GetStats()

	compliance := "0%"
	if total > 0 {
		compliance = fmt.Sprintf("%d%%", (online * 100 / total))
	}

	stats := struct {
		Vitality int    `json:"vitality"`
		Immune   string `json:"immune"`
		Recovery int    `json:"recovery"`
	}{
		Vitality: total,
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
	if group == "" {
		group = "default"
	}
	if !isValidGroupName(group) {
		http.Error(w, "Invalid group name", http.StatusBadRequest)
		return
	}
	osType := r.URL.Query().Get("os")
	if osType == "" {
		osType = "linux"
	}

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

	var script string
	switch osType {
	case "windows":
		script = generateWindowsScript(group, timestamp, signature, baseURL)
	case "alpine":
		script = generateAlpineScript(group, timestamp, signature, baseURL)
	default:
		script = generateLinuxScript(group, timestamp, signature, baseURL)
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
	data := struct{ BaseURL string }{BaseURL: baseURL}
	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("Error executing template: %v", err)
	}
}

func generateLinuxScript(group, ts, sig, url string) string {
	return fmt.Sprintf(`#!/bin/bash
set -e
echo "--- Patchli Agent Setup (Linux) ---"
GROUP='%s'
TIMESTAMP='%s'
SIGNATURE='%s'
SERVER_URL='%s'

echo "1. Creating configuration..."
mkdir -p /etc/patchli
cat <<EOF > /etc/patchli/config.yaml
server_url: $SERVER_URL
group: $GROUP
EOF

echo "2. Installing systemd service..."
cat <<EOF > /etc/systemd/system/patchli-agent.service
[Unit]
Description=Patchli Patch Management Agent
After=network.target

[Service]
ExecStart=/usr/local/bin/patchli-agent
Restart=always
User=root

[Install]
WantedBy=multi-user.target
EOF

# systemctl daemon-reload
# systemctl enable patchli-agent
# systemctl start patchli-agent

echo "SUCCESS: Patchli Agent configured for group: $GROUP"
`, escapeBash(group), escapeBash(ts), escapeBash(sig), escapeBash(url))
}

func generateAlpineScript(group, ts, sig, url string) string {
	return fmt.Sprintf(`#!/bin/sh
set -e
echo "--- Patchli Agent Setup (Alpine) ---"
GROUP='%s'
TIMESTAMP='%s'
SIGNATURE='%s'
SERVER_URL='%s'

echo "1. Creating configuration..."
mkdir -p /etc/patchli
cat <<EOF > /etc/patchli/config.yaml
server_url: $SERVER_URL
group: $GROUP
EOF

echo "2. Installing OpenRC service..."
cat <<EOF > /etc/init.d/patchli-agent
#!/sbin/openrc-run
name="patchli-agent"
command="/usr/local/bin/patchli-agent"
command_background="yes"
pidfile="/run/patchli-agent.pid"
EOF
chmod +x /etc/init.d/patchli-agent

echo "SUCCESS: Patchli Agent configured for group: $GROUP"
`, escapeBash(group), escapeBash(ts), escapeBash(sig), escapeBash(url))
}

func generateWindowsScript(group, ts, sig, url string) string {
	return fmt.Sprintf(`$ErrorActionPreference = "Stop"
Write-Host "--- Patchli Agent Setup (Windows) ---"
$Group = '%s'
$Timestamp = '%s'
$Signature = '%s'
$ServerUrl = '%s'

Write-Host "1. Creating configuration..."
$ConfigDir = "C:\ProgramData\Patchli"
if (!(Test-Path -Path $ConfigDir)) { New-Item -ItemType Directory -Path $ConfigDir | Out-Null }
@"
server_url: $ServerUrl
group: $Group
"@ | Out-File -FilePath "$ConfigDir\config.yaml" -Encoding UTF8

Write-Host "SUCCESS: Patchli Agent configured for group: $Group"
`, escapePowerShell(group), escapePowerShell(ts), escapePowerShell(sig), escapePowerShell(url))
}

func HandleStream(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	fmt.Fprintf(w, "data: %s\n\n", `{"message": "System stream initialized", "level": "system"}`)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	} else {
		log.Println("ResponseWriter does not support flushing")
	}

	<-r.Context().Done()
}
