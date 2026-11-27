package api

import (
	"errors"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/arumes31/rpatch/server/internal/auth"
	"github.com/arumes31/rpatch/server/internal/cmdsign"
	"github.com/arumes31/rpatch/server/internal/proxy"
	"github.com/arumes31/rpatch/server/static"
)

var validGroupRegex = regexp.MustCompile("^[a-zA-Z0-9._-]+$")

// validHostRegex matches a plain host name, IPv4 or bracketed IPv6 address
// with an optional port. It is applied to the Host header when BASE_URL is
// not configured.
var validHostRegex = regexp.MustCompile(`^([a-zA-Z0-9.-]+|\[[0-9a-fA-F:.]+\])(:[0-9]{1,5})?$`)

var (
	errInvalidBaseURL  = errors.New("invalid base URL")
	errInsecureBaseURL = errors.New("base URL must use HTTPS for non-loopback hosts")
)

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
	// Generated install commands pipe the downloaded script into a privileged
	// shell, so remote origins must be authenticated with TLS. Plain HTTP is
	// only acceptable for a controller on the same host.
	if parsedURL.Scheme == "http" && !isLoopbackHost(parsedURL.Hostname()) {
		return "", errInsecureBaseURL
	}
	return baseURL, nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
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
		log.Printf("BaseURL rejected for setup script (host %q): %v", r.Host, err) // #nosec G706 -- %q escapes control characters.
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
		log.Printf("BaseURL rejected for host %q: %v", r.Host, err) // #nosec G706 -- %q escapes control characters.
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

// generateLinuxScript returns a POSIX sh installer for systemd and OpenRC
// hosts. It downloads the agent from this server, redeems the one-time
// registration grant and starts the service. Re-running it upgrades the agent
// and keeps the existing enrollment.
func generateLinuxScript(group, ts, nonce, sig, url string) string {
	pub := cmdsign.PublicKey()
	insecure := ""
	if strings.HasPrefix(url, "http://") {
		// resolveBaseURL only allows plain HTTP for loopback (development).
		insecure = "ALLOW_INSECURE_LOOPBACK=true\n"
	}
	return fmt.Sprintf(`#!/bin/sh
set -eu
echo "--- rpatch agent setup ---"
SERVER_URL='%s'
COMMAND_PUB_KEY='%s'
RPATCH_GROUP='%s'
RPATCH_TIMESTAMP='%s'
RPATCH_NONCE='%s'
RPATCH_SIGNATURE='%s'
export RPATCH_GROUP RPATCH_TIMESTAMP RPATCH_NONCE RPATCH_SIGNATURE

[ "$(id -u)" -eq 0 ] || { echo "Run as root (sudo sh)." >&2; exit 1; }
case "$(uname -m)" in
  x86_64|amd64) ARCH=amd64 ;;
  aarch64|arm64) ARCH=arm64 ;;
  *) echo "Unsupported architecture: $(uname -m)" >&2; exit 1 ;;
esac

echo "1. Downloading agent (linux/$ARCH)..."
mkdir -p /etc/rpatch /var/lib/rpatch
chmod 700 /etc/rpatch /var/lib/rpatch
TMP=$(mktemp)
trap 'rm -f "$TMP"' EXIT
URL="$SERVER_URL/download/rpatch-agent-linux-$ARCH"
PROTO=$(echo "$SERVER_URL" | cut -d: -f1)
if command -v curl >/dev/null 2>&1; then
  curl -fsS --proto "=$PROTO" --max-redirs 0 -o "$TMP" "$URL"
else
  wget -q --max-redirect=0 -O "$TMP" "$URL"
fi
install -m 0755 "$TMP" /usr/local/bin/rpatch-agent.new
mv -f /usr/local/bin/rpatch-agent.new /usr/local/bin/rpatch-agent

echo "2. Writing configuration..."
umask 077
printf 'SERVER_URL=%%s\nCOMMAND_PUB_KEY=%%s\n%s' "$SERVER_URL" "$COMMAND_PUB_KEY" > /etc/rpatch/agent.env

echo "3. Enrolling..."
set -a
. /etc/rpatch/agent.env
set +a
/usr/local/bin/rpatch-agent --enroll

echo "4. Installing service..."
if [ -d /run/systemd/system ] && command -v systemctl >/dev/null 2>&1; then
  cat > /etc/systemd/system/rpatch-agent.service <<'UNIT'
[Unit]
Description=rpatch patch management agent
After=network-online.target
Wants=network-online.target

[Service]
EnvironmentFile=/etc/rpatch/agent.env
ExecStart=/usr/local/bin/rpatch-agent
Restart=always
RestartSec=10
# Package upgrades must not be killed with the agent.
KillMode=process

[Install]
WantedBy=multi-user.target
UNIT
  systemctl daemon-reload
  systemctl enable rpatch-agent >/dev/null 2>&1
  systemctl restart rpatch-agent
elif command -v rc-update >/dev/null 2>&1; then
  cat > /etc/init.d/rpatch-agent <<'RC'
#!/sbin/openrc-run
name="rpatch-agent"
description="rpatch patch management agent"
command="/usr/local/bin/rpatch-agent"
supervisor="supervise-daemon"
respawn_delay=10
output_log="/var/log/rpatch-agent.log"
error_log="/var/log/rpatch-agent.log"
depend() { need net; }
start_pre() { set -a; . /etc/rpatch/agent.env; set +a; }
RC
  chmod 0755 /etc/init.d/rpatch-agent
  rc-update add rpatch-agent default >/dev/null 2>&1
  rc-service rpatch-agent restart
else
  echo "No systemd or OpenRC found; start /usr/local/bin/rpatch-agent with /etc/rpatch/agent.env yourself." >&2
  exit 1
fi
echo "SUCCESS: rpatch agent installed and enrolled (group: $RPATCH_GROUP)"
`, escapeBash(url), escapeBash(pub), escapeBash(group), escapeBash(ts), escapeBash(nonce), escapeBash(sig), insecure)
}

func generateWindowsScript(group, ts, nonce, sig, url string) string {
	return fmt.Sprintf(`$ErrorActionPreference = "Stop"
Write-Host "--- rpatch Agent Setup (Windows) ---"
$Group = '%s'
$Timestamp = '%s'
$Nonce = '%s'
$Signature = '%s'
$ServerUrl = '%s'

Write-Host "1. Creating configuration..."
$ConfigDir = "C:\ProgramData\rpatch"
if (!(Test-Path -Path $ConfigDir)) { New-Item -ItemType Directory -Path $ConfigDir | Out-Null }
@"
server_url: $ServerUrl
group: $Group
"@ | Out-File -FilePath "$ConfigDir\config.yaml" -Encoding UTF8

Write-Host "SUCCESS: rpatch Agent configured for group: $Group"
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

var agentBinary = regexp.MustCompile(`^rpatch-agent-linux-(amd64|arm64)$`)

// HandleAgentDownload serves agent binaries built into the server image
// (AGENT_DIR, default /app/agents) so agents always match the server.
func HandleAgentDownload(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("file")
	if !agentBinary.MatchString(name) {
		http.NotFound(w, r)
		return
	}
	dir := os.Getenv("AGENT_DIR")
	if dir == "" {
		dir = "/app/agents"
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	http.ServeFileFS(w, r, os.DirFS(dir), name)
}
