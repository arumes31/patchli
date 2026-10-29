package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/arumes31/patchli/agent/internal/state"
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

func TestRunAgentTimeout(t *testing.T) {
	oldIdentity, oldState, oldTokens := getIdentityFunc, loadStateFunc, loadTokensFunc
	t.Cleanup(func() { getIdentityFunc, loadStateFunc, loadTokensFunc = oldIdentity, oldState, oldTokens })
	getIdentityFunc = func() (string, error) { return "fixture-node", nil }
	loadStateFunc = func() (*state.State, error) { return nil, nil }
	loadTokensFunc = func() (*TokenPair, error) { return nil, os.ErrNotExist }

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	heartbeats := make(chan HeartbeatPayload, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/poll" {
			t.Errorf("unexpected request: %s", r.URL.Path)
		}
		var hb HeartbeatPayload
		if err := json.NewDecoder(r.Body).Decode(&hb); err != nil {
			t.Error(err)
		}
		heartbeats <- hb
		w.WriteHeader(http.StatusNoContent)
		cancel()
	}))
	defer server.Close()
	allowLoopbackHTTP(t)
	t.Setenv("SERVER_URL", server.URL)

	RunAgent(ctx)
	if !errors.Is(ctx.Err(), context.Canceled) {
		t.Fatalf("agent did not exit after cancellation: %v", ctx.Err())
	}
	select {
	case hb := <-heartbeats:
		if hb.MacAddress != "fixture-node" {
			t.Fatalf("heartbeat identity = %q", hb.MacAddress)
		}
	default:
		t.Fatal("agent never sent its heartbeat")
	}
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

	allowLoopbackHTTP(t)
	startHTTPPolling(ctx, server.URL, "test-node", &mockPM{}, nil)
}

func TestExecuteCommand(t *testing.T) {
	pm := &mockPM{}
	oldSave, oldClear, oldDestruct := saveStateFunc, clearStateFunc, selfDestructFunc
	t.Cleanup(func() { saveStateFunc, clearStateFunc, selfDestructFunc = oldSave, oldClear, oldDestruct })
	var saved *state.State
	cleared, destructed := false, false
	saveStateFunc = func(value state.State) error { saved = &value; return nil }
	clearStateFunc = func() error { cleared = true; return nil }
	selfDestructFunc = func() error { destructed = true; return nil }

	t.Run("check_updates", func(t *testing.T) {
		cmd := CommandPayload{Action: "check_updates", ID: "job1"}
		executeCommand(context.Background(), pm, cmd)
	})

	t.Run("apply_updates", func(t *testing.T) {
		cmd := CommandPayload{Action: "apply_updates", ID: "job2"}
		executeCommand(context.Background(), pm, cmd)
		if saved == nil || saved.JobID != "job2" || saved.Action != "apply_updates" || saved.Status != "running" {
			t.Fatalf("incorrect saved state: %+v", saved)
		}
		if !cleared {
			t.Fatal("completed job state was not cleared")
		}
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
		cmd := CommandPayload{Action: "self_destruct", ID: "job5"}
		executeCommand(context.Background(), pm, cmd)
		if !destructed {
			t.Fatal("self-destruct command did not reach the host operation boundary")
		}
	})

	t.Run("unknown", func(t *testing.T) {
		cmd := CommandPayload{Action: "unknown", ID: "job4"}
		executeCommand(context.Background(), pm, cmd)
	})
}
