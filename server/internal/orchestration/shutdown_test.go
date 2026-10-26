package orchestration

import (
	"github.com/arumes31/patchli/server/internal/models"
	"github.com/arumes31/patchli/server/internal/webhooks"
	"testing"
	"time"
)

func TestShutdownResumesGroupsAndDrainsQueuedJobs(t *testing.T) {
	previous := notifyWebhooks
	completed := make(chan webhooks.WebhookPayload, 2)
	notifyWebhooks = func(payload webhooks.WebhookPayload) { completed <- payload }
	t.Cleanup(func() { notifyWebhooks = previous })
	pool := NewWorkerPool(1)
	pool.PauseGroup(920)
	for _, id := range []string{"shutdown-a", "shutdown-b"} {
		if err := pool.Submit(models.Job{ID: id, NodeMac: "offline-" + id, GroupID: 920}); err != nil {
			t.Fatal(err)
		}
	}
	pool.Start()
	pool.Stop()
	pool.mu.Lock()
	paused := pool.pausedGroups[920]
	pool.mu.Unlock()
	if paused {
		t.Error("shutdown left group paused")
	}
	done := make(chan struct{})
	go func() { pool.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown did not finish draining queued work")
	}
	seen := map[string]bool{}
	for i := 0; i < 2; i++ {
		select {
		case event := <-completed:
			if event.Event != "job_failed" {
				t.Fatalf("unexpected dispatch result: %+v", event)
			}
			seen[event.JobID] = true
		default:
			t.Fatal("shutdown discarded queued work")
		}
	}
	if !seen["shutdown-a"] || !seen["shutdown-b"] {
		t.Fatalf("missing dispatch result: %v", seen)
	}
}
