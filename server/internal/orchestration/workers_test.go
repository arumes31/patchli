package orchestration

import (
	"sync"
	"testing"
	"time"

	"github.com/arumes31/patchli/server/internal/models"
	"github.com/arumes31/patchli/server/internal/webhooks"
)

func TestWorkerPool_Submit(t *testing.T) {
	wp := NewWorkerPool(1)
	job := models.Job{
		ID:      "job-test-submit",
		NodeMac: "00:11:22:33:44:55",
		GroupID: 1,
		Action:  "patch",
	}

	wp.Submit(job)

	select {
	case received := <-wp.jobQueue:
		if received.ID != job.ID {
			t.Errorf("Expected job ID %s, got %s", job.ID, received.ID)
		}
	case <-time.After(1 * time.Second):
		t.Error("Timed out waiting for job in queue")
	}
}

func TestWorkerPool_Stop(t *testing.T) {
	wp := NewWorkerPool(1)
	wp.Start()

	wp.Stop()

	select {
	case <-wp.stopChan:
		// Success
	case <-time.After(1 * time.Second):
		t.Error("Stop() did not close stopChan")
	}
}

func TestWorkerPool(t *testing.T) {
	wp := NewWorkerPool(2)

	t.Run("Submit Job", func(t *testing.T) {
		job := models.Job{
			ID:      "job-1",
			NodeMac: "00:11:22:33:44:55",
			GroupID: 1,
			Action:  "patch",
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
	webhooks.NotifyWebhooks = func(payload webhooks.WebhookPayload) {
		lastEvent = payload.Event
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
		if lastEvent != "job_failed" {
			t.Errorf("Expected job_failed event, got %s", lastEvent)
		}
	})
}

func TestWorkerPoolStartStop(t *testing.T) {
	wp := NewWorkerPool(1)
	wp.Start()

	// Submit a dummy job to ensure worker is running
	wp.Submit(models.Job{ID: "dummy", GroupID: 99})
	time.Sleep(20 * time.Millisecond)

	// Close the jobQueue to stop workers
	wp.Stop()

	// Wait a bit for goroutines to exit (coverage check)
	time.Sleep(50 * time.Millisecond)
}
