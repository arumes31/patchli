//go:build integration

package patching

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/arumes31/rpatch/server/internal/db"
	"github.com/arumes31/rpatch/server/internal/models"
	"github.com/arumes31/rpatch/server/internal/webhooks"
)

// testDB migrates a fresh schema of the disposable test database.
func testDB(t *testing.T) {
	t.Helper()
	raw := os.Getenv("RPATCH_MIGRATION_TEST_URL")
	if raw == "" {
		t.Skip("RPATCH_MIGRATION_TEST_URL is not set")
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1") {
		t.Fatal("integration tests require a local disposable PostgreSQL instance")
	}
	admin, err := sql.Open("pgx", raw)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	schema := fmt.Sprintf("patching_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec("CREATE SCHEMA " + schema); err != nil {
		t.Fatal(err)
	}
	previous := db.DB
	t.Cleanup(func() {
		if db.DB != nil && db.DB != previous {
			_ = db.DB.Close()
		}
		db.DB = previous
		_, _ = admin.Exec("DROP SCHEMA " + schema + " CASCADE")
	})
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	if err := db.Init(u.String()); err != nil {
		t.Fatal(err)
	}
}

type fakeFleet struct {
	mu      sync.Mutex
	online  map[string]bool
	sent    []string
	respond func(node string, cmd models.CommandPayload)
	notes   []webhooks.WebhookPayload
}

func installFake(t *testing.T, f *fakeFleet) {
	t.Helper()
	oldSend, oldOnline, oldNotify, oldPoll, oldStart := sendCommand, isOnline, notify, pollInterval, startedAt
	t.Cleanup(func() { sendCommand, isOnline, notify, pollInterval, startedAt = oldSend, oldOnline, oldNotify, oldPoll, oldStart })
	pollInterval = 10 * time.Millisecond
	startedAt = time.Now().Add(-time.Hour)
	isOnline = func(id string) bool { f.mu.Lock(); defer f.mu.Unlock(); return f.online[id] }
	notify = func(p webhooks.WebhookPayload) { f.mu.Lock(); f.notes = append(f.notes, p); f.mu.Unlock() }
	sendCommand = func(node string, cmd models.CommandPayload) error {
		f.mu.Lock()
		f.sent = append(f.sent, node+":"+cmd.Action)
		respond := f.respond
		f.mu.Unlock()
		if respond != nil {
			go respond(node, cmd)
		}
		return nil
	}
}

func enroll(t *testing.T, id, host, boot string, slot int) {
	t.Helper()
	if err := db.EnrollNode(id, "nonce-"+id, time.Now().Add(10*time.Minute), "tok-"+id, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := db.UpdateHeartbeat(id, host, "linux", "Debian", "6.1", boot, "test", false); err != nil {
		t.Fatal(err)
	}
	if err := db.SetNodePolicy(id, true, false, slot); err != nil {
		t.Fatal(err)
	}
}

func lastNote(f *fakeFleet, event string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := len(f.notes) - 1; i >= 0; i-- {
		if f.notes[i].Event == event {
			return f.notes[i].Message
		}
	}
	return ""
}

func TestWindowPatchesRebootsAndSkips(t *testing.T) {
	testDB(t)
	enroll(t, "node-a", "alpha", "boot-a1", 10) // needs a reboot
	enroll(t, "node-b", "bravo", "boot-b1", 20) // just patches
	enroll(t, "node-c", "charlie", "boot-c1", 30)

	f := &fakeFleet{online: map[string]bool{"node-a": true, "node-b": true}}
	f.respond = func(node string, cmd models.CommandPayload) {
		switch cmd.Action {
		case "apply_updates":
			HandleResult(node, Result{JobID: cmd.ID, Action: cmd.Action, Success: true, Output: "upgraded", RebootNeeded: node == "node-a", Pending: 0})
		case "reboot":
			HandleResult(node, Result{JobID: cmd.ID, Action: cmd.Action, Success: true, RebootNeeded: true, Pending: -1})
			// came back with a new boot ID (what the WebSocket handler does per heartbeat)
			HandleHeartbeat(node, "boot-a2")
			_ = db.UpdateHeartbeat(node, "alpha", "linux", "Debian", "6.2", "boot-a2", "test", false)
		}
	}
	installFake(t, f)

	Default.run(context.Background(), true)

	if got := strings.Join(f.sent, ","); got != "node-a:apply_updates,node-a:reboot,node-b:apply_updates" {
		t.Fatalf("commands sent: %s", got)
	}
	msg := lastNote(f, "window_summary")
	if !strings.Contains(msg, "1 ok, 1 rebooted, 0 failed, 1 skipped") || !strings.Contains(msg, "charlie (offline)") {
		t.Fatalf("summary: %q", msg)
	}
	jobs, err := db.ListJobs("node-a", 10)
	if err != nil || len(jobs) != 2 || jobs[0].State != "ok" || jobs[0].Action != "reboot" {
		t.Fatalf("node-a jobs: %+v %v", jobs, err)
	}
	if st := Default.Status(); st.Running {
		t.Fatalf("status after run: %+v", st)
	}

	// The same window again (e.g. after the control plane rebooted itself):
	// nothing is re-patched, the hosts count as done.
	if err := db.SaveSettings(db.Settings{WindowStart: "00:00", WindowEnd: "23:59", Timezone: "UTC", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	f.sent = nil
	f.mu.Unlock()
	Default.run(context.Background(), false)
	if len(f.sent) != 0 {
		t.Fatalf("resumed window re-sent commands: %v", f.sent)
	}
}

func TestWindowStopsAfterTwoFailures(t *testing.T) {
	testDB(t)
	for i, id := range []string{"n1", "n2", "n3"} {
		enroll(t, id, "host-"+id, "boot", i)
	}
	f := &fakeFleet{online: map[string]bool{"n1": true, "n2": true, "n3": true}}
	f.respond = func(node string, cmd models.CommandPayload) {
		HandleResult(node, Result{JobID: cmd.ID, Action: cmd.Action, Error: "dpkg was interrupted", Pending: -1})
	}
	installFake(t, f)

	Default.run(context.Background(), true)

	if len(f.sent) != 2 {
		t.Fatalf("window continued after two failures: %v", f.sent)
	}
	if lastNote(f, "window_paused") == "" || lastNote(f, "job_failed") == "" {
		t.Fatalf("missing failure notifications: %+v", f.notes)
	}
	if msg := lastNote(f, "window_summary"); !strings.Contains(msg, "2 failed") || !strings.Contains(msg, "stopped after failures") {
		t.Fatalf("summary: %q", msg)
	}
}

func TestDispatchBusyAndWaitTimeout(t *testing.T) {
	testDB(t)
	enroll(t, "n1", "host", "boot", 1)
	f := &fakeFleet{online: map[string]bool{"n1": true}} // agent never answers
	installFake(t, f)

	id, err := Dispatch("n1", "check_updates", "manual")
	if err != nil {
		t.Fatal(err)
	}
	if again, err := Dispatch("n1", "reboot", "manual"); err != ErrBusy || again != id {
		t.Fatalf("second job: %q %v", again, err)
	}
	j, err := Wait(context.Background(), id, 50*time.Millisecond)
	if err != nil || j.State != "failed" || !strings.Contains(j.Error, "timeout") {
		t.Fatalf("Wait: %+v %v", j, err)
	}
	// A late result from another node is ignored.
	HandleResult("intruder", Result{JobID: id, Success: true})
	if j, _ := db.GetJob(id); j.State != "failed" {
		t.Fatalf("foreign result changed the job: %+v", j)
	}
}

func TestSchedulerRunNow(t *testing.T) {
	testDB(t)
	f := &fakeFleet{online: map[string]bool{}}
	installFake(t, f)
	s := &Scheduler{runNow: make(chan struct{}, 1)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.Start(ctx)
	if !s.RunNow() {
		t.Fatal("RunNow refused")
	}
	deadline := time.Now().Add(5 * time.Second)
	for lastNote(f, "window_summary") == "" {
		if time.Now().After(deadline) {
			t.Fatal("manual window did not run")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if st := s.Status(); st.NextRun == "" {
		t.Fatalf("status without next run: %+v", st)
	}
}
