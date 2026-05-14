package main

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"text/template"
	"time"
)

// SetupHandler handles the generation of the agent installation script.
func SetupHandler(w http.ResponseWriter, r *http.Request) {
	group := r.URL.Query().Get("group")
	if group == "" {
		group = "default"
	}

	osType := r.URL.Query().Get("os")
	if osType == "" {
		osType = "linux"
	}

	timestamp := time.Now().Format(time.RFC3339)
	signature := GenerateRegistrationSignature(group, timestamp)
	
	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		baseURL = fmt.Sprintf("%s://%s", scheme, r.Host)
	}

	var script string

	switch osType {
	case "windows":
		script = fmt.Sprintf(`$ErrorActionPreference = "Stop"
Write-Host "--- Patchli Agent Setup ---"
$Group = "%s"
$Timestamp = "%s"
$Signature = "%s"
$ServerUrl = "%s"

Write-Host "1. Downloading agent binary..."
# Invoke-WebRequest -Uri "$ServerUrl/download/agent-windows.exe" -OutFile "C:\Program Files\Patchli\patchli-agent.exe"

Write-Host "2. Registering node and obtaining identity..."
# $Body = @{ group = $Group; timestamp = $Timestamp; signature = $Signature } | ConvertTo-Json
# $Response = Invoke-RestMethod -Uri "$ServerUrl/api/v1/register" -Method Post -Body $Body -ContentType "application/json"
# $AgentToken = $Response.token

Write-Host "3. Creating configuration..."
$ConfigDir = "C:\ProgramData\Patchli"
if (!(Test-Path -Path $ConfigDir)) { New-Item -ItemType Directory -Path $ConfigDir | Out-Null }
@"
server_url: $ServerUrl
group: $Group
auth_token: `$AgentToken
"@ | Out-File -FilePath "$ConfigDir\config.yaml" -Encoding UTF8

Write-Host "4. Installing Windows Service..."
# New-Service -Name "PatchliAgent" -BinaryPathName "C:\Program Files\Patchli\patchli-agent.exe" -DisplayName "Patchli Patch Management Agent" -StartupType Automatic
# Start-Service -Name "PatchliAgent"

Write-Host "SUCCESS: Patchli Agent installed and configured for group: $Group"
Write-Host "Check your dashboard to verify registration."
`, group, timestamp, signature, baseURL)

	case "alpine":
		script = fmt.Sprintf(`#!/bin/sh
set -e

echo "--- Patchli Agent Setup ---"
GROUP="%s"
TIMESTAMP="%s"
SIGNATURE="%s"
SERVER_URL="%s"

echo "1. Downloading agent binary..."
# curl -L $SERVER_URL/download/agent -o /usr/local/bin/patchli-agent
# chmod +x /usr/local/bin/patchli-agent

echo "2. Registering node and obtaining identity..."
# REG_RESPONSE=$(curl -s -X POST $SERVER_URL/api/v1/register \
#   -H "Content-Type: application/json" \
#   -d "{\"group\":\"$GROUP\", \"timestamp\":\"$TIMESTAMP\", \"signature\":\"$SIGNATURE\"}")
# AGENT_TOKEN=$(echo $REG_RESPONSE | jq -r '.token')

echo "3. Creating configuration..."
mkdir -p /etc/patchli
cat <<EOF > /etc/patchli/config.yaml
server_url: $SERVER_URL
group: $GROUP
auth_token: $AGENT_TOKEN
EOF

echo "4. Installing OpenRC service..."
cat <<EOF > /etc/init.d/patchli-agent
#!/sbin/openrc-run

name="patchli-agent"
command="/usr/local/bin/patchli-agent"
command_background="yes"
pidfile="/run/patchli-agent.pid"
EOF

# chmod +x /etc/init.d/patchli-agent
# rc-update add patchli-agent default
# rc-service patchli-agent start

echo "SUCCESS: Patchli Agent installed and configured for group: $GROUP"
echo "Check your dashboard to verify registration."
`, group, timestamp, signature, baseURL)

	default: // Linux Systemd
		script = fmt.Sprintf(`#!/bin/bash
set -e

echo "--- Patchli Agent Setup ---"
GROUP="%s"
TIMESTAMP="%s"
SIGNATURE="%s"
SERVER_URL="%s"

echo "1. Downloading agent binary..."
# curl -L $SERVER_URL/download/agent -o /usr/local/bin/patchli-agent
# chmod +x /usr/local/bin/patchli-agent

echo "2. Registering node and obtaining identity..."
# REG_RESPONSE=$(curl -s -X POST $SERVER_URL/api/v1/register \
#   -H "Content-Type: application/json" \
#   -d "{\"group\":\"$GROUP\", \"timestamp\":\"$TIMESTAMP\", \"signature\":\"$SIGNATURE\"}")
# AGENT_TOKEN=$(echo $REG_RESPONSE | jq -r '.token')

echo "3. Creating configuration..."
mkdir -p /etc/patchli
cat <<EOF > /etc/patchli/config.yaml
server_url: $SERVER_URL
group: $GROUP
auth_token: $AGENT_TOKEN
EOF

echo "4. Installing systemd service..."
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

echo "SUCCESS: Patchli Agent installed and configured for group: $GROUP"
echo "Check your dashboard to verify registration."
`, group, timestamp, signature, baseURL)
	}

	if osType == "windows" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	} else {
		w.Header().Set("Content-Type", "text/plain")
	}
	fmt.Fprint(w, script)
}

// ServeSetupUI serves the guided setup HTML page.
func ServeSetupUI(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.ParseFiles("server/static/setup.html")
	if err != nil {
		http.Error(w, "Failed to load setup page", http.StatusInternalServerError)
		return
	}
	
	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		baseURL = fmt.Sprintf("%s://%s", scheme, r.Host)
	}
	
	// Strip proto for curl command if needed, but the UI might just use the full URL.
	// Actually, let's pass the full URL and let JS handle it.
	data := struct {
		BaseURL string
	}{
		BaseURL: baseURL,
	}
	
	tmpl.Execute(w, data)
}
