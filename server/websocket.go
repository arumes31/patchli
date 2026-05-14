package main

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // In production, verify origin/token
	},
}

type AgentDetails struct {
	Hostname      string `json:"hostname"`
	MacAddress    string `json:"mac_address"`
	OS            string `json:"os"`
	Kernel        string `json:"kernel"`
	Status        string `json:"status"`
	LastHeartbeat string `json:"last_heartbeat"`
}

// AgentManager tracks active WebSocket connections and their details.
type AgentManager struct {
	agents     map[string]*websocket.Conn // mac_address -> connection
	details    map[string]*AgentDetails   // mac_address -> details
	activeJobs map[string]string         // node_mac -> job_id
	mu         sync.RWMutex
}

var manager = AgentManager{
	agents:     make(map[string]*websocket.Conn),
	details:    make(map[string]*AgentDetails),
	activeJobs: make(map[string]string),
}

// Message types for communication
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
	ID                 string   `json:"id"`
	Action             string   `json:"action"` // e.g., "patch", "reboot", "chaos_restart"
	Packages           []string `json:"packages,omitempty"`
	PrePatchScript     string   `json:"pre_patch_script,omitempty"`
	PostPatchScript    string   `json:"post_patch_script,omitempty"`
	HealthCheckCommand string   `json:"health_check_command,omitempty"`
}

type LogPayload struct {
	JobID string `json:"job_id"`
	Data  string `json:"data"`
}

func HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Upgrade error: %v", err)
		return
	}
	defer conn.Close()

	var macAddr string

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			log.Printf("Read error: %v", err)
			if macAddr != "" {
				manager.mu.Lock()
				delete(manager.agents, macAddr)
				manager.mu.Unlock()
				log.Printf("Agent %s disconnected", macAddr)
			}
			break
		}

		var wsMsg WSMessage
		if err := json.Unmarshal(message, &wsMsg); err != nil {
			log.Printf("Unmarshal error: %v", err)
			continue
		}

		switch wsMsg.Type {
		case MsgTypeHeartbeat:
			var p HeartbeatPayload
			json.Unmarshal(wsMsg.Payload, &p)
			macAddr = p.MacAddress
			
			manager.mu.Lock()
			manager.agents[macAddr] = conn
			manager.details[macAddr] = &AgentDetails{
				Hostname:      p.Hostname,
				MacAddress:    p.MacAddress,
				OS:            p.OS,
				Kernel:        p.Kernel,
				Status:        "Online",
				LastHeartbeat: time.Now().Format(time.RFC3339),
			}
			manager.mu.Unlock()

			// Update node status in DB (skipped for brevity, but this is where it happens)
			// log.Printf("Heartbeat from %s (%s)", p.Hostname, macAddr)

		case MsgTypeLog:
			var p LogPayload
			json.Unmarshal(wsMsg.Payload, &p)
			// Stream to dashboard or log file
			// log.Printf("[%s] %s", p.JobID, p.Data)

		case MsgTypeResult:
			// Handle job completion
		}
	}
}

// SendCommand sends a command to a specific agent.
func (am *AgentManager) SendCommand(macAddr string, cmd CommandPayload) error {
	am.mu.RLock()
	conn, ok := am.agents[macAddr]
	am.mu.RUnlock()

	if !ok {
		return http.ErrHandlerTimeout // Or custom "Agent Offline" error
	}

	payload, _ := json.Marshal(cmd)
	msg := WSMessage{
		Type:    MsgTypeCommand,
		Payload: payload,
	}

	return conn.WriteJSON(msg)
}

// GetNodes returns a list of all known nodes.
func (am *AgentManager) GetNodes() []AgentDetails {
	am.mu.RLock()
	defer am.mu.RUnlock()

	nodes := make([]AgentDetails, 0, len(am.details))
	for _, d := range am.details {
		nodes = append(nodes, *d)
	}
	return nodes
}
