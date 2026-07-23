package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/arumes31/patchli/server/internal/auth"
)

func TestMain(m *testing.M) {
	auth.SetTestSecrets()
	os.Exit(m.Run())
}

func TestHandleSetupSecurity(t *testing.T) {
	tests := []struct {
		name           string
		group          string
		expectedStatus int
	}{
		{"valid", "production", http.StatusOK},
		{"valid-dots", "web.prod.01", http.StatusOK},
		{"valid-dashes", "web-prod-01", http.StatusOK},
		{"valid-underscores", "web_prod_01", http.StatusOK},
		{"invalid-space", "prod web", http.StatusBadRequest},
		{"invalid-semicolon", "prod; echo vulnerable", http.StatusBadRequest},
		{"invalid-quote", "prod' injection", http.StatusBadRequest},
		{"invalid-backtick", "prod` injection", http.StatusBadRequest},
		{"invalid-dollar", "prod$ injection", http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, _ := url.Parse("/api/v1/setup")
			q := u.Query()
			q.Set("group", tt.group)
			u.RawQuery = q.Encode()

			req, err := http.NewRequest("GET", u.String(), nil)
			if err != nil {
				t.Fatal(err)
			}

			rr := httptest.NewRecorder()
			handler := http.HandlerFunc(HandleSetup)

			handler.ServeHTTP(rr, req)

			if status := rr.Code; status != tt.expectedStatus {
				t.Errorf("handler returned wrong status code for %s: got %v want %v", tt.group, status, tt.expectedStatus)
			}
		})
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

func TestEscapeBash(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"no single quotes", "hello", "hello"},
		{"single quote", "it's", "it'\\''s"},
		{"multiple quotes", "it's a test's", "it'\\''s a test'\\''s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := escapeBash(tt.input)
			if result != tt.expected {
				t.Errorf("escapeBash(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestEscapePowerShell(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"no single quotes", "hello", "hello"},
		{"single quote", "it's", "it''s"},
		{"multiple quotes", "it's a test's", "it''s a test''s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := escapePowerShell(tt.input)
			if result != tt.expected {
				t.Errorf("escapePowerShell(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestIsValidGroupName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		valid bool
	}{
		{"simple", "production", true},
		{"with dots", "web.prod.01", true},
		{"with dashes", "web-prod-01", true},
		{"with underscores", "web_prod_01", true},
		{"with space", "prod web", false},
		{"with semicolon", "prod; echo", false},
		{"with quote", "prod' injection", false},
		{"with backtick", "prod` injection", false},
		{"with dollar", "prod$ injection", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidGroupName(tt.input)
			if result != tt.valid {
				t.Errorf("isValidGroupName(%q) = %v, want %v", tt.input, result, tt.valid)
			}
		})
	}
}

func TestHandleNodesWithAuth(t *testing.T) {
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
}

func TestHandleStatsWithAuth(t *testing.T) {
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
