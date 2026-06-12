package orchestration

import (
	"testing"
	"time"
	"github.com/arumes31/patchli/server/internal/models"
)

func TestWorkerPool_MaintenanceWindow(t *testing.T) {
	wp := NewWorkerPool(1)

	now := time.Now()
	job := models.Job{
		ID: "job-mw-test",
		MaintenanceWindow: &models.MaintenanceWindow{
			StartTime: now.Add(1 * time.Hour),
			EndTime:   now.Add(2 * time.Hour),
		},
	}

	// This will log "delayed" and retry.
	// We just want to exercise the code path.
	wp.processJob(job)
}

func TestWorkerPool_ExpiredWindow(t *testing.T) {
	wp := NewWorkerPool(1)

	now := time.Now()
	job := models.Job{
		ID: "job-mw-expired",
		MaintenanceWindow: &models.MaintenanceWindow{
			StartTime: now.Add(-2 * time.Hour),
			EndTime:   now.Add(-1 * time.Hour),
		},
	}

	wp.processJob(job)
}
