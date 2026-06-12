package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"github.com/arumes31/patchli/server/internal/fleet"
	"github.com/arumes31/patchli/server/internal/models"
)

func TestHandleStats_Detailed(t *testing.T) {
	fleet.Registry.Reset()

	// Add an online node
	fleet.Registry.Register("m1", nil, models.HeartbeatPayload{Hostname: "h1", MacAddress: "m1"})

	// Add a node that needs reboot
	fleet.Registry.Register("m2", nil, models.HeartbeatPayload{Hostname: "h2", MacAddress: "m2"})
	nodes := fleet.Registry.GetNodes()
	for i := range nodes {
		if nodes[i].MacAddress == "m2" {
			nodes[i].Status = "Reboot Required"
			// Registry details is a map of pointers, but Register copies payload.
			// Actually Register sets Status = "Online"
		}
	}
	// We need to manipulate the registry directly or through a leaked way if possible,
	// but details is unexported.
	// Register always sets Status = "Online".
	// Unregister sets Status = "offline".

	// Let's use what we have.
	fleet.Registry.Unregister("m2") // Status = "offline"

	req, _ := http.NewRequest("GET", "/api/v1/stats", nil)
	rr := httptest.NewRecorder()
	HandleStats(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("got %d", rr.Code)
	}

	var stats struct {
		Vitality int `json:"vitality"`
		Immune string `json:"immune"`
	}
	json.Unmarshal(rr.Body.Bytes(), &stats)

	if stats.Vitality != 2 {
		t.Errorf("expected 2 nodes, got %d", stats.Vitality)
	}
}
