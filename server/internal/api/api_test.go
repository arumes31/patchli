package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
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
		t.Errorf("Response body should contain group name: got %s", rr.Body.String())
	}
}

func TestServeSetupUI(t *testing.T) {
	// This requires the template file to exist at the correct relative path
	// During tests, the working directory might be internal/api, so server/static/setup.html won't be found.
	// We might need to skip this or mock the filesystem.
	t.Skip("Skipping template test due to relative path issues in tests")
}

func TestHandleSetupSecurity(t *testing.T) {
	tests := []struct {
		name     string
		os       string
		payload  string
		expected string
	}{
		{
			name:     "Linux Script Injection",
			os:       "linux",
			payload:  "test'; whoami; '",
			expected: "GROUP='test'\\''; whoami; '\\'''",
		},
		{
			name:     "Alpine Script Injection",
			os:       "alpine",
			payload:  "test'; whoami; '",
			expected: "GROUP='test'\\''; whoami; '\\'''",
		},
		{
			name:     "Windows Script Injection",
			os:       "windows",
			payload:  "test'; whoami; '",
			expected: "$Group = 'test''; whoami; '''",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, _ := url.Parse("/api/v1/setup")
			q := u.Query()
			q.Set("group", tt.payload)
			q.Set("os", tt.os)
			u.RawQuery = q.Encode()

			req, err := http.NewRequest("GET", u.String(), nil)
			if err != nil {
				t.Fatal(err)
			}

			rr := httptest.NewRecorder()
			handler := http.HandlerFunc(HandleSetup)
			handler.ServeHTTP(rr, req)

			if status := rr.Code; status != http.StatusOK {
				t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
			}

			body := rr.Body.String()
			if !strings.Contains(body, tt.expected) {
				t.Errorf("expected pattern not found in body.\nExpected: %s\nBody:\n%s", tt.expected, body)
			}
		})
	}
}
