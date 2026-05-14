package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/patchli/agent/updater"
)

// Message types matching server
const (
	MsgTypeHeartbeat = "heartbeat"
	MsgTypeCommand   = "command"
	MsgTypeLog       = "log"
	MsgTypeResult    = "result"
)

type WSMessage struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type HeartbeatPayload struct {
	MacAddress    string `json:"mac_address"` // Keeping field name for backwards compatibility, but it will hold UUID
	Hostname      string `json:"hostname"`
	OS            string `json:"os"`
	Kernel        string `json:"kernel"`
	RebootNeeded  bool   `json:"reboot_needed"`
}

type CommandPayload struct {
	ID                 string   `json:"id"`
	Action             string   `json:"action"`
	Packages           []string `json:"packages,omitempty"`
	PrePatchScript     string   `json:"pre_patch_script,omitempty"`
	PostPatchScript    string   `json:"post_patch_script,omitempty"`
	HealthCheckCommand string   `json:"health_check_command,omitempty"`
}

func getOrGenerateIdentity() string {
	idFile := "/etc/patchli/node_id"
	if data, err := os.ReadFile(idFile); err == nil && len(data) > 0 {
		return strings.TrimSpace(string(data))
	}

	newID := uuid.New().String()
	if err := os.MkdirAll("/etc/patchli", 0755); err != nil {
		log.Printf("Warning: failed to create /etc/patchli directory: %v", err)
	} else if err := os.WriteFile(idFile, []byte(newID), 0644); err != nil {
		log.Printf("Warning: failed to write node ID to %s: %v", idFile, err)
	}
	return newID
}

func main() {
	log.Println("Starting Patchli Agent...")

	pm, err := updater.DetectPackageManager()
	if err != nil {
		log.Fatalf("Failed to detect package manager: %v", err)
	}

	nodeID := getOrGenerateIdentity()
	log.Printf("Agent Identity (UUID): %s", nodeID)

	// State Recovery on Boot
	lastState, _ := updater.LoadState()
	if lastState != nil && lastState.Status == "running" {
		log.Printf("RECOVERY: Detected interrupted job %s. Reporting to server...", lastState.JobID)
		// Logic to notify server that job was interrupted and check current package status
	}

	serverURL := os.Getenv("SERVER_URL")
	if serverURL == "" {
		serverURL = "localhost:8080"
	}

	// Try WebSocket first, fallback to HTTP long-polling
	u := url.URL{Scheme: "ws", Host: serverURL, Path: "/ws"}
	log.Printf("Connecting to %s", u.String())

	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Printf("WebSocket dial error: %v. Falling back to HTTP Long-Polling...", err)
		startHTTPPolling(serverURL, nodeID, pm)
		return // Block forever in HTTP polling
	}
	defer conn.Close()

	// Heartbeat loop
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			hostname, _ := os.Hostname()
			payload := HeartbeatPayload{
				MacAddress:   nodeID,
				Hostname:     hostname,
				RebootNeeded: pm.RebootRequired(),
			}
			data, _ := json.Marshal(payload)
			msg := WSMessage{Type: MsgTypeHeartbeat, Payload: data}
			if err := conn.WriteJSON(msg); err != nil {
				log.Printf("Heartbeat error: %v", err)
				// If WS dies, we could attempt to switch to HTTP polling here.
				return
			}
		}
	}()

	// Command listener loop
	for {
		var wsMsg WSMessage
		err := conn.ReadJSON(&wsMsg)
		if err != nil {
			log.Printf("Read error: %v", err)
			break
		}

		if wsMsg.Type == MsgTypeCommand {
			var cmd CommandPayload
			json.Unmarshal(wsMsg.Payload, &cmd)
			log.Printf("Received command: %s (Job: %s)", cmd.Action, cmd.ID)
			
			go executeCommand(conn, pm, cmd)
		}
	}
}

func startHTTPPolling(serverHost, nodeID string, pm updater.PackageManager) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	client := &http.Client{Timeout: 60 * time.Second} // Long poll timeout

	for {
		// Send heartbeat
		hostname, _ := os.Hostname()
		hb := HeartbeatPayload{
			MacAddress:   nodeID,
			Hostname:     hostname,
			RebootNeeded: pm.RebootRequired(),
		}
		data, _ := json.Marshal(hb)
		
		req, _ := http.NewRequest("POST", "http://"+serverHost+"/api/v1/poll", bytes.NewBuffer(data))
		req.Header.Set("Content-Type", "application/json")
		
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("HTTP Poll error: %v", err)
			time.Sleep(10 * time.Second)
			continue
		}
		
		if resp.StatusCode == http.StatusOK {
			var cmd CommandPayload
			if err := json.NewDecoder(resp.Body).Decode(&cmd); err == nil && cmd.Action != "" {
				go executeCommand(nil, pm, cmd)
			}
		}
		resp.Body.Close()
		<-ticker.C
	}
}

func executeCommand(conn *websocket.Conn, pm updater.PackageManager, cmd CommandPayload) {
	ctx := context.Background()
	var res updater.UpdateResult
	var err error

	// Pre-flight check for actual update actions
	if cmd.Action == "apply_updates" {
		if err := pm.PreFlightCheck(ctx); err != nil {
			log.Printf("PreFlight Check Failed for Job %s: %v", cmd.ID, err)
			// Send error result back
			return
		}

		if cmd.PrePatchScript != "" {
			log.Printf("Executing Pre-Patch Script...")
			out, err := exec.CommandContext(ctx, "sh", "-c", cmd.PrePatchScript).CombinedOutput()
			if err != nil {
				log.Printf("Pre-Patch Script Failed: %v, Output: %s", err, string(out))
				return
			}
		}
		
		// Save state before starting
		updater.SaveState(updater.State{
			JobID: cmd.ID,
			Action: cmd.Action,
			Status: "running",
		})
		defer updater.ClearState()
	}

	switch cmd.Action {
	case "check_updates":
		res, err = pm.CheckUpdates(ctx)
	case "apply_updates":
		res, err = pm.ApplyUpdates(ctx, cmd.Packages)
	case "cleanup":
		err = pm.Cleanup(ctx)
		res = updater.UpdateResult{Success: err == nil, Error: err}
	case "self_destruct":
		err = updater.SelfDestruct()
		res = updater.UpdateResult{Success: err == nil, Error: err}
	case "update_agent":
		log.Printf("Auto-updating agent...")
		res = performSecureAgentUpdate(ctx)
	default:
		log.Printf("Unknown action: %s", cmd.Action)
		return
	}


	if cmd.Action == "apply_updates" && err == nil {
		if cmd.PostPatchScript != "" {
			log.Printf("Executing Post-Patch Script...")
			out, execErr := exec.CommandContext(ctx, "sh", "-c", cmd.PostPatchScript).CombinedOutput()
			if execErr != nil {
				log.Printf("Post-Patch Script Failed: %v, Output: %s", execErr, string(out))
				err = execErr
				res.Success = false
			}
		}

		if err == nil && cmd.HealthCheckCommand != "" {
			log.Printf("Executing Health Check Command...")
			out, execErr := exec.CommandContext(ctx, "sh", "-c", cmd.HealthCheckCommand).CombinedOutput()
			if execErr != nil {
				log.Printf("Health Check Failed: %v, Output: %s", execErr, string(out))
				err = execErr
				res.Success = false
			}
		}
	}

	// Send results back (simplified)
	log.Printf("Job %s finished. Success: %v. Error: %v", cmd.ID, res.Success, err)
	// conn.WriteJSON(...)
}

func performSecureAgentUpdate(ctx context.Context) updater.UpdateResult {
	req, err := http.NewRequestWithContext(ctx, "GET", "http://localhost:8080/download/agent", nil)
	if err != nil {
		return updater.UpdateResult{Success: false, Error: err}
	}
	
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return updater.UpdateResult{Success: false, Error: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return updater.UpdateResult{Success: false, Error: fmt.Errorf("HTTP %d during agent download", resp.StatusCode)}
	}

	tmpFile, err := os.CreateTemp("", "patchli-agent-*")
	if err != nil {
		return updater.UpdateResult{Success: false, Error: err}
	}
	tmpName := tmpFile.Name()
	defer os.Remove(tmpName)

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		tmpFile.Close()
		return updater.UpdateResult{Success: false, Error: err}
	}
	tmpFile.Close()

	// In a real app, verify signature/checksum of tmpName here

	if err := os.Chmod(tmpName, 0755); err != nil {
		return updater.UpdateResult{Success: false, Error: err}
	}

	if err := os.Rename(tmpName, "/usr/local/bin/patchli-agent"); err != nil {
		return updater.UpdateResult{Success: false, Error: err}
	}

	out, err := exec.CommandContext(ctx, "systemctl", "restart", "patchli-agent").CombinedOutput()
	return updater.UpdateResult{Success: err == nil, Output: string(out), Error: err}
}
