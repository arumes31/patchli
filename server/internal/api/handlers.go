package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
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

func publicBaseURL() (string, error) {
	raw := strings.TrimSpace(os.Getenv("BASE_URL"))
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return "", fmt.Errorf("BASE_URL must be an https origin")
	}
	return strings.TrimSuffix(parsed.String(), "/"), nil
}

func HandleNodes(w http.ResponseWriter, r *http.Request) {
	nodes := fleet.Registry.GetNodes()
	w.Header().Set("Content-Type", "application/json")
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(nodes); err != nil {
		log.Printf("Error encoding nodes: %v", err)
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
	if _, err := w.Write(buf.Bytes()); err != nil {
		log.Printf("Failed to write nodes response: %v", err)
	}
}

func HandleStats(w http.ResponseWriter, r *http.Request) {
	nodes := fleet.Registry.GetNodes()
	online := 0
	rebootRequired := 0
	for _, n := range nodes {
		if strings.Contains(strings.ToLower(n.Status), "online") {
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
		Vitality int    `json:"vitality"`
		Immune   string `json:"immune"`
		Recovery int    `json:"recovery"`
	}{
		Vitality: len(nodes),
		Immune:   compliance,
		Recovery: rebootRequired,
	}
	w.Header().Set("Content-Type", "application/json")
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(stats); err != nil {
		log.Printf("Error encoding stats: %v", err)
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
	if _, err := w.Write(buf.Bytes()); err != nil {
		log.Printf("Failed to write stats response: %v", err)
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

	baseURL, err := publicBaseURL()
	if err != nil {
		log.Printf("Invalid BASE_URL: %v", err)
		http.Error(w, "Invalid server configuration", http.StatusInternalServerError)
		return
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
	if _, err := fmt.Fprint(w, script); err != nil {
		log.Printf("Failed to write setup script: %v", err)
	}
}

func ServeSetupUI(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.ParseFS(static.FS, "setup.html")
	if err != nil {
		http.Error(w, "Failed to load setup page", http.StatusInternalServerError)
		return
	}
	baseURL, err := publicBaseURL()
	if err != nil {
		log.Printf("Invalid BASE_URL: %v", err)
		http.Error(w, "Invalid server configuration", http.StatusInternalServerError)
		return
	}

	data := struct{ BaseURL string }{BaseURL: baseURL}
	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("Error executing template: %v", err)
		http.Error(w, "Failed to render setup page", http.StatusInternalServerError)
		return
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
install -d -m 0700 /etc/patchli
umask 077
cat <<EOF > /etc/patchli/agent.env
SERVER_URL='$SERVER_URL'
REGISTRATION_GROUP='$GROUP'
REGISTRATION_TIMESTAMP='$TIMESTAMP'
REGISTRATION_SIGNATURE='$SIGNATURE'
EOF

echo "2. Installing systemd service..."
cat <<EOF > /etc/systemd/system/patchli-agent.service
[Unit]
Description=Patchli Patch Management Agent
After=network.target

[Service]
ExecStart=/usr/local/bin/patchli-agent
EnvironmentFile=/etc/patchli/agent.env
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
install -d -m 0700 /etc/patchli
umask 077
cat <<EOF > /etc/conf.d/patchli-agent
export SERVER_URL='$SERVER_URL'
export REGISTRATION_GROUP='$GROUP'
export REGISTRATION_TIMESTAMP='$TIMESTAMP'
export REGISTRATION_SIGNATURE='$SIGNATURE'
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
[Environment]::SetEnvironmentVariable("SERVER_URL", $ServerUrl, "Machine")
[Environment]::SetEnvironmentVariable("REGISTRATION_GROUP", $Group, "Machine")
[Environment]::SetEnvironmentVariable("REGISTRATION_TIMESTAMP", $Timestamp, "Machine")
[Environment]::SetEnvironmentVariable("REGISTRATION_SIGNATURE", $Signature, "Machine")

Write-Host "SUCCESS: Patchli Agent configured for group: $Group"
`, escapePowerShell(group), escapePowerShell(ts), escapePowerShell(sig), escapePowerShell(url))
}

func HandleStream(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	if _, err := fmt.Fprintf(w, "data: %s\n\n", `{"message": "System stream initialized", "level": "system"}`); err != nil {
		log.Printf("Failed to write stream response: %v", err)
		return
	}
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	} else {
		log.Println("ResponseWriter does not support flushing")
	}

	<-r.Context().Done()
}
