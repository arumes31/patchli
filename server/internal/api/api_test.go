package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/arumes31/patchli/server/internal/auth"
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

func TestHandleRegister(t *testing.T) {
	now := time.Now().Format(time.RFC3339)
	sig := auth.GenerateRegistrationSignature("default", now)

	body := map[string]string{
		"group":     "default",
		"timestamp": now,
		"signature": sig,
		"node_id":   "test-node-123",
	}
	b, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/api/v1/register", strings.NewReader(string(b)))
	rr := httptest.NewRecorder()

	HandleRegister(rr, req)

	if status := rr.Code; status != http.StatusCreated {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusCreated)
	}

	var tokens auth.TokenResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &tokens); err != nil {
		t.Fatal(err)
	}
	if tokens.AccessToken == "" || tokens.RefreshToken == "" {
		t.Error("expected access and refresh tokens")
	}
}

func TestHandleRefresh(t *testing.T) {
	tokens, _ := auth.GenerateAgentJWT("test-node-123")

	body := map[string]string{
		"refresh_token": tokens.RefreshToken,
	}
	b, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/api/v1/refresh", strings.NewReader(string(b)))
	rr := httptest.NewRecorder()

	HandleRefresh(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	var newTokens auth.TokenResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &newTokens); err != nil {
		t.Fatal(err)
	}
	if newTokens.AccessToken == "" || newTokens.RefreshToken == "" {
		t.Error("expected new access and refresh tokens")
	}
}
