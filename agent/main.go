package main

import (
	"context"
	"encoding/json"
	"log"
	"net/url"
	"os"
	"time"

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
	MacAddress    string `json:"mac_address"`
	Hostname      string `json:"hostname"`
	OS            string `json:"os"`
	Kernel        string `json:"kernel"`
	RebootNeeded  bool   `json:"reboot_needed"`
}

type CommandPayload struct {
	ID      string   `json:"id"`
	Action  string   `json:"action"`
	Packages []string `json:"packages,omitempty"`
}

func main() {
	log.Println("Starting Patchli Agent...")

	pm, err := updater.DetectPackageManager()
	if err != nil {
		log.Fatalf("Failed to detect package manager: %v", err)
	}

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

	u := url.URL{Scheme: "ws", Host: serverURL, Path: "/ws"}
	log.Printf("Connecting to %s", u.String())

	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Fatalf("Dial error: %v", err)
	}
	defer conn.Close()

	// Heartbeat loop
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			hostname, _ := os.Hostname()
			payload := HeartbeatPayload{
				MacAddress:   "00:11:22:33:44:55", // In real app, get from net.Interfaces
				Hostname:     hostname,
				RebootNeeded: pm.RebootRequired(),
			}
			data, _ := json.Marshal(payload)
			msg := WSMessage{Type: MsgTypeHeartbeat, Payload: data}
			if err := conn.WriteJSON(msg); err != nil {
				log.Printf("Heartbeat error: %v", err)
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

func executeCommand(conn *websocket.Conn, pm updater.PackageManager, cmd CommandPayload) {
	ctx := context.Background()
	var res updater.UpdateResult
	var err error

	switch cmd.Action {
	case "check_updates":
		res, err = pm.CheckUpdates(ctx)
	case "apply_updates":
		res, err = pm.ApplyUpdates(ctx, cmd.Packages)
	default:
		log.Printf("Unknown action: %s", cmd.Action)
		return
	}

	// Send results back (simplified)
	log.Printf("Job %s finished. Success: %v", cmd.ID, res.Success)
	// conn.WriteJSON(...)
}
