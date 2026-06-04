package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/arumes31/patchli/server/internal/models"
)

func TestHandleNodes(t *testing.T) {
	os.Setenv("ADMIN_TOKEN", "testtoken")
	defer os.Unsetenv("ADMIN_TOKEN")

	req, err := http.NewRequest("GET", "/api/v1/nodes", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer testtoken")

	rr := httptest.NewRecorder()
	handler := AuthMiddleware(HandleNodes)

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
	os.Setenv("ADMIN_TOKEN", "testtoken")
	defer os.Unsetenv("ADMIN_TOKEN")

	req, err := http.NewRequest("GET", "/api/v1/stats", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer testtoken")

	rr := httptest.NewRecorder()
	handler := AuthMiddleware(HandleStats)

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
	os.Setenv("ADMIN_TOKEN", "testtoken")
	defer os.Unsetenv("ADMIN_TOKEN")
	os.Setenv("REGISTRATION_SECRET", "testsecret")
	defer os.Unsetenv("REGISTRATION_SECRET")

	req, err := http.NewRequest("GET", "/api/v1/setup?group=test&os=linux", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer testtoken")

	rr := httptest.NewRecorder()
	handler := AuthMiddleware(HandleSetup)

	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v, body: %s", status, http.StatusOK, rr.Body.String())
	}

	if !strings.Contains(rr.Body.String(), "GROUP=\"test\"") {
		t.Error("Response body should contain group name")
	}
}

func TestHandleSetupUnauthorized(t *testing.T) {
	os.Setenv("ADMIN_TOKEN", "testtoken")
	defer os.Unsetenv("ADMIN_TOKEN")

	tests := []struct {
		name       string
		authHeader string
		wantCode   int
	}{
		{"No Auth Header", "", http.StatusUnauthorized},
		{"Wrong Token", "Bearer wrongtoken", http.StatusUnauthorized},
		{"Malformed Header", "WrongPrefix testtoken", http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/api/v1/setup", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}

			rr := httptest.NewRecorder()
			handler := AuthMiddleware(HandleSetup)
			handler.ServeHTTP(rr, req)

			if rr.Code != tt.wantCode {
				t.Errorf("expected status %v, got %v", tt.wantCode, rr.Code)
			}
		})
	}
}

func TestAuthMiddlewareNoAdminTokenSet(t *testing.T) {
	os.Unsetenv("ADMIN_TOKEN")

	req, _ := http.NewRequest("GET", "/api/v1/setup", nil)
	req.Header.Set("Authorization", "Bearer anything")

	rr := httptest.NewRecorder()
	handler := AuthMiddleware(HandleSetup)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected status %v when ADMIN_TOKEN is not set, got %v", http.StatusUnauthorized, rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Server Configuration Error") {
		t.Errorf("expected error message to mention configuration error, got %s", rr.Body.String())
	}
}

func TestServeSetupUI(t *testing.T) {
	// This requires the template file to exist at the correct relative path
	// During tests, the working directory might be internal/api, so server/static/setup.html won't be found.
	// We might need to skip this or mock the filesystem.
	t.Skip("Skipping template test due to relative path issues in tests")
}
