package fleet

import (
	"strings"
	"sync"
	"time"

	"github.com/arumes31/patchli/server/internal/models"
	"github.com/gorilla/websocket"
)

type AgentManager struct {
	agents     map[string]*websocket.Conn
	details    map[string]*models.AgentDetails
	activeJobs map[string]string
	mu         sync.RWMutex
}

var Registry = AgentManager{
	agents:     make(map[string]*websocket.Conn),
	details:    make(map[string]*models.AgentDetails),
	activeJobs: make(map[string]string),
}

func (am *AgentManager) Register(mac string, conn *websocket.Conn, p models.HeartbeatPayload) {
	am.mu.Lock()
	defer am.mu.Unlock()
	// Close existing connection to prevent resource leak on re-registration
	if oldConn, ok := am.agents[mac]; ok {
		oldConn.Close()
	}
	am.agents[mac] = conn
	am.details[mac] = &models.AgentDetails{
		Hostname:      p.Hostname,
		MacAddress:    p.MacAddress,
		OS:            p.OS,
		Kernel:        p.Kernel,
		Status:        "Online",
		LastHeartbeat: time.Now().Format(time.RFC3339),
	}
}

func (am *AgentManager) Unregister(mac string) {
	am.mu.Lock()
	defer am.mu.Unlock()
	delete(am.agents, mac)
	delete(am.activeJobs, mac)
	if a, ok := am.details[mac]; ok {
		a.Status = "offline"
	}
}

func (am *AgentManager) PruneStaleAgents(ttl time.Duration) {
	am.mu.Lock()
	defer am.mu.Unlock()
	cutoff := time.Now().Add(-ttl)
	for mac, detail := range am.details {
		last, err := time.Parse(time.RFC3339, detail.LastHeartbeat)
		if err != nil || last.Before(cutoff) {
			delete(am.details, mac)
			delete(am.agents, mac)
			delete(am.activeJobs, mac)
		}
	}
}

func (am *AgentManager) GetNodes() []models.AgentDetails {
	am.mu.RLock()
	defer am.mu.RUnlock()
	nodes := make([]models.AgentDetails, 0, len(am.details))
	for _, d := range am.details {
		nodes = append(nodes, *d)
	}
	return nodes
}

// ⚡ Bolt: Calculate stats directly from the map to avoid O(N) slice allocation overhead
func (am *AgentManager) GetStats() (int, int, int) {
	am.mu.RLock()
	defer am.mu.RUnlock()
	total := len(am.details)
	online := 0
	rebootRequired := 0
	for _, d := range am.details {
		if strings.Contains(strings.ToLower(d.Status), "online") {
			online++
		}
		if strings.Contains(strings.ToLower(d.Status), "reboot") {
			rebootRequired++
		}
	}
	return total, online, rebootRequired
}

func (am *AgentManager) SendCommand(mac string, cmd models.CommandPayload) error {
	am.mu.Lock()
	conn, ok := am.agents[mac]
	if !ok {
		am.mu.Unlock()
		return models.ErrAgentOffline
	}

	msg := struct {
		Type    string      `json:"type"`
		Payload interface{} `json:"payload"`
	}{
		Type:    "command",
		Payload: cmd,
	}
	err := conn.WriteJSON(msg)
	am.mu.Unlock()
	return err
}

func (am *AgentManager) AcquireJob(mac, jobID string) bool {
	am.mu.Lock()
	defer am.mu.Unlock()
	if _, busy := am.activeJobs[mac]; busy {
		return false
	}
	am.activeJobs[mac] = jobID
	return true
}

func (am *AgentManager) ReleaseJob(mac string) {
	am.mu.Lock()
	defer am.mu.Unlock()
	delete(am.activeJobs, mac)
}

func (am *AgentManager) Reset() {
	am.mu.Lock()
	defer am.mu.Unlock()
	am.agents = make(map[string]*websocket.Conn)
	am.details = make(map[string]*models.AgentDetails)
	am.activeJobs = make(map[string]string)
}

func (am *AgentManager) SetStatus(id string, status string) {
	am.mu.Lock()
	defer am.mu.Unlock()
	if a, ok := am.details[id]; ok {
		a.Status = status
	}
}
