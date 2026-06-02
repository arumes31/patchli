package api

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/arumes31/patchli/server/internal/models"
)

func TestHandleNodes(t *testing.T) {
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
}

func TestHandleStats(t *testing.T) {
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
		Vitality int `json:"vitality"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &stats); err != nil {
		t.Errorf("failed to unmarshal response: %v", err)
	}
}

func TestHandleStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(HandleStream))
	defer server.Close()

	client := &http.Client{}
	req, err := http.NewRequest("GET", server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	req = req.WithContext(ctx)

	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Errorf("expected Content-Type text/event-stream, got %s", resp.Header.Get("Content-Type"))
	}

	if resp.Header.Get("Cache-Control") != "no-cache" {
		t.Errorf("expected Cache-Control no-cache, got %s", resp.Header.Get("Cache-Control"))
	}

	if resp.Header.Get("Connection") != "keep-alive" {
		t.Errorf("expected Connection keep-alive, got %s", resp.Header.Get("Connection"))
	}

	reader := bufio.NewReader(resp.Body)
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}

	expectedPrefix := "data: {\"message\": \"System stream initialized\", \"level\": \"system\"}"
	if !strings.HasPrefix(line, expectedPrefix) {
		t.Errorf("expected prefix %s, got %s", expectedPrefix, line)
	}

	cancel()
}

func TestHandleSetup(t *testing.T) {
	req, err := http.NewRequest("GET", "/api/v1/setup?group=test&os=linux", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(HandleSetup)

	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	if !strings.Contains(rr.Body.String(), "GROUP=\"test\"") {
		t.Error("Response body should contain group name")
	}
}

func TestServeSetupUI(t *testing.T) {
	t.Skip("Skipping template test due to relative path issues in tests")
}
