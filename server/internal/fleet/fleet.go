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
	if d, ok := am.details[mac]; ok {
		d.Status = "offline"
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

// ⚡ Bolt: GetStats calculates node statistics directly without O(N) slice allocation
func (am *AgentManager) GetStats() (total int, online int, reboot int) {
	am.mu.RLock()
	defer am.mu.RUnlock()
	total = len(am.details)
	for _, d := range am.details {
		lowerStatus := strings.ToLower(d.Status)
		if strings.Contains(lowerStatus, "online") {
			online++
		}
		if strings.Contains(lowerStatus, "reboot") {
			reboot++
		}
	}
	return total, online, reboot
}

func (am *AgentManager) SendCommand(mac string, cmd models.CommandPayload) error {
	am.mu.RLock()
	conn, ok := am.agents[mac]
	am.mu.RUnlock()
	if !ok {
		return models.ErrAgentOffline
	}

	msg := struct {
		Type    string      `json:"type"`
		Payload interface{} `json:"payload"`
	}{
		Type:    "command",
		Payload: cmd,
	}
	return conn.WriteJSON(msg)
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
