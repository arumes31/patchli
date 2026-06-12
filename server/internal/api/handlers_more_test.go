package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"os"
)

func TestHandleSetup_Variants(t *testing.T) {
	os.Setenv("BASE_URL", "https://custom.example.com")
	defer os.Unsetenv("BASE_URL")

	variants := []struct{
		os string
		expected string
	}{
		{"windows", "Patchli Agent Setup (Windows)"},
		{"alpine", "Patchli Agent Setup (Alpine)"},
		{"linux", "Patchli Agent Setup (Linux)"},
		{"", "Patchli Agent Setup (Linux)"},
	}

	for _, v := range variants {
		req, _ := http.NewRequest("GET", "/api/v1/setup?os="+v.os, nil)
		rr := httptest.NewRecorder()
		HandleSetup(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("expected 200 for os=%s, got %d", v.os, rr.Code)
		}
		if !contains(rr.Body.String(), v.expected) {
			t.Errorf("os=%s: expected %s in output", v.os, v.expected)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || s[0:len(substr)] == substr || contains(s[1:], substr))
}
