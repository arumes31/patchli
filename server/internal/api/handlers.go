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

var (
	validGroupRegex = regexp.MustCompile("^[a-zA-Z0-9._-]+$")
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
		Vitality int    `json:"vitality"`
		Immune   string `json:"immune"`
		Recovery int    `json:"recovery"`
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
	return fmt.Sprintf("#!/bin/bash\nset -e\necho \"--- Patchli Agent Setup (Linux) ---\"\nGROUP='%s'\nTIMESTAMP='%s'\nSIGNATURE='%s'\nSERVER_URL='%s'\n\necho \"1. Creating configuration...\"\nmkdir -p /etc/patchli\ncat <<EOF > /etc/patchli/config.yaml\nserver_url: $SERVER_URL\ngroup: $GROUP\nEOF\n\necho \"2. Installing systemd service...\"\ncat <<EOF > /etc/systemd/system/patchli-agent.service\n[Unit]\nDescription=Patchli Patch Management Agent\nAfter=network.target\n\n[Service]\nExecStart=/usr/local/bin/patchli-agent\nRestart=always\nUser=root\n\n[Install]\nWantedBy=multi-user.target\nEOF\n\n# systemctl daemon-reload\n# systemctl enable patchli-agent\n# systemctl start patchli-agent\n\necho \"SUCCESS: Patchli Agent configured for group: $GROUP\"\n", escapeBash(group), escapeBash(ts), escapeBash(sig), escapeBash(url))
}

func generateAlpineScript(group, ts, sig, url string) string {
	return fmt.Sprintf("#!/bin/sh\nset -e\necho \"--- Patchli Agent Setup (Alpine) ---\"\nGROUP='%s'\nTIMESTAMP='%s'\nSIGNATURE='%s'\nSERVER_URL='%s'\n\necho \"1. Creating configuration...\"\nmkdir -p /etc/patchli\ncat <<EOF > /etc/patchli/config.yaml\nserver_url: $SERVER_URL\ngroup: $GROUP\nEOF\n\necho \"2. Installing OpenRC service...\"\ncat <<EOF > /etc/init.d/patchli-agent\n#!/sbin/openrc-run\nname=\"patchli-agent\"\ncommand=\"/usr/local/bin/patchli-agent\"\ncommand_background=\"yes\"\npidfile=\"/run/patchli-agent.pid\"\nEOF\nchmod +x /etc/init.d/patchli-agent\n\necho \"SUCCESS: Patchli Agent configured for group: $GROUP\"\n", escapeBash(group), escapeBash(ts), escapeBash(sig), escapeBash(url))
}

func generateWindowsScript(group, ts, sig, url string) string {
	return fmt.Sprintf("$ErrorActionPreference = \"Stop\"\nWrite-Host \"--- Patchli Agent Setup (Windows) ---\"\n$Group = '%s'\n$Timestamp = '%s'\n$Signature = '%s'\n$ServerUrl = '%s'\n\nWrite-Host \"1. Creating configuration...\"\n$ConfigDir = \"C:\\ProgramData\\Patchli\"\nif (!(Test-Path -Path $ConfigDir)) { New-Item -ItemType Directory -Path $ConfigDir | Out-Null }\n@\"\nserver_url: $ServerUrl\ngroup: $Group\n\"@ | Out-File -FilePath \"$ConfigDir\\config.yaml\" -Encoding UTF8\n\nWrite-Host \"SUCCESS: Patchli Agent configured for group: $Group\"\n", escapePowerShell(group), escapePowerShell(ts), escapePowerShell(sig), escapePowerShell(url))
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

func isValidGroupName(group string) bool {
	return validGroupRegex.MatchString(group)
}

func escapePowerShell(s string) string {
	// In PowerShell single-quoted strings, only single quotes need to be escaped by doubling them
	return strings.ReplaceAll(s, "'", "''")
}

func escapeBash(s string) string {
	// In Bash single-quoted strings, the only character that cannot appear is a single quote.
	// We can escape it by closing the string, adding an escaped single quote, and reopening.
	return strings.ReplaceAll(s, "'", "'\\''")
}
