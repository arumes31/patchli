package fleet

import (
	"testing"
	"github.com/arumes31/patchli/server/internal/models"
	"github.com/gorilla/websocket"
)

func TestAgentManager_Full(t *testing.T) {
	am := &AgentManager{
		agents: make(map[string]*websocket.Conn),
		details: make(map[string]*models.AgentDetails),
		activeJobs: make(map[string]string),
	}

	am.Register("mac1", nil, models.HeartbeatPayload{Hostname: "host1", MacAddress: "mac1"})
	if len(am.GetNodes()) != 1 {
		t.Error("expected 1 node")
	}

	if !am.AcquireJob("mac1", "job1") {
		t.Error("should acquire job")
	}
	if am.AcquireJob("mac1", "job2") {
		t.Error("should NOT acquire second job")
	}

	am.ReleaseJob("mac1")
	if !am.AcquireJob("mac1", "job2") {
		t.Error("should acquire job after release")
	}

	am.Unregister("mac1")
	nodes := am.GetNodes()
	if len(nodes) == 0 {
		t.Error("expected nodes list not to be empty")
	} else if nodes[0].Status != "offline" {
		t.Error("status should be offline")
	}

	am.Reset()
	if len(am.GetNodes()) != 0 {
		t.Error("should be empty after reset")
	}
}
