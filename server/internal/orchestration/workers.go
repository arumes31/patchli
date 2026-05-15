package orchestration

import (
	"log"
	"sync"
	"time"

	"github.com/arumes31/patchli/server/internal/db"
	"github.com/arumes31/patchli/server/internal/fleet"
	"github.com/arumes31/patchli/server/internal/models"
	"github.com/arumes31/patchli/server/internal/webhooks"
)

type WorkerPool struct {
	jobQueue        chan models.Job
	maxWorkers      int
	groupSemaphores map[int]chan struct{}
	pausedGroups    map[int]bool
	pauseCond       *sync.Cond
	mu              sync.Mutex
	stopChan        chan struct{}
}

func NewWorkerPool(maxWorkers int) *WorkerPool {
	wp := &WorkerPool{
		jobQueue:        make(chan models.Job, 100),
		maxWorkers:      maxWorkers,
		groupSemaphores: make(map[int]chan struct{}),
		pausedGroups:    make(map[int]bool),
		stopChan:        make(chan struct{}),
	}
	wp.pauseCond = sync.NewCond(&wp.mu)
	return wp
}

func (wp *WorkerPool) Start() {
	for i := 0; i < wp.maxWorkers; i++ {
		go wp.worker()
	}
}

func (wp *WorkerPool) Stop() {
	close(wp.stopChan)
}

func (wp *WorkerPool) Submit(job models.Job) {
	wp.jobQueue <- job
}

func (wp *WorkerPool) PauseGroup(groupID int) {
	wp.mu.Lock()
	wp.pausedGroups[groupID] = true
	wp.mu.Unlock()
	log.Printf("Group %d paused.", groupID)
}

func (wp *WorkerPool) ResumeGroup(groupID int) {
	wp.mu.Lock()
	wp.pausedGroups[groupID] = false
	wp.pauseCond.Broadcast()
	wp.mu.Unlock()
	log.Printf("Group %d resumed.", groupID)
}

func (wp *WorkerPool) worker() {
	for {
		select {
		case <-wp.stopChan:
			for {
				select {
				case job, ok := <-wp.jobQueue:
					if ok {
						wp.processJob(job)
					} else {
						return
					}
				default:
					return
				}
			}
		case job, ok := <-wp.jobQueue:
			if !ok {
				return
			}
			wp.processJob(job)
		}
	}
}

func (wp *WorkerPool) processJob(job models.Job) {
	if !fleet.Registry.AcquireJob(job.NodeMac, job.ID) {
		log.Printf("Conflict: Node %s already has active job. Skipping job %s.", job.NodeMac, job.ID)
		return
	}
	defer fleet.Registry.ReleaseJob(job.NodeMac)

	wp.mu.Lock()
	for wp.pausedGroups[job.GroupID] {
		wp.pauseCond.Wait()
	}
	wp.mu.Unlock()

	if job.MaintenanceWindow != nil {
		now := time.Now()
		if now.After(job.MaintenanceWindow.EndTime) {
			log.Printf("Job %s skipped: Maintenance window ended.", job.ID)
			return
		}
		if job.MaintenanceWindow.EndTime.Sub(now) < 15*time.Minute {
			log.Printf("Job %s skipped: Less than 15 minutes left in window.", job.ID)
			return
		}
		if now.Before(job.MaintenanceWindow.StartTime) {
			log.Printf("Job %s delayed: Outside window.", job.ID)
			go func(j models.Job) {
				time.Sleep(10 * time.Second)
				wp.Submit(j)
			}(job)
			return
		}
	}

	wp.mu.Lock()
	sem, ok := wp.groupSemaphores[job.GroupID]
	if !ok {
		sem = make(chan struct{}, 2)
		wp.groupSemaphores[job.GroupID] = sem
	}
	wp.mu.Unlock()

	sem <- struct{}{}
	defer func() { <-sem }()

	log.Printf("Worker executing job %s on node %s (Action: %s)", job.ID, job.NodeMac, job.Action)

	job.StartTime = time.Now()
	err := fleet.Registry.SendCommand(job.NodeMac, models.CommandPayload{
		ID:                 job.ID,
		Action:             job.Action,
		Packages:           job.Packages,
		PrePatchScript:     job.PrePatchScript,
		PostPatchScript:    job.PostPatchScript,
		HealthCheckCommand: job.HealthCheckCommand,
	})

	if err != nil {
		log.Printf("Failed to send command to agent %s: %v", job.NodeMac, err)
		webhooks.NotifyWebhooks(webhooks.WebhookPayload{
			Event:   "job_failed",
			Message: "Failed to communicate with agent.",
			JobID:   job.ID,
			NodeMac: job.NodeMac,
		})
		return
	}

	go wp.monitorJobExpiration(job)

	webhooks.NotifyWebhooks(webhooks.WebhookPayload{
		Event:   "job_started",
		Message: "Patch job dispatched successfully.",
		JobID:   job.ID,
		NodeMac: job.NodeMac,
	})
}

func (wp *WorkerPool) monitorJobExpiration(job models.Job) {
	timeout := 2 * time.Hour
	timer := time.NewTimer(timeout)
	<-timer.C
	
	running, err := db.IsJobRunning(job.ID)
	if err == nil && running {
		log.Printf("Job %s timed out after %v", job.ID, timeout)
	}
}
