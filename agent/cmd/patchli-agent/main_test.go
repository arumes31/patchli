package main

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/arumes31/patchli/agent/internal/control"
	"github.com/arumes31/patchli/agent/internal/identity"
	"github.com/arumes31/patchli/agent/internal/updater"
)

type mockPM struct{}

func (m *mockPM) CheckUpdates(ctx context.Context) (updater.UpdateResult, error) {
	return updater.UpdateResult{Success: true}, nil
}
func (m *mockPM) ApplyUpdates(ctx context.Context, packages []string) (updater.UpdateResult, error) {
	return updater.UpdateResult{Success: true}, nil
}
func (m *mockPM) RebootRequired() bool                     { return false }
func (m *mockPM) PreFlightCheck(ctx context.Context) error { return nil }
func (m *mockPM) Cleanup(ctx context.Context) error        { return nil }

func TestGetOrGenerateIdentity(t *testing.T) {
	// identity.GetOrGenerate uses a hardcoded path or env var.
	// Since we can't easily change the path in identity package without refactoring,
	// we just test it doesn't crash and returns something.
	id1, err := identity.GetOrGenerate()
	if err != nil {
		t.Fatalf("Identity GetOrGenerate returned error: %v", err)
	}
	if id1 == "" {
		t.Fatal("Identity should not be empty")
	}

	id2, err := identity.GetOrGenerate()
	if err != nil {
		t.Fatalf("Identity GetOrGenerate returned error: %v", err)
	}
	if id1 != id2 {
		t.Errorf("Identity changed: %s vs %s", id1, id2)
	}
}

func TestRunAgentRequiresEnrollment(t *testing.T) {
	t.Setenv("SERVER_URL", "https://127.0.0.1:1")

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	if err := RunAgent(ctx); err == nil {
		t.Fatal("RunAgent accepted missing enrollment credentials")
	}
}

func TestRegisterAgentUsesVerifiedHTTPS(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/auth/login" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		var request map[string]string
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request["mac"] != "node-1" || request["group"] != "production" {
			t.Errorf("unexpected registration request: %#v", request)
		}
		_ = json.NewEncoder(w).Encode(TokenPair{AccessToken: "access", RefreshToken: "refresh"})
	}))
	defer server.Close()

	caFile := filepath.Join(t.TempDir(), "ca.pem")
	encoded := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	if err := os.WriteFile(caFile, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	values := map[string]string{
		"SERVER_URL":             server.URL,
		"SERVER_CA_FILE":         caFile,
		"REGISTRATION_GROUP":     "production",
		"REGISTRATION_TIMESTAMP": "2026-08-28T00:00:00Z",
		"REGISTRATION_SIGNATURE": "signature",
	}
	getenv := func(key string) string { return values[key] }
	controlClient, err := control.FromEnvironment(getenv)
	if err != nil {
		t.Fatal(err)
	}
	pair, err := registerAgent(context.Background(), controlClient, "node-1", getenv)
	if err != nil {
		t.Fatal(err)
	}
	if pair.AccessToken != "access" || pair.RefreshToken != "refresh" {
		t.Fatalf("unexpected tokens: %#v", pair)
	}
}

func TestHTTPPolling(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/poll" {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(CommandPayload{
				Action: "check_updates",
				ID:     "job1",
			})
		}
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	caFile := filepath.Join(t.TempDir(), "ca.pem")
	encoded := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	if err := os.WriteFile(caFile, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	controlClient, err := control.FromEnvironment(func(key string) string {
		if key == "SERVER_URL" {
			return server.URL
		}
		if key == "SERVER_CA_FILE" {
			return caFile
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}
	startHTTPPolling(ctx, controlClient, "test-node", &mockPM{}, nil)
}

func TestExecuteCommand(t *testing.T) {
	pm := &mockPM{}

	t.Run("check_updates", func(t *testing.T) {
		cmd := CommandPayload{Action: "check_updates", ID: "job1"}
		executeCommand(context.Background(), pm, cmd)
	})

	t.Run("apply_updates", func(t *testing.T) {
		cmd := CommandPayload{Action: "apply_updates", ID: "job2"}
		executeCommand(context.Background(), pm, cmd)
	})

	t.Run("cleanup", func(t *testing.T) {
		cmd := CommandPayload{Action: "cleanup", ID: "job3"}
		executeCommand(context.Background(), pm, cmd)
	})

	t.Run("update_agent", func(t *testing.T) {
		oldUpdate := performSecureAgentUpdateFunc
		defer func() { performSecureAgentUpdateFunc = oldUpdate }()
		performSecureAgentUpdateFunc = func(ctx context.Context) updater.UpdateResult {
			return updater.UpdateResult{Success: true}
		}
		cmd := CommandPayload{Action: "update_agent", ID: "job4"}
		executeCommand(context.Background(), pm, cmd)
	})

	t.Run("self_destruct", func(t *testing.T) {
		// Mocking geteuidFunc to avoid permission issues
		// However, executeCommand calls updater.SelfDestruct() which uses package level vars
		// We'd need to mock them in the updater package.
		cmd := CommandPayload{Action: "self_destruct", ID: "job5"}
		executeCommand(context.Background(), pm, cmd)
	})

	t.Run("unknown", func(t *testing.T) {
		cmd := CommandPayload{Action: "unknown", ID: "job4"}
		executeCommand(context.Background(), pm, cmd)
	})

	t.Run("remote scripts disabled by default", func(t *testing.T) {
		t.Setenv("ALLOW_REMOTE_SCRIPTS", "")
		marker := filepath.Join(t.TempDir(), "executed")
		script := "touch '" + marker + "'"
		if runtime.GOOS == "windows" {
			script = "type nul > \"" + marker + "\""
		}
		executeCommand(context.Background(), pm, CommandPayload{Action: "apply_updates", ID: "script-job", PrePatchScript: script})
		if _, err := os.Stat(marker); !os.IsNotExist(err) {
			t.Fatal("remote script executed without explicit opt-in")
		}
	})
}
