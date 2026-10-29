package api

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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
			Vitality int    `json:"vitality"`
			Immune   string `json:"immune"`
			Recovery int    `json:"recovery"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &stats); err != nil {
			t.Fatal(err)
		}
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
			Vitality int    `json:"vitality"`
			Immune   string `json:"immune"`
			Recovery int    `json:"recovery"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &stats); err != nil {
			t.Fatal(err)
		}

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
		name           string
		query          string
		header         map[string]string
		env            map[string]string
		host           string
		useTLS         bool
		expectedInBody []string
		expectedStatus int
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
			expectedInBody: []string{"GROUP='prod'", "#!/bin/bash", "SERVER_URL=", "NONCE='"},
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
			name:           "Remote HTTP BASE_URL rejected",
			query:          "group=test",
			env:            map[string]string{"BASE_URL": "http://patchli.example.com"},
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name:           "Remote HTTP host rejected",
			query:          "group=test",
			host:           "patchli.example.com",
			expectedStatus: http.StatusInternalServerError,
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
				t.Setenv(k, v)
			}

			// Plain HTTP setup origins are only accepted for loopback hosts.
			req := httptest.NewRequest("GET", "http://localhost/api/v1/setup?"+tt.query, nil)
			if tt.host != "" {
				req.Host = tt.host
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
		req := httptest.NewRequest("GET", "http://localhost/setup", nil)

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
		t.Setenv("BASE_URL", "https://custom.base.url")

		req := httptest.NewRequest("GET", "/setup", nil)

		rr := httptest.NewRecorder()
		ServeSetupUI(rr, req)

		if status := rr.Code; status != http.StatusOK {
			t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
		}
		if body := rr.Body.String(); !strings.Contains(body, "--proto") {
			t.Error("HTTPS install command should pin the protocol")
		}
	})

	t.Run("Rejects remote HTTP BASE_URL", func(t *testing.T) {
		t.Setenv("BASE_URL", "http://custom.base.url")

		rr := httptest.NewRecorder()
		ServeSetupUI(rr, httptest.NewRequest("GET", "/setup", nil))

		if status := rr.Code; status != http.StatusInternalServerError {
			t.Errorf("remote HTTP origin accepted: got %v", status)
		}
	})

	t.Run("With X-Forwarded-Proto", func(t *testing.T) {
		req := httptest.NewRequest("GET", "http://localhost/setup", nil)
		req.Header.Set("X-Forwarded-Proto", "https")

		rr := httptest.NewRecorder()
		ServeSetupUI(rr, req)

		if status := rr.Code; status != http.StatusOK {
			t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
		}
	})

	t.Run("With TLS", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/setup", nil)
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

func TestSetupURLTrust(t *testing.T) {
	for _, tc := range []struct {
		name, base, proxy, remote, proto, want string
		status                                 int
	}{
		{"untrusted forwarded header", "", "192.0.2.1", "192.0.2.2:1234", "https", "Invalid server configuration", http.StatusInternalServerError},
		{"trusted forwarded header", "", "192.0.2.1", "192.0.2.1:1234", "HTTPS", "https://controller.example", http.StatusOK},
		{"trusted without port", "", "192.0.2.1", "192.0.2.1", "https", "https://controller.example", http.StatusOK},
		{"trusted unsupported protocol", "", "192.0.2.1", "192.0.2.1:1234", "ftp", "Invalid server configuration", http.StatusInternalServerError},
		{"loopback http base", "http://127.0.0.1:8080", "", "", "", "http://127.0.0.1:8080", http.StatusOK},
		{"ipv6 loopback http base", "http://[::1]:8080", "", "", "", "http://[::1]:8080", http.StatusOK},
		{"remote http base", "http://controller.example", "", "", "", "Invalid server configuration", http.StatusInternalServerError},
		{"invalid scheme", "javascript:alert(1)", "", "", "", "Invalid server configuration", http.StatusInternalServerError},
		{"malformed url", "http://%", "", "", "", "Invalid server configuration", http.StatusInternalServerError},
		{"injected host header", "", "", "", "", "Invalid server configuration", http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("BASE_URL", tc.base)
			t.Setenv("TRUSTED_PROXY", tc.proxy)
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "http://controller.example/setup", nil)
			if tc.name == "injected host header" {
				req.Host = `evil.example/"><script>alert(1)</script>`
			}
			req.RemoteAddr = tc.remote
			req.Header.Set("X-Forwarded-Proto", tc.proto)
			ServeSetupUI(rr, req)
			if rr.Code != tc.status || !strings.Contains(rr.Body.String(), tc.want) {
				t.Fatalf("status=%d want=%d expected body to contain %q", rr.Code, tc.status, tc.want)
			}
		})
	}
}

type failingResponseWriter struct {
	header http.Header
	writes int
}

func (w *failingResponseWriter) Header() http.Header { return w.header }
func (w *failingResponseWriter) WriteHeader(int)     {}
func (w *failingResponseWriter) Write([]byte) (int, error) {
	w.writes++
	return 0, fmt.Errorf("client disconnected")
}

func TestHandlersDisconnectedClient(t *testing.T) {
	for _, tc := range []struct {
		name    string
		handler http.HandlerFunc
	}{
		{"nodes", HandleNodes}, {"stats", HandleStats}, {"setup", HandleSetup}, {"setup UI", ServeSetupUI}, {"stream", HandleStream},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := &failingResponseWriter{header: make(http.Header)}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			req := httptest.NewRequest(http.MethodGet, "http://localhost/", nil).WithContext(ctx)
			tc.handler(w, req)
			if w.writes == 0 {
				t.Fatal("handler did not attempt response")
			}
		})
	}
}
