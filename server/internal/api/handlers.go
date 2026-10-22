package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/arumes31/patchli/server/internal/auth"
	"github.com/arumes31/patchli/server/internal/fleet"
	"github.com/arumes31/patchli/server/internal/proxy"
	"github.com/arumes31/patchli/server/static"
)

var validGroupRegex = regexp.MustCompile("^[a-zA-Z0-9._-]+$")

// validHostRegex matches a plain host name, IPv4 or bracketed IPv6 address
// with an optional port. It is applied to the Host header when BASE_URL is
// not configured.
var validHostRegex = regexp.MustCompile(`^([a-zA-Z0-9.-]+|\[[0-9a-fA-F:.]+\])(:[0-9]{1,5})?$`)

var errInvalidBaseURL = errors.New("invalid base URL")

func isValidGroupName(group string) bool {
	return validGroupRegex.MatchString(group)
}

func escapeBash(s string) string {
	return strings.ReplaceAll(s, "'", "'\\''")
}

func escapePowerShell(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

// resolveBaseURL returns the externally visible server URL. BASE_URL should
// always be set in production; the Host header fallback is restricted to
// well-formed host[:port] values so it cannot inject markup or script text.
func resolveBaseURL(r *http.Request) (string, error) {
	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		// NOTE: If TRUSTED_PROXY is not set, X-Forwarded-Proto is ignored and only r.TLS is used.
		// Set TRUSTED_PROXY to the proxy's IP to enable X-Forwarded-Proto trust.
		if !validHostRegex.MatchString(r.Host) {
			return "", errInvalidBaseURL
		}
		baseURL = fmt.Sprintf("%s://%s", proxy.Scheme(r), r.Host)
	}
	parsedURL, err := url.Parse(baseURL)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") || parsedURL.Host == "" {
		return "", errInvalidBaseURL
	}
	return baseURL, nil
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
		log.Printf("Response write failed: %v", err)
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
		log.Printf("Response write failed: %v", err)
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

	baseURL, err := resolveBaseURL(r)
	if err != nil {
		log.Printf("Invalid BaseURL rejected for setup script: %q", r.Host) // #nosec G706 -- %q escapes control characters.
		http.Error(w, "Invalid server configuration", http.StatusInternalServerError)
		return
	}

	timestamp := time.Now().Format(time.RFC3339)
	nonce := auth.NewRegistrationNonce()
	signature := auth.GenerateRegistrationSignature(group, timestamp, nonce)

	var script string
	switch osType {
	case "windows":
		script = generateWindowsScript(group, timestamp, nonce, signature, baseURL)
	case "alpine":
		script = generateAlpineScript(group, timestamp, nonce, signature, baseURL)
	default:
		script = generateLinuxScript(group, timestamp, nonce, signature, baseURL)
	}

	w.Header().Set("Content-Type", "text/plain")
	if _, err := fmt.Fprint(w, script); err != nil {
		log.Printf("Setup response write failed: %v", err)
	}
}

func ServeSetupUI(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.ParseFS(static.FS, "setup.html")
	if err != nil {
		http.Error(w, "Failed to load setup page", http.StatusInternalServerError)
		return
	}
	baseURL, err := resolveBaseURL(r)
	if err != nil {
		log.Printf("Invalid BaseURL rejected for host %q", r.Host) // #nosec G706 -- %q escapes control characters.
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

func generateLinuxScript(group, ts, nonce, sig, url string) string {
	return fmt.Sprintf(`#!/bin/bash
set -e
echo "--- Patchli Agent Setup (Linux) ---"
GROUP='%s'
TIMESTAMP='%s'
NONCE='%s'
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
`, escapeBash(group), escapeBash(ts), escapeBash(nonce), escapeBash(sig), escapeBash(url))
}

func generateAlpineScript(group, ts, nonce, sig, url string) string {
	return fmt.Sprintf(`#!/bin/sh
set -e
echo "--- Patchli Agent Setup (Alpine) ---"
GROUP='%s'
TIMESTAMP='%s'
NONCE='%s'
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
`, escapeBash(group), escapeBash(ts), escapeBash(nonce), escapeBash(sig), escapeBash(url))
}

func generateWindowsScript(group, ts, nonce, sig, url string) string {
	return fmt.Sprintf(`$ErrorActionPreference = "Stop"
Write-Host "--- Patchli Agent Setup (Windows) ---"
$Group = '%s'
$Timestamp = '%s'
$Nonce = '%s'
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
`, escapePowerShell(group), escapePowerShell(ts), escapePowerShell(nonce), escapePowerShell(sig), escapePowerShell(url))
}

func HandleStream(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	if _, err := fmt.Fprintf(w, "data: %s\n\n", `{"message": "System stream initialized", "level": "system"}`); err != nil {
		return
	}
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	} else {
		log.Println("ResponseWriter does not support flushing")
	}

	<-r.Context().Done()
}
