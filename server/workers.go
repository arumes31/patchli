package main

import (
	"log"
	"sync"
	"time"
)

type Job struct {
	ID                 string
	NodeMac            string
	GroupID            int
	Action             string
	Packages           []string
	PrePatchScript     string
	PostPatchScript    string
	HealthCheckCommand string
	MaintenanceWindow  *MaintenanceWindow
}

type MaintenanceWindow struct {
	StartTime time.Time
	EndTime   time.Time
}

type WorkerPool struct {
	jobQueue        chan Job
	maxWorkers      int
	groupSemaphores map[int]chan struct{}
	pausedGroups    map[int]bool // Tracks if a group's patching is paused
	pauseCond       *sync.Cond
	mu              sync.Mutex
}

func NewWorkerPool(maxWorkers int) *WorkerPool {
	wp := &WorkerPool{
		jobQueue:        make(chan Job, 100),
		maxWorkers:      maxWorkers,
		groupSemaphores: make(map[int]chan struct{}),
		pausedGroups:    make(map[int]bool),
	}
	wp.pauseCond = sync.NewCond(&wp.mu)
	return wp
}

func (wp *WorkerPool) Start() {
	for i := 0; i < wp.maxWorkers; i++ {
		go wp.worker()
	}
}

func (wp *WorkerPool) Submit(job Job) {
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
	for job := range wp.jobQueue {
		wp.processJob(job)
	}
}

func (wp *WorkerPool) processJob(job Job) {
	// 1. Check if group is paused (Feature 11)
	wp.mu.Lock()
	for wp.pausedGroups[job.GroupID] {
		wp.pauseCond.Wait()
	}
	wp.mu.Unlock()

	// 2. Check Maintenance Window (Feature 5)
	if job.MaintenanceWindow != nil {
		now := time.Now()
		if now.After(job.MaintenanceWindow.EndTime) {
			log.Printf("Job %s skipped/failed: Maintenance window ended.", job.ID)
			return
		}
		if now.Before(job.MaintenanceWindow.StartTime) {
			log.Printf("Job %s delayed: Outside maintenance window.", job.ID)
			// Re-queue job for later
			go func(j Job) {
				time.Sleep(10 * time.Second)
				wp.Submit(j)
			}(job)
			return
		}
	}

	// 3. Get or create semaphore for the group
	wp.mu.Lock()
	sem, ok := wp.groupSemaphores[job.GroupID]
	if !ok {
		sem = make(chan struct{}, 2) // Default parallelism of 2
		wp.groupSemaphores[job.GroupID] = sem
	}
	wp.mu.Unlock()

	// 4. Acquire group semaphore (Respect Max Parallelism)
	sem <- struct{}{}
	defer func() { <-sem }()

	log.Printf("Worker executing job %s on node %s (Action: %s)", job.ID, job.NodeMac, job.Action)

	// 5. Send command to agent via WebSocket
	err := manager.SendCommand(job.NodeMac, CommandPayload{
		ID:                 job.ID,
		Action:             job.Action,
		Packages:           job.Packages,
		PrePatchScript:     job.PrePatchScript,
		PostPatchScript:    job.PostPatchScript,
		HealthCheckCommand: job.HealthCheckCommand,
	})

	if err != nil {
		log.Printf("Failed to send command to agent %s: %v", job.NodeMac, err)
		NotifyWebhooks(WebhookPayload{
			Event:   "job_failed",
			Message: "Failed to communicate with agent.",
			JobID:   job.ID,
			NodeMac: job.NodeMac,
		})
		return
	}

	NotifyWebhooks(WebhookPayload{
		Event:   "job_started",
		Message: "Patch job dispatched to agent successfully.",
		JobID:   job.ID,
		NodeMac: job.NodeMac,
	})
}
