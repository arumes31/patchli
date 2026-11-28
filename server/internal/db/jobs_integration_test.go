//go:build integration

package db

import (
	"testing"
	"time"
)

// The TestMigrations prefix keeps this in the CI integration run.
func TestMigrationsPatchJobs(t *testing.T) {
	conn, m := migrationDatabase(t)
	if err := m.Up(); err != nil {
		t.Fatal(err)
	}
	previous := DB
	DB = conn
	t.Cleanup(func() { DB = previous })

	const node = "0b5e2c1a-7d4f-4e2b-9c3a-1f2e3d4c5b6a"
	if err := EnrollNode(node, "jobs-nonce", time.Now().Add(10*time.Minute), "tok", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := UpdateHeartbeat(node, "host-a", "linux", "Debian GNU/Linux 12 (bookworm)", "6.1.0", "boot-1", "v1", false); err != nil {
		t.Fatal(err)
	}
	nodes, err := ListNodes()
	if err != nil || len(nodes) != 1 || nodes[0].BootID != "boot-1" || nodes[0].PendingUpdates != -1 || !nodes[0].AutoReboot {
		t.Fatalf("ListNodes: %+v %v", nodes, err)
	}

	// check -> ok with a pending count
	id, err := CreateJob(node, "check_updates", "manual", "boot-1")
	if err != nil {
		t.Fatal(err)
	}
	if open, _ := OpenJob(node); open != id {
		t.Fatalf("OpenJob = %q want %q", open, id)
	}
	if err := RecordResult(node, id, "check_updates", true, "", "Inst foo", false, 3); err != nil {
		t.Fatal(err)
	}
	if j, _ := GetJob(id); j.State != "ok" || j.FinishedAt == nil || j.Output != "Inst foo" {
		t.Fatalf("check job: %+v", j)
	}
	if n, _ := GetNode(node); n.PendingUpdates != 3 || n.LastCheckAt == nil || n.LastPatchAt != nil {
		t.Fatalf("after check: %+v", n)
	}
	if err := RecordResult("other-node", id, "check_updates", false, "x", "", false, 0); err == nil {
		t.Fatal("a node recorded a result for another node's job")
	}

	// patch -> ok, reboot needed
	id, _ = CreateJob(node, "apply_updates", "schedule", "boot-1")
	if err := RecordResult(node, id, "apply_updates", true, "", "done", true, 0); err != nil {
		t.Fatal(err)
	}
	if n, _ := GetNode(node); n.PendingUpdates != 0 || !n.RebootRequired || n.LastPatchAt == nil {
		t.Fatalf("after patch: %+v", n)
	}

	// reboot -> rebooting -> ok once the boot ID changes
	id, _ = CreateJob(node, "reboot", "schedule", "boot-1")
	if err := RecordResult(node, id, "reboot", true, "", "rebooting", true, -1); err != nil {
		t.Fatal(err)
	}
	if j, _ := GetJob(id); j.State != "rebooting" || j.FinishedAt != nil {
		t.Fatalf("reboot job before return: %+v", j)
	}
	_ = ConfirmReboots(node, "boot-1")
	if j, _ := GetJob(id); j.State != "rebooting" {
		t.Fatal("same boot ID confirmed a reboot")
	}
	_ = ConfirmReboots(node, "boot-2")
	if j, _ := GetJob(id); j.State != "ok" || j.FinishedAt == nil {
		t.Fatalf("reboot job after return: %+v", j)
	}

	// failure, expiry and listing
	id, _ = CreateJob(node, "apply_updates", "manual", "boot-2")
	_ = FailJob(id, "could not send")
	if j, _ := GetJob(id); j.State != "failed" || j.Error != "could not send" {
		t.Fatalf("FailJob: %+v", j)
	}
	stale, _ := CreateJob(node, "reboot", "manual", "boot-2")
	if _, err := conn.Exec(`UPDATE jobs SET created_at = now() - interval '1 hour' WHERE id = $1`, stale); err != nil {
		t.Fatal(err)
	}
	if err := ExpireJobs(); err != nil {
		t.Fatal(err)
	}
	if j, _ := GetJob(stale); j.State != "failed" {
		t.Fatalf("stale reboot not expired: %+v", j)
	}
	if jobs, err := ListJobs(node, 10); err != nil || len(jobs) != 5 || jobs[4].ID != stale {
		t.Fatalf("ListJobs: %d %v", len(jobs), err)
	}
	if jobs, err := ListJobs("", 2); err != nil || len(jobs) != 2 {
		t.Fatalf("ListJobs all: %d %v", len(jobs), err)
	}

	// policy and settings
	if err := SetNodePolicy(node, false, true, 999); err != nil {
		t.Fatal(err)
	}
	if n, _ := GetNode(node); n.AutoReboot || !n.Excluded || n.Slot != 999 {
		t.Fatalf("policy: %+v", n)
	}
	if err := SetNodePolicy("missing", true, false, 1); err == nil {
		t.Fatal("policy for unknown node accepted")
	}
	s, err := GetSettings()
	if err != nil || s.WindowStart != "03:00" || s.WindowEnd != "05:30" || s.Timezone != "Europe/Vienna" || !s.Enabled || s.LastRunDate != nil {
		t.Fatalf("default settings: %+v %v", s, err)
	}
	s.WindowStart, s.WindowEnd = "02:15", "04:45"
	if err := SaveSettings(s); err != nil {
		t.Fatal(err)
	}
	if err := MarkWindowDone("2026-11-28"); err != nil {
		t.Fatal(err)
	}
	if s, _ = GetSettings(); s.WindowStart != "02:15" || s.LastRunDate == nil || *s.LastRunDate != "2026-11-28" {
		t.Fatalf("saved settings: %+v", s)
	}
}
