package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/arumes31/rpatch/agent/internal/state"
	"github.com/arumes31/rpatch/agent/internal/updater"
)

// ResultPayload reports the outcome of one job to the controller.
type ResultPayload struct {
	JobID        string `json:"job_id"`
	Action       string `json:"action"`
	Success      bool   `json:"success"`
	Output       string `json:"output,omitempty"`
	Error        string `json:"error,omitempty"`
	RebootNeeded bool   `json:"reboot_needed"`
	// Pending is the number of upgradable packages after the job, -1 if unknown.
	Pending int `json:"pending"`
}

const (
	// maxResultOutput keeps a result well under the controller's 64 KiB
	// WebSocket read limit.
	maxResultOutput = 16 << 10
	rebootDelay     = 10 * time.Second
)

var (
	// jobMu serializes jobs: two package-manager runs must never overlap.
	jobMu sync.Mutex
	// resultsReady wakes the connection writer when a result was queued.
	resultsReady = make(chan struct{}, 1)
	// resultsDirFunc is where results wait until the controller received
	// them, so a result survives a dropped connection or an agent restart.
	resultsDirFunc = func() string { return filepath.Join(getDataDir(), "results") }
	rebootFunc     = func() error { return exec.Command("reboot").Run() }
	validJobID     = regexp.MustCompile(`^[A-Za-z0-9-]{1,64}$`)
)

func actionTimeout(action string) time.Duration {
	switch action {
	case "apply_updates":
		return 90 * time.Minute
	case "check_updates":
		return 20 * time.Minute
	default:
		return 10 * time.Minute
	}
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n:]
}

// queueResult persists r and wakes the writer.
func queueResult(r ResultPayload) {
	r.Output = tail(r.Output, maxResultOutput)
	if !validJobID.MatchString(r.JobID) {
		log.Printf("Not reporting result with invalid job ID %q", r.JobID)
		return
	}
	data, err := json.Marshal(r)
	if err != nil {
		return
	}
	dir := resultsDirFunc()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		log.Printf("Cannot store result for job %q: %v", r.JobID, err)
		return
	}
	if err := os.WriteFile(filepath.Join(dir, r.JobID+".json"), data, 0o600); err != nil {
		log.Printf("Cannot store result for job %q: %v", r.JobID, err)
		return
	}
	select {
	case resultsReady <- struct{}{}:
	default:
	}
}

// pendingResults returns stored results, oldest first.
func pendingResults() map[string][]byte {
	dir := resultsDirFunc()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	sort.Slice(entries, func(i, j int) bool {
		a, _ := entries[i].Info()
		b, _ := entries[j].Info()
		return a != nil && b != nil && a.ModTime().Before(b.ModTime())
	})
	out := make(map[string][]byte, len(entries))
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name())) // #nosec G304 -- file names come from our own results directory.
		if err == nil {
			out[filepath.Join(dir, e.Name())] = data
		}
	}
	return out
}

func resultsEmpty() bool { return len(pendingResults()) == 0 }

// reportInterruptedJob turns a job that was running when the agent stopped
// into a failed result.
func reportInterruptedJob(last *state.State) {
	if last == nil || last.Status != "running" {
		return
	}
	log.Printf("RECOVERY: job %q was interrupted; reporting it as failed", last.JobID)
	queueResult(ResultPayload{JobID: last.JobID, Action: last.Action, Error: "interrupted: the agent restarted during the job", Pending: -1})
	_ = clearStateFunc()
}

func executeCommand(ctx context.Context, pm updater.PackageManager, cmd CommandPayload) {
	if !jobMu.TryLock() {
		queueResult(ResultPayload{JobID: cmd.ID, Action: cmd.Action, Error: "busy: another job is running", Pending: -1})
		return
	}
	defer jobMu.Unlock()

	ctx, cancel := context.WithTimeout(ctx, actionTimeout(cmd.Action))
	defer cancel()

	res, err := runAction(ctx, pm, cmd)
	if cmd.Action == "chaos_restart" {
		return
	}
	r := ResultPayload{
		JobID:        cmd.ID,
		Action:       cmd.Action,
		Success:      err == nil && res.Success,
		Output:       res.Output,
		RebootNeeded: pm.RebootRequired(),
		Pending:      -1,
	}
	if err != nil {
		r.Error = err.Error()
	}
	if cmd.Action == "check_updates" && r.Success {
		r.Pending = updater.CountPending(pm, res.Output)
	}
	if cmd.Action == "apply_updates" && r.Success {
		r.Pending = 0
	}
	log.Printf("Job %q (%s) finished. Success: %v. Error: %q", cmd.ID, cmd.Action, r.Success, r.Error)
	queueResult(r)

	if cmd.Action == "reboot" && r.Success {
		go rebootSoon()
	}
}

// rebootSoon gives the writer a moment to deliver the result, then reboots.
func rebootSoon() {
	deadline := time.Now().Add(rebootDelay)
	for time.Now().Before(deadline) && !resultsEmpty() {
		time.Sleep(500 * time.Millisecond)
	}
	log.Println("Rebooting the host as requested by the controller")
	if err := rebootFunc(); err != nil {
		log.Printf("Reboot failed: %v", err)
	}
}

func runAction(ctx context.Context, pm updater.PackageManager, cmd CommandPayload) (updater.UpdateResult, error) {
	switch cmd.Action {
	case "check_updates":
		return pm.CheckUpdates(ctx)
	case "apply_updates":
		return applyUpdates(ctx, pm, cmd)
	case "reboot":
		return updater.UpdateResult{Success: true, Output: "rebooting"}, nil
	case "cleanup":
		err := pm.Cleanup(ctx)
		return updater.UpdateResult{Success: err == nil}, err
	case "self_destruct":
		err := selfDestructFunc()
		return updater.UpdateResult{Success: err == nil}, err
	case "update_agent":
		log.Printf("Auto-updating agent...")
		res := performSecureAgentUpdateFunc(ctx)
		return res, res.Error
	case "chaos_restart":
		log.Printf("CHAOS MONKEY: Restarting agent...")
		os.Exit(1)
	}
	return updater.UpdateResult{}, fmt.Errorf("unknown action %q", cmd.Action)
}

// applyUpdates runs the package manager only. Scripts in the command
// (pre/post-patch, health check) are ignored: the agent never executes shell
// code received from the network.
func applyUpdates(ctx context.Context, pm updater.PackageManager, cmd CommandPayload) (updater.UpdateResult, error) {
	if cmd.PrePatchScript != "" || cmd.PostPatchScript != "" || cmd.HealthCheckCommand != "" {
		log.Printf("Job %q: ignoring scripts; the agent does not run remote shell code", cmd.ID)
	}
	if err := pm.PreFlightCheck(ctx); err != nil {
		return updater.UpdateResult{}, fmt.Errorf("pre-flight check failed: %w", err)
	}
	if err := saveStateFunc(state.State{JobID: cmd.ID, Action: cmd.Action, Status: "running"}); err != nil {
		log.Printf("Failed to save state: %v", err)
	}
	defer func() { _ = clearStateFunc() }()

	res, err := pm.ApplyUpdates(ctx, cmd.Packages)
	if err == nil && !res.Success {
		err = errors.New("package manager reported failure")
	}
	return res, err
}

// hostInfo reads OS release, kernel and boot ID. The boot ID changes on
// every boot, which is how the controller confirms that a reboot happened.
func hostInfo() (osVersion, kernel, bootID string) {
	read := func(p string) string {
		b, err := os.ReadFile(p) // #nosec G304 -- fixed system paths.
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(b))
	}
	for _, line := range strings.Split(read("/etc/os-release"), "\n") {
		if v, ok := strings.CutPrefix(line, "PRETTY_NAME="); ok {
			osVersion = strings.Trim(v, `"`)
		}
	}
	return osVersion, read("/proc/sys/kernel/osrelease"), read("/proc/sys/kernel/random/boot_id")
}
