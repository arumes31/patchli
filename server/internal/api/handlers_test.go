package api

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/arumes31/patchli/server/internal/fleet"
	"github.com/arumes31/patchli/server/internal/models"
)

func setupHandlers(t *testing.T) {
	fleet.Registry.Reset()
	t.Cleanup(func() {
		fleet.Registry.Reset()
	})
}

func TestHandleNodes(t *testing.T) {
	setupHandlers(t)

	t.Run("Empty nodes", func(t *testing.T) {
		req, err := http.NewRequest("GET", "/api/v1/nodes", nil)
		if err != nil {
			t.Fatal(err)
		}
		rr := httptest.NewRecorder()
		HandleNodes(rr, req)
		if status := rr.Code; status != http.StatusOK {
			t.Errorf("expected 200, got %d", status)
		}
		if rr.Body.String() != "[]\n" {
			t.Errorf("expected empty array, got %s", rr.Body.String())
		}
	})

	t.Run("With nodes", func(t *testing.T) {
		fleet.Registry.Register("00:11:22:33:44:55", nil, models.HeartbeatPayload{
			Hostname:   "node1",
			MacAddress: "00:11:22:33:44:55",
			OS:         "linux",
		})

		req, err := http.NewRequest("GET", "/api/v1/nodes", nil)
		if err != nil {
			t.Fatal(err)
		}

		rr := httptest.NewRecorder()
		HandleNodes(rr, req)

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
	})
}

func TestHandleStats(t *testing.T) {
	setupHandlers(t)

	t.Run("Empty nodes", func(t *testing.T) {
		req, err := http.NewRequest("GET", "/api/v1/stats", nil)
		if err != nil {
			t.Fatal(err)
		}
		rr := httptest.NewRecorder()
		HandleStats(rr, req)
		var stats struct {
			Vitality  int    `json:"vitality"`
			Immune    string `json:"immune"`
			Recovery  int    `json:"recovery"`
		}
		json.Unmarshal(rr.Body.Bytes(), &stats)
		if stats.Vitality != 0 || stats.Immune != "0%" {
			t.Errorf("Expected 0 stats, got %+v", stats)
		}
	})

	t.Run("With mixed nodes", func(t *testing.T) {
		fleet.Registry.Register("01", nil, models.HeartbeatPayload{MacAddress: "01", Hostname: "n1"})
		fleet.Registry.Register("02", nil, models.HeartbeatPayload{MacAddress: "02", Hostname: "n2"})
		fleet.Registry.Unregister("02") // Status becomes "offline"
		fleet.Registry.Register("03", nil, models.HeartbeatPayload{MacAddress: "03", Hostname: "n3"})
		fleet.Registry.SetStatus("03", "Online, Reboot Required")

		req, err := http.NewRequest("GET", "/api/v1/stats", nil)
		if err != nil {
			t.Fatal(err)
		}

		rr := httptest.NewRecorder()
		HandleStats(rr, req)

		var stats struct {
			Vitality  int    `json:"vitality"`
			Immune    string `json:"immune"`
			Recovery  int    `json:"recovery"`
		}
		json.Unmarshal(rr.Body.Bytes(), &stats)

		if stats.Vitality != 3 {
			t.Errorf("expected vitality 3, got %d", stats.Vitality)
		}
		// 2 online (n1, n3), 3 total -> 66%
		if stats.Immune != "66%" {
			t.Errorf("expected 66%% compliance, got %s", stats.Immune)
		}
		if stats.Recovery != 1 {
			t.Errorf("expected recovery 1, got %d", stats.Recovery)
		}
	})
}

func TestHandleSetup(t *testing.T) {
	setupHandlers(t)

	tests := []struct {
		name            string
		query           string
		header          map[string]string
		env             map[string]string
		useTLS          bool
		expectedInBody  []string
		expectedStatus  int
	}{
		{
			name:           "Defaults",
			query:          "",
			expectedInBody: []string{"GROUP='default'", "#!/bin/bash"},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Linux default",
			query:          "group=prod&os=linux",
			expectedInBody: []string{"GROUP='prod'", "#!/bin/bash", "SERVER_URL="},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Alpine",
			query:          "group=dev&os=alpine",
			expectedInBody: []string{"GROUP='dev'", "#!/bin/sh", "Alpine"},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Windows",
			query:          "group=win&os=windows",
			expectedInBody: []string{"$Group = 'win'", "Windows"},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Invalid group name",
			query:          "group=prod%20web",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Custom BASE_URL",
			query:          "group=test",
			env:            map[string]string{"BASE_URL": "https://patchli.example.com"},
			expectedInBody: []string{"SERVER_URL='https://patchli.example.com'"},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "X-Forwarded-Proto https",
			query:          "group=test",
			header:         map[string]string{"X-Forwarded-Proto": "https"},
			env:            map[string]string{"TRUSTED_PROXY": "127.0.0.1"},
			expectedInBody: []string{"SERVER_URL='https://"},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Direct TLS",
			query:          "group=test",
			useTLS:         true,
			expectedInBody: []string{"SERVER_URL='https://"},
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set env
			for k, v := range tt.env {
				os.Setenv(k, v)
				defer os.Unsetenv(k)
			}

			req, err := http.NewRequest("GET", "/api/v1/setup?"+tt.query, nil)
			if err != nil {
				t.Fatal(err)
			}

			if tt.useTLS {
				req.TLS = &tls.ConnectionState{}
			}

			// Set RemoteAddr to match TRUSTED_PROXY when configured
			if _, ok := tt.env["TRUSTED_PROXY"]; ok {
				req.RemoteAddr = tt.env["TRUSTED_PROXY"] + ":1234"
			}

			// Set headers
			for k, v := range tt.header {
				req.Header.Set(k, v)
			}

			rr := httptest.NewRecorder()
			HandleSetup(rr, req)

			if status := rr.Code; status != tt.expectedStatus {
				t.Errorf("handler returned wrong status code: got %v want %v", status, tt.expectedStatus)
			}

			if tt.expectedStatus == http.StatusOK {
				body := rr.Body.String()
				for _, expected := range tt.expectedInBody {
					if !strings.Contains(body, expected) {
						t.Errorf("expected body to contain %q, but it didn't. Body: %s", expected, body)
					}
				}
			}
		})
	}
}

func TestServeSetupUI(t *testing.T) {
	setupHandlers(t)

	t.Run("Default", func(t *testing.T) {
		req, err := http.NewRequest("GET", "/setup", nil)
		if err != nil {
			t.Fatal(err)
		}

		rr := httptest.NewRecorder()
		ServeSetupUI(rr, req)

		if status := rr.Code; status != http.StatusOK {
			t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
		}

		if !strings.Contains(rr.Body.String(), "<title>") && !strings.Contains(rr.Body.String(), "<html>") {
			t.Error("Response body should contain HTML")
		}
	})

	t.Run("With BASE_URL", func(t *testing.T) {
		os.Setenv("BASE_URL", "http://custom.base.url")
		defer os.Unsetenv("BASE_URL")

		req, err := http.NewRequest("GET", "/setup", nil)
		if err != nil {
			t.Fatal(err)
		}

		rr := httptest.NewRecorder()
		ServeSetupUI(rr, req)

		if status := rr.Code; status != http.StatusOK {
			t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
		}
	})

	t.Run("With X-Forwarded-Proto", func(t *testing.T) {
		req, err := http.NewRequest("GET", "/setup", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("X-Forwarded-Proto", "https")

		rr := httptest.NewRecorder()
		ServeSetupUI(rr, req)

		if status := rr.Code; status != http.StatusOK {
			t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
		}
	})

	t.Run("With TLS", func(t *testing.T) {
		req, err := http.NewRequest("GET", "/setup", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.TLS = &tls.ConnectionState{}

		rr := httptest.NewRecorder()
		ServeSetupUI(rr, req)

		if status := rr.Code; status != http.StatusOK {
			t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
		}
	})
}

func TestHandleStream(t *testing.T) {
	setupHandlers(t)

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

	if !strings.Contains(rr.Body.String(), "System stream initialized") {
		t.Error("Expected initial data in stream")
	}
}

type noFlusher struct {
	http.ResponseWriter
}

func TestHandleStreamNoFlusher(t *testing.T) {
	setupHandlers(t)

	req, err := http.NewRequest("GET", "/api/v1/stream", nil)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(req.Context())
	req = req.WithContext(ctx)
	cancel()

	rr := httptest.NewRecorder()
	nf := &noFlusher{rr}

	HandleStream(nf, req)
}
