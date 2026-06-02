package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
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

	if !strings.Contains(rr.Body.String(), "GROUP='test'") {
		t.Error("Response body should contain group name")
	}
}

func TestHandleSetupInjection(t *testing.T) {
	os.Setenv("REGISTRATION_SECRET", "testsecret")
	os.Setenv("JWT_SECRET", "atleast16charslongsecret")

	tests := []struct {
		name        string
		group       string
		osType      string
		expected    string
		notExpected string
	}{
		{
			name:        "Linux injection",
			group:       "default'; touch /tmp/pwned; #",
			osType:      "linux",
			expected:    "GROUP='default'\\''; touch /tmp/pwned; #'",
			notExpected: "GROUP='default'; touch /tmp/pwned; #'",
		},
		{
			name:        "Windows injection",
			group:       "default'; Write-Host 'pwned'; #",
			osType:      "windows",
			expected:    "$Group = 'default''; Write-Host ''pwned''; #'",
			notExpected: "$Group = 'default'; Write-Host 'pwned'; #'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := url.Values{}
			q.Add("group", tt.group)
			q.Add("os", tt.osType)

			req, _ := http.NewRequest("GET", "/api/v1/setup?"+q.Encode(), nil)
			rr := httptest.NewRecorder()
			handler := http.HandlerFunc(HandleSetup)
			handler.ServeHTTP(rr, req)

			body := rr.Body.String()
			if !strings.Contains(body, tt.expected) {
				t.Errorf("Could not find expected escaped string in body. Body was:\n%s", body)
			}
			if tt.notExpected != "" && strings.Contains(body, tt.notExpected) {
				t.Errorf("Found unescaped (dangerous) string in body. Body was:\n%s", body)
			}
		})
	}
}

func TestServeSetupUI(t *testing.T) {
	t.Skip("Skipping template test due to relative path issues in tests")
}
