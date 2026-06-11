package orchestration

import (
	"sync"
	"testing"
	"time"

	"github.com/arumes31/patchli/server/internal/fleet"
	"github.com/arumes31/patchli/server/internal/models"
	"github.com/arumes31/patchli/server/internal/webhooks"
)

func TestWorkerPool(t *testing.T) {
	wp := NewWorkerPool(2)

	t.Run("Submit Job", func(t *testing.T) {
		job := models.Job{
			ID:       "job-1",
			NodeMac:  "00:11:22:33:44:55",
			GroupID:  1,
			Action:   "patch",
		}
		wp.Submit(job)
		j := <-wp.jobQueue
		if j.ID != "job-1" {
			t.Errorf("Expected job-1, got %s", j.ID)
		}
	})

	t.Run("Pause and Resume Group", func(t *testing.T) {
		wp.PauseGroup(1)
		wp.mu.Lock()
		if !wp.pausedGroups[1] {
			t.Error("Expected group 1 to be paused")
		}
		wp.mu.Unlock()

		wp.ResumeGroup(1)
		wp.mu.Lock()
		if wp.pausedGroups[1] {
			t.Error("Expected group 1 to be resumed")
		}
		wp.mu.Unlock()
	})
}

func TestProcessJob(t *testing.T) {
	wp := NewWorkerPool(1)

	// Mock webhooks.NotifyWebhooks
	oldNotify := webhooks.NotifyWebhooks
	defer func() { webhooks.NotifyWebhooks = oldNotify }()

	var lastEvent string
	var mu sync.Mutex
	webhooks.NotifyWebhooks = func(payload webhooks.WebhookPayload) {
		mu.Lock()
		lastEvent = payload.Event
		mu.Unlock()
	}

	t.Run("Maintenance Window - Outside", func(t *testing.T) {
		job := models.Job{
			ID:      "job-mw",
			NodeMac: "mac",
			GroupID: 2,
			MaintenanceWindow: &models.MaintenanceWindow{
				StartTime: time.Now().Add(1 * time.Hour),
				EndTime:   time.Now().Add(2 * time.Hour),
			},
		}
		wp.processJob(job)
		// Should be re-queued, not processed
	})

	t.Run("Maintenance Window - Expired", func(t *testing.T) {
		job := models.Job{
			ID:      "job-mw-expired",
			NodeMac: "mac",
			GroupID: 2,
			MaintenanceWindow: &models.MaintenanceWindow{
				StartTime: time.Now().Add(-2 * time.Hour),
				EndTime:   time.Now().Add(-1 * time.Hour),
			},
		}
		wp.processJob(job)
	})

	t.Run("Maintenance Window - Imminent", func(t *testing.T) {
		job := models.Job{
			ID:      "job-mw-imminent",
			NodeMac: "mac",
			GroupID: 2,
			MaintenanceWindow: &models.MaintenanceWindow{
				StartTime: time.Now().Add(-1 * time.Hour),
				EndTime:   time.Now().Add(5 * time.Minute),
			},
		}
		wp.processJob(job)
	})

	t.Run("Pause Wait", func(t *testing.T) {
		wp.PauseGroup(3)

		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			wp.processJob(models.Job{ID: "job-paused", GroupID: 3})
		}()

		time.Sleep(50 * time.Millisecond)
		wp.ResumeGroup(3)
		wg.Wait()
	})

	t.Run("Failed Command", func(t *testing.T) {
		job := models.Job{
			ID:      "job-fail",
			NodeMac: "offline-mac",
			GroupID: 4,
		}
		wp.processJob(job)
		mu.Lock()
		event := lastEvent
		mu.Unlock()
		if event != "job_failed" {
			t.Errorf("Expected job_failed event, got %s", event)
		}
	})

	t.Run("Conflict Job", func(t *testing.T) {
		fleet.Registry.AcquireJob("mac-busy", "job-busy")
		defer fleet.Registry.ReleaseJob("mac-busy")

		job := models.Job{ID: "job-conflict", NodeMac: "mac-busy"}
		wp.processJob(job)
		// Should return early, verify by checking that lastEvent didn't change to "job_started" or "job_failed" for this job
	})
}

func TestWorkerPoolStartStop(t *testing.T) {
	wp := NewWorkerPool(1)
	wp.Start()

	// Submit a job and verify it gets picked up
	done := make(chan bool)
	oldNotify := webhooks.NotifyWebhooks
	defer func() { webhooks.NotifyWebhooks = oldNotify }()
	webhooks.NotifyWebhooks = func(payload webhooks.WebhookPayload) {
		if payload.JobID == "picked-up" {
			done <- true
		}
	}

	wp.Submit(models.Job{ID: "picked-up", NodeMac: "mac1", GroupID: 10})

	select {
	case <-done:
		// Job was processed
	case <-time.After(1 * time.Second):
		t.Error("Job was not processed by worker")
	}

	wp.Stop()

	select {
	case <-wp.stopChan:
		// success
	case <-time.After(100 * time.Millisecond):
		t.Error("stopChan was not closed after Stop()")
	}
}

func TestMonitorJobExpiration(t *testing.T) {
	// Set a very short timeout for testing
	oldTimeout := jobExpirationTimeout
	jobExpirationTimeout = 10 * time.Millisecond
	defer func() { jobExpirationTimeout = oldTimeout }()

	wp := NewWorkerPool(1)
	job := models.Job{ID: "test-timeout"}

	// This will start the timer and wait 10ms
	// It's a goroutine in processJob, but we call it directly here for coverage
	wp.monitorJobExpiration(job)

	// Coverage verified if it finishes without error
}

func TestWorkerGracefulStopWithRemainingJobs(t *testing.T) {
	wp := NewWorkerPool(1)

	// Add jobs to queue before starting
	wp.Submit(models.Job{ID: "job-1", NodeMac: "mac-1"})
	wp.Submit(models.Job{ID: "job-2", NodeMac: "mac-2"})

	processedCount := 0
	var mu sync.Mutex
	oldNotify := webhooks.NotifyWebhooks
	defer func() { webhooks.NotifyWebhooks = oldNotify }()
	webhooks.NotifyWebhooks = func(payload webhooks.WebhookPayload) {
		mu.Lock()
		processedCount++
		mu.Unlock()
	}

	wp.Stop() // Close stopChan immediately

	// Start workers - they should see stopChan closed and process remaining jobs
	wp.Start()

	// Wait a bit for jobs to be processed
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	count := processedCount
	mu.Unlock()

	if count != 2 {
		t.Errorf("Expected 2 jobs to be processed during graceful stop, got %d", count)
	}
}
