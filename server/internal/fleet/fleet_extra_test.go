package fleet

import (
	"testing"
	"time"
	"github.com/arumes31/patchli/server/internal/models"
	"github.com/gorilla/websocket"
)

func TestAgentManager_Prune(t *testing.T) {
	am := &AgentManager{
		agents: make(map[string]*websocket.Conn),
		details: make(map[string]*models.AgentDetails),
		activeJobs: make(map[string]string),
	}
	am.Register("old", nil, models.HeartbeatPayload{Hostname: "old"})
	am.details["old"].LastHeartbeat = time.Now().Add(-2 * time.Hour).Format(time.RFC3339)

	am.Register("new", nil, models.HeartbeatPayload{Hostname: "new"})

	am.PruneStaleAgents(1 * time.Hour)
	if len(am.GetNodes()) != 1 {
		t.Errorf("expected 1 node after prune, got %d", len(am.GetNodes()))
	}
}

func TestAgentManager_SendCommand_Offline_Extra(t *testing.T) {
	am := &AgentManager{
		agents: make(map[string]*websocket.Conn),
		details: make(map[string]*models.AgentDetails),
		activeJobs: make(map[string]string),
	}
	err := am.SendCommand("offline-mac", models.CommandPayload{ID: "1"})
	if err != models.ErrAgentOffline {
		t.Errorf("expected ErrAgentOffline, got %v", err)
	}
}
