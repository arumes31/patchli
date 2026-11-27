package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/arumes31/rpatch/agent/internal/state"
	"github.com/arumes31/rpatch/agent/internal/updater"
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

func useTempResults(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	old := resultsDirFunc
	t.Cleanup(func() { resultsDirFunc = old })
	resultsDirFunc = func() string { return dir }
	return dir
}

func readResults(t *testing.T) map[string]ResultPayload {
	t.Helper()
	out := map[string]ResultPayload{}
	for _, data := range pendingResults() {
		var r ResultPayload
		if err := json.Unmarshal(data, &r); err != nil {
			t.Fatal(err)
		}
		out[r.JobID] = r
	}
	return out
}

func TestRunAgentWithoutTokensWaitsForEnrollment(t *testing.T) {
	oldIdentity, oldState, oldTokens := getIdentityFunc, loadStateFunc, loadTokensFunc
	t.Cleanup(func() { getIdentityFunc, loadStateFunc, loadTokensFunc = oldIdentity, oldState, oldTokens })
	getIdentityFunc = func() (string, error) { return "fixture-node", nil }
	loadStateFunc = func() (*state.State, error) { return nil, nil }
	loadTokensFunc = func() (*TokenPair, error) { return nil, os.ErrNotExist }

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unenrolled agent contacted the server: %s", r.URL.Path)
	}))
	defer server.Close()
	allowLoopbackHTTP(t)
	t.Setenv("SERVER_URL", server.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	RunAgent(ctx)
	if !errors.Is(ctx.Err(), context.DeadlineExceeded) {
		t.Fatalf("agent did not exit after cancellation: %v", ctx.Err())
	}
}

// TestSessionRoundTrip runs heartbeat -> signed command -> result over a real
// WebSocket, the path every patch job takes.
func TestSessionRoundTrip(t *testing.T) {
	useTempResults(t)
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	oldKey := CommandPubKey
	t.Cleanup(func() { CommandPubKey = oldKey })
	CommandPubKey = base64.StdEncoding.EncodeToString(pub)

	got := make(chan WSMessage, 4)
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		for {
			var m WSMessage
			if err := conn.ReadJSON(&m); err != nil {
				return
			}
			got <- m
			if m.Type == MsgTypeHeartbeat {
				cmd := signTestCommand(t, priv, CommandPayload{ID: "job-1", Action: "check_updates", NodeID: "node-1", ExpiresAt: time.Now().Add(time.Minute).Unix()})
				payload, _ := json.Marshal(cmd)
				_ = conn.WriteJSON(WSMessage{Type: MsgTypeCommand, Payload: payload})
			}
		}
	}))
	defer server.Close()

	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = runSession(ctx, conn, "node-1", &mockPM{}) }()

	deadline := time.After(5 * time.Second)
	var sawHeartbeat bool
	for {
		select {
		case m := <-got:
			switch m.Type {
			case MsgTypeHeartbeat:
				sawHeartbeat = true
			case MsgTypeResult:
				var r ResultPayload
				_ = json.Unmarshal(m.Payload, &r)
				if !sawHeartbeat || r.JobID != "job-1" || !r.Success {
					t.Fatalf("unexpected result %+v (heartbeat first: %v)", r, sawHeartbeat)
				}
				time.Sleep(50 * time.Millisecond)
				if !resultsEmpty() {
					t.Fatal("delivered result was not removed from the queue")
				}
				return
			}
		case <-deadline:
			t.Fatal("no result received")
		}
	}
}

func TestExecuteCommand(t *testing.T) {
	useTempResults(t)
	pm := &mockPM{}
	oldSave, oldClear, oldDestruct, oldReboot := saveStateFunc, clearStateFunc, selfDestructFunc, rebootFunc
	t.Cleanup(func() { saveStateFunc, clearStateFunc, selfDestructFunc, rebootFunc = oldSave, oldClear, oldDestruct, oldReboot })
	var saved *state.State
	cleared, destructed := false, false
	saveStateFunc = func(value state.State) error { saved = &value; return nil }
	clearStateFunc = func() error { cleared = true; return nil }
	selfDestructFunc = func() error { destructed = true; return nil }
	rebooted := make(chan struct{}, 1)
	rebootFunc = func() error { rebooted <- struct{}{}; return nil }

	oldUpdate := performSecureAgentUpdateFunc
	t.Cleanup(func() { performSecureAgentUpdateFunc = oldUpdate })
	performSecureAgentUpdateFunc = func(ctx context.Context) updater.UpdateResult { return updater.UpdateResult{Success: true} }

	for _, c := range []CommandPayload{
		{Action: "check_updates", ID: "job1"},
		{Action: "apply_updates", ID: "job2"},
		{Action: "cleanup", ID: "job3"},
		{Action: "update_agent", ID: "job4"},
		{Action: "self_destruct", ID: "job5"},
		{Action: "unknown", ID: "job6"},
		{Action: "check_updates", ID: "../escape"},
	} {
		executeCommand(context.Background(), pm, c)
	}
	if saved == nil || saved.JobID != "job2" || saved.Status != "running" || !cleared {
		t.Fatalf("apply_updates state handling: saved=%+v cleared=%v", saved, cleared)
	}
	if !destructed {
		t.Fatal("self-destruct command did not reach the host operation boundary")
	}
	results := readResults(t)
	for _, id := range []string{"job1", "job2", "job3", "job4", "job5"} {
		if !results[id].Success {
			t.Errorf("%s: want success, got %+v", id, results[id])
		}
	}
	if results["job2"].Pending != 0 || results["job1"].Pending != -1 {
		t.Errorf("pending counts: %+v %+v", results["job1"], results["job2"])
	}
	if r := results["job6"]; r.Success || r.Error == "" {
		t.Errorf("unknown action must fail: %+v", r)
	}
	if len(results) != 6 {
		t.Errorf("invalid job ID must not be stored, got %d results", len(results))
	}

	executeCommand(context.Background(), pm, CommandPayload{Action: "reboot", ID: "job7"})
	if r := readResults(t)["job7"]; !r.Success {
		t.Fatalf("reboot result: %+v", r)
	}
	select {
	case <-rebooted:
	case <-time.After(rebootDelay + 2*time.Second):
		t.Fatal("reboot was not triggered")
	}
}

func TestBusyAgentRejectsSecondJob(t *testing.T) {
	useTempResults(t)
	jobMu.Lock()
	executeCommand(context.Background(), &mockPM{}, CommandPayload{Action: "check_updates", ID: "busy-1"})
	jobMu.Unlock()
	if r := readResults(t)["busy-1"]; r.Success || !strings.Contains(r.Error, "busy") {
		t.Fatalf("want busy failure, got %+v", r)
	}
}

func TestInterruptedJobIsReported(t *testing.T) {
	useTempResults(t)
	oldClear := clearStateFunc
	t.Cleanup(func() { clearStateFunc = oldClear })
	clearStateFunc = func() error { return nil }
	reportInterruptedJob(&state.State{JobID: "job-int", Action: "apply_updates", Status: "running"})
	if r := readResults(t)["job-int"]; r.Success || !strings.Contains(r.Error, "interrupted") {
		t.Fatalf("got %+v", r)
	}
}

func TestRemoteScriptsAreNeverExecuted(t *testing.T) {
	useTempResults(t)
	marker := t.TempDir() + "/ran"
	oldSave, oldClear := saveStateFunc, clearStateFunc
	t.Cleanup(func() { saveStateFunc, clearStateFunc = oldSave, oldClear })
	saveStateFunc = func(state.State) error { return nil }
	clearStateFunc = func() error { return nil }
	script := "touch " + marker
	executeCommand(context.Background(), &mockPM{}, CommandPayload{ID: "job-s", Action: "apply_updates",
		PrePatchScript: script, PostPatchScript: script, HealthCheckCommand: script})
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("a script from the command was executed")
	}
	if r := readResults(t)["job-s"]; !r.Success {
		t.Fatalf("patch with ignored scripts must still succeed: %+v", r)
	}
}
