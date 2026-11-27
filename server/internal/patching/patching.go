// Package patching sends jobs to agents, records their results and runs the
// nightly patch window: one host at a time, patch, then reboot if required.
package patching

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/arumes31/rpatch/server/internal/db"
	"github.com/arumes31/rpatch/server/internal/fleet"
	"github.com/arumes31/rpatch/server/internal/models"
	"github.com/arumes31/rpatch/server/internal/webhooks"
)

// Actions an operator or the scheduler may start.
var Actions = map[string]bool{"check_updates": true, "apply_updates": true, "reboot": true}

var (
	ErrOffline = errors.New("node is offline")
	ErrBusy    = errors.New("node already has a running job")
)

// Test seams.
var (
	sendCommand  = fleet.Registry.SendCommand
	isOnline     = fleet.Registry.IsOnline
	notify       = webhooks.NotifyWebhooks
	pollInterval = 3 * time.Second
	now          = time.Now
	startedAt    = time.Now()
)

const (
	patchTimeout  = 95 * time.Minute
	rebootTimeout = 15 * time.Minute
	// reconnectGrace lets agents reconnect after a control-plane restart
	// before the window treats them as offline.
	reconnectGrace = 3 * time.Minute
)

// Dispatch records a job and sends it to the node.
func Dispatch(nodeID, action, trigger string) (string, error) {
	if !Actions[action] {
		return "", fmt.Errorf("unknown action %q", action)
	}
	if !isOnline(nodeID) {
		return "", ErrOffline
	}
	if open, err := db.OpenJob(nodeID); err != nil {
		return "", err
	} else if open != "" {
		return open, ErrBusy
	}
	node, err := db.GetNode(nodeID)
	if err != nil {
		return "", err
	}
	id, err := db.CreateJob(nodeID, action, trigger, node.BootID)
	if err != nil {
		return "", err
	}
	if err := sendCommand(nodeID, models.CommandPayload{ID: id, Action: action}); err != nil {
		_ = db.FailJob(id, "could not send to agent: "+err.Error())
		return id, err
	}
	log.Printf("Job %s: %s sent to %s (%s)", id, action, node.Hostname, trigger)
	return id, nil
}

// Wait polls until the job finished or timeout passed; a timed-out job is
// marked failed.
func Wait(ctx context.Context, id string, timeout time.Duration) (db.Job, error) {
	deadline := now().Add(timeout)
	for {
		j, err := db.GetJob(id)
		if err == nil && (j.State == "ok" || j.State == "failed") {
			return j, nil
		}
		if now().After(deadline) {
			_ = db.FailJob(id, "timeout: no result within "+timeout.String())
			j, _ = db.GetJob(id)
			return j, nil
		}
		select {
		case <-ctx.Done():
			return j, ctx.Err()
		case <-time.After(pollInterval):
		}
	}
}

// Result is an agent's job report (see the agent's ResultPayload).
type Result struct {
	JobID        string `json:"job_id"`
	Action       string `json:"action"`
	Success      bool   `json:"success"`
	Output       string `json:"output"`
	Error        string `json:"error"`
	RebootNeeded bool   `json:"reboot_needed"`
	Pending      int    `json:"pending"`
}

// HandleResult stores a result reported by nodeID. Jobs of other nodes are
// ignored.
func HandleResult(nodeID string, r Result) {
	job, err := db.GetJob(r.JobID)
	if err != nil || job.NodeID != nodeID {
		log.Printf("Ignoring result for unknown job %q from %q", r.JobID, nodeID) // #nosec G706 -- %q escapes control characters.
		return
	}
	if err := db.RecordResult(nodeID, r.JobID, job.Action, r.Success, r.Error, r.Output, r.RebootNeeded, r.Pending); err != nil {
		log.Printf("Recording result of job %s failed: %v", r.JobID, err)
		return
	}
	if !r.Success {
		notify(webhooks.WebhookPayload{Event: "job_failed", Message: fmt.Sprintf("%s on %s failed: %s", job.Action, job.Hostname, r.Error), JobID: r.JobID, NodeMac: nodeID})
	}
}

// HandleHeartbeat completes pending reboots of a node that came back.
func HandleHeartbeat(nodeID, bootID string) {
	if err := db.ConfirmReboots(nodeID, bootID); err != nil {
		log.Printf("Confirming reboots of %q failed: %v", nodeID, err) // #nosec G706 -- %q escapes control characters.
	}
}

// Status describes the patch window for the UI.
type Status struct {
	Running bool   `json:"running"`
	Current string `json:"current"`
	NextRun string `json:"next_run"`
}

// Scheduler runs the daily window.
type Scheduler struct {
	mu      sync.Mutex
	running bool
	current string
	runNow  chan struct{}
}

// Default is the process-wide scheduler.
var Default = &Scheduler{runNow: make(chan struct{}, 1)}

// Status reports whether a window is running and the next scheduled start.
func (s *Scheduler) Status() Status {
	s.mu.Lock()
	st := Status{Running: s.running, Current: s.current}
	s.mu.Unlock()
	if set, err := db.GetSettings(); err == nil && set.Enabled {
		if next, err := nextStart(set, now()); err == nil {
			st.NextRun = next.Format(time.RFC3339)
		}
	}
	return st
}

// RunNow starts a window immediately (outside the schedule).
func (s *Scheduler) RunNow() bool {
	select {
	case s.runNow <- struct{}{}:
		return true
	default:
		return false
	}
}

// Start runs the scheduler until ctx ends.
func (s *Scheduler) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			if err := db.ExpireJobs(); err != nil {
				log.Printf("Expiring jobs failed: %v", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-s.runNow:
				s.run(ctx, true)
			case <-ticker.C:
				set, err := db.GetSettings()
				if err != nil {
					log.Printf("Reading settings failed: %v", err)
					continue
				}
				if date, ok := due(set, now()); ok {
					s.run(ctx, false)
					if err := db.MarkWindowDone(date); err != nil {
						log.Printf("Recording window %s failed: %v", date, err)
					}
				}
			}
		}
	}()
}

// window returns today's window [start, end) in the configured timezone.
func window(set db.Settings, t time.Time) (start, end time.Time, err error) {
	loc, err := time.LoadLocation(set.Timezone)
	if err != nil {
		return start, end, err
	}
	t = t.In(loc)
	at := func(hhmm string) (time.Time, error) {
		p, err := time.ParseInLocation("15:04", hhmm, loc)
		if err != nil {
			return time.Time{}, err
		}
		return time.Date(t.Year(), t.Month(), t.Day(), p.Hour(), p.Minute(), 0, 0, loc), nil
	}
	if start, err = at(set.WindowStart); err != nil {
		return start, end, err
	}
	if end, err = at(set.WindowEnd); err != nil {
		return start, end, err
	}
	if !end.After(start) {
		return start, end, errors.New("window end must be after window start")
	}
	return start, end, nil
}

// due reports whether today's window should run now, and its date.
func due(set db.Settings, t time.Time) (string, bool) {
	start, end, err := window(set, t)
	if err != nil || !set.Enabled {
		return "", false
	}
	date := start.Format("2006-01-02")
	if set.LastRunDate != nil && *set.LastRunDate == date {
		return date, false
	}
	return date, !t.Before(start) && t.Before(end)
}

func nextStart(set db.Settings, t time.Time) (time.Time, error) {
	start, _, err := window(set, t)
	if err != nil {
		return start, err
	}
	if _, ok := due(set, t); ok || t.Before(start) {
		return start, nil
	}
	start, _, err = window(set, t.Add(24*time.Hour))
	return start, err
}

// skipReason says why a node is not patched in this window, or "".
func skipReason(n db.Node, online bool, windowStart time.Time) string {
	switch {
	case n.Excluded:
		return "excluded"
	case !online:
		return "offline"
	case n.LastPatchAt != nil && !n.LastPatchAt.Before(windowStart) && !(n.RebootRequired && n.AutoReboot):
		return "already patched"
	}
	return ""
}

// waitForJob dispatches action, or adopts a job that is already open (for
// example after a control-plane restart), and waits for it.
func waitForJob(ctx context.Context, n db.Node, action string, timeout time.Duration) (db.Job, error) {
	id, err := Dispatch(n.ID, action, "schedule")
	if errors.Is(err, ErrBusy) {
		log.Printf("Window: %s already has job %s; waiting for it", n.Hostname, id)
		err = nil
	}
	if err != nil {
		return db.Job{ID: id, State: "failed", Error: err.Error()}, nil
	}
	return Wait(ctx, id, timeout)
}

func (s *Scheduler) setCurrent(running bool, host string) {
	s.mu.Lock()
	s.running, s.current = running, host
	s.mu.Unlock()
}

// run patches every node in slot order, one at a time.
func (s *Scheduler) run(ctx context.Context, manual bool) {
	s.setCurrent(true, "")
	defer s.setCurrent(false, "")

	set, err := db.GetSettings()
	if err != nil {
		log.Printf("Window: cannot read settings: %v", err)
		return
	}
	start, end, err := window(set, now())
	if manual || err != nil {
		start, end = now(), now().Add(3*time.Hour)
	}
	if wait := reconnectGrace - now().Sub(startedAt); wait > 0 {
		log.Printf("Window: waiting %v for agents to reconnect", wait.Round(time.Second))
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
	nodes, err := db.ListNodes()
	if err != nil {
		log.Printf("Window: cannot list nodes: %v", err)
		return
	}
	log.Printf("Window started (manual=%v) for %d nodes", manual, len(nodes))

	var ok, rebooted, failed, skipped []string
	consecutiveFailures := 0
	for i, n := range nodes {
		if ctx.Err() != nil {
			return
		}
		if now().After(end.Add(-10 * time.Minute)) {
			for _, rest := range nodes[i:] {
				skipped = append(skipped, rest.Hostname+" (window ended)")
			}
			break
		}
		switch reason := skipReason(n, isOnline(n.ID), start); reason {
		case "":
		case "already patched":
			// e.g. the window resumed after the control plane rebooted itself
			ok = append(ok, n.Hostname)
			continue
		default:
			skipped = append(skipped, n.Hostname+" ("+reason+")")
			continue
		}
		s.setCurrent(true, n.Hostname)

		job := db.Job{}
		if !(n.LastPatchAt != nil && !n.LastPatchAt.Before(start)) {
			job, err = waitForJob(ctx, n, "apply_updates", patchTimeout)
			if err != nil {
				return
			}
			if job.State != "ok" {
				failed = append(failed, n.Hostname+": "+job.Error)
				if consecutiveFailures++; consecutiveFailures >= 2 {
					notify(webhooks.WebhookPayload{Event: "window_paused", Message: "Patch window stopped after two consecutive failures: " + strings.Join(failed, "; ")})
					for _, rest := range nodes[i+1:] {
						skipped = append(skipped, rest.Hostname+" (stopped after failures)")
					}
					break
				}
				continue
			}
		}
		consecutiveFailures = 0

		needsReboot := (job.RebootNeeded != nil && *job.RebootNeeded) || (job.ID == "" && n.RebootRequired)
		if needsReboot && n.AutoReboot {
			rj, err := waitForJob(ctx, n, "reboot", rebootTimeout)
			if err != nil {
				return
			}
			if rj.State != "ok" {
				failed = append(failed, n.Hostname+": reboot "+rj.Error)
				consecutiveFailures++
				continue
			}
			rebooted = append(rebooted, n.Hostname)
			continue
		}
		ok = append(ok, n.Hostname)
	}

	msg := fmt.Sprintf("Patch window done: %d ok, %d rebooted, %d failed, %d skipped.", len(ok), len(rebooted), len(failed), len(skipped))
	if len(failed) > 0 {
		msg += " Failed: " + strings.Join(failed, "; ") + "."
	}
	if len(skipped) > 0 {
		msg += " Skipped: " + strings.Join(skipped, ", ") + "."
	}
	log.Print(msg)
	notify(webhooks.WebhookPayload{Event: "window_summary", Message: msg})
}
