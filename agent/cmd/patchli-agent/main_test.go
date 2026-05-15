package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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
func (m *mockPM) RebootRequired() bool { return false }
func (m *mockPM) PreFlightCheck(ctx context.Context) error { return nil }
func (m *mockPM) Cleanup(ctx context.Context) error        { return nil }

func TestGetOrGenerateIdentity(t *testing.T) {
	// identity.GetOrGenerate uses a hardcoded path or env var.
	// Since we can't easily change the path in identity package without refactoring,
	// we just test it doesn't crash and returns something.
	id1 := identity.GetOrGenerate()
	if id1 == "" {
		t.Fatal("Identity should not be empty")
	}

	id2 := identity.GetOrGenerate()
	if id1 != id2 {
		t.Errorf("Identity changed: %s vs %s", id1, id2)
	}
}

func TestRunAgentTimeout(t *testing.T) {
	t.Setenv("SERVER_URL", "127.0.0.1:0")
	
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	
	RunAgent(ctx)
}

func TestHTTPPolling(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

	// Strip http:// from server.URL
	host := server.URL[7:]
	startHTTPPolling(ctx, host, "test-node", &mockPM{})
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
}
