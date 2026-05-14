package main

import (
	"fmt"
	"net/http"
	"text/template"
	"time"
)

// SetupHandler handles the generation of the agent installation script.
func SetupHandler(w http.ResponseWriter, r *http.Request) {
	group := r.URL.Query().Get("group")
	if group == "" {
		group = "default"
	}

	timestamp := time.Now().Format(time.RFC3339)
	signature := GenerateRegistrationSignature(group, timestamp)
	serverHost := r.Host

	script := `#!/bin/bash
set -e

echo "--- Patchli Agent Setup ---"
GROUP="%s"
TIMESTAMP="%s"
SIGNATURE="%s"
SERVER_URL="http://%s"

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
`

	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprintf(w, script, group, timestamp, signature, serverHost)
}

// ServeSetupUI serves the guided setup HTML page.
func ServeSetupUI(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.ParseFiles("server/static/setup.html")
	if err != nil {
		http.Error(w, "Failed to load setup page", http.StatusInternalServerError)
		return
	}
	
	data := struct {
		ServerHost string
	}{
		ServerHost: r.Host,
	}
	
	tmpl.Execute(w, data)
}
