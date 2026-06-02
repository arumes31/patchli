package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/arumes31/patchli/server/internal/fleet"
	"github.com/arumes31/patchli/server/internal/models"
)

func TestHandleNodes(t *testing.T) {
	fleet.Registry.Reset()
	fleet.Registry.Register("00:11:22:33:44:55", nil, models.HeartbeatPayload{
		MacAddress: "00:11:22:33:44:55",
		Hostname:   "test-node",
		OS:         "linux",
	})

	req, err := http.NewRequest("GET", "/api/v1/nodes", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(HandleNodes)

	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	var nodes []models.AgentDetails
	if err := json.Unmarshal(rr.Body.Bytes(), &nodes); err != nil {
		t.Errorf("failed to unmarshal response: %v", err)
	}

	if len(nodes) != 1 {
		t.Errorf("expected 1 node, got %d", len(nodes))
	}
	if nodes[0].Hostname != "test-node" {
		t.Errorf("expected hostname test-node, got %s", nodes[0].Hostname)
	}
}

func TestHandleStats(t *testing.T) {
	fleet.Registry.Reset()
	fleet.Registry.Register("00:11:22:33:44:55", nil, models.HeartbeatPayload{
		MacAddress: "00:11:22:33:44:55",
		Hostname:   "online-node",
		OS:         "linux",
	})

	req, err := http.NewRequest("GET", "/api/v1/stats", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(HandleStats)

	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	var stats struct {
		Vitality int    `json:"vitality"`
		Immune   string `json:"immune"`
		Recovery int    `json:"recovery"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &stats); err != nil {
		t.Errorf("failed to unmarshal response: %v", err)
	}

	if stats.Vitality != 1 {
		t.Errorf("expected vitality 1, got %d", stats.Vitality)
	}
	if stats.Immune != "100%" {
		t.Errorf("expected immune 100%%, got %s", stats.Immune)
	}
}

func TestHandleStream(t *testing.T) {
	req, err := http.NewRequest("GET", "/api/v1/stream", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()

	ctx, cancel := context.WithCancel(req.Context())
	req = req.WithContext(ctx)

	go func() {
		cancel()
	}()

	HandleStream(rr, req)

	if rr.Header().Get("Content-Type") != "text/event-stream" {
		t.Errorf("expected Content-Type text/event-stream, got %s", rr.Header().Get("Content-Type"))
	}
}

func TestHandleSetup(t *testing.T) {
	tests := []struct {
		name     string
		os       string
		contains string
	}{
		{"Linux", "linux", "GROUP=\"test\""},
		{"Alpine", "alpine", "--- Patchli Agent Setup (Alpine) ---"},
		{"Windows", "windows", "--- Patchli Agent Setup (Windows) ---"},
		{"Default", "", "GROUP=\"test\""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := "/api/v1/setup?group=test"
			if tt.os != "" {
				url += "&os=" + tt.os
			}
			req, err := http.NewRequest("GET", url, nil)
			if err != nil {
				t.Fatal(err)
			}

			rr := httptest.NewRecorder()
			handler := http.HandlerFunc(HandleSetup)

			handler.ServeHTTP(rr, req)

			if status := rr.Code; status != http.StatusOK {
				t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
			}

			if !strings.Contains(rr.Body.String(), tt.contains) {
				t.Errorf("Response body should contain %q", tt.contains)
			}
		})
	}
}

func TestServeSetupUI(t *testing.T) {
	req, err := http.NewRequest("GET", "/setup", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(ServeSetupUI)

	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	if !strings.Contains(rr.Body.String(), "<title>Patchli - Add Agent</title>") {
		t.Error("Response body should contain correct title")
	}
}
