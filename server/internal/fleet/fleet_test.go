package fleet

import (
	"testing"

	"github.com/arumes31/patchli/server/internal/models"
	"github.com/gorilla/websocket"
)

func TestAgentManager(t *testing.T) {
	am := &AgentManager{
		agents:  make(map[string]*websocket.Conn),
		details: make(map[string]*models.AgentDetails),
	}

	mac := "00:11:22:33:44:55"
	details := &models.AgentDetails{
		Hostname:   "test-host",
		MacAddress: mac,
		OS:         "Linux",
		Status:     "Online",
	}

	t.Run("GetNodes empty", func(t *testing.T) {
		nodes := am.GetNodes()
		if len(nodes) != 0 {
			t.Errorf("Expected 0 nodes, got %d", len(nodes))
		}
	})

	t.Run("Add and GetNodes", func(t *testing.T) {
		am.mu.Lock()
		am.details[mac] = details
		am.mu.Unlock()

		nodes := am.GetNodes()
		if len(nodes) != 1 {
			t.Errorf("Expected 1 node, got %d", len(nodes))
		}
		if nodes[0].MacAddress != mac {
			t.Errorf("Expected mac %s, got %s", mac, nodes[0].MacAddress)
		}
	})

	t.Run("SendCommand Offline", func(t *testing.T) {
		err := am.SendCommand("offline-mac", models.CommandPayload{Action: "patch"})
		if err == nil {
			t.Error("Expected error when sending command to offline agent")
		}
	})
}
