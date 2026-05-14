package main

import (
	"log"
	"sync"
)

type Job struct {
	ID         string
	NodeMac    string
	GroupID    int
	Action     string
	Packages   []string
}

type WorkerPool struct {
	jobQueue chan Job
	maxWorkers int
	// groupSemaphores tracks parallelism per group
	groupSemaphores map[int]chan struct{}
	mu sync.Mutex
}

func NewWorkerPool(maxWorkers int) *WorkerPool {
	return &WorkerPool{
		jobQueue:        make(chan Job, 100),
		maxWorkers:     maxWorkers,
		groupSemaphores: make(map[int]chan struct{}),
	}
}

func (wp *WorkerPool) Start() {
	for i := 0; i < wp.maxWorkers; i++ {
		go wp.worker()
	}
}

func (wp *WorkerPool) Submit(job Job) {
	wp.jobQueue <- job
}

func (wp *WorkerPool) worker() {
	for job := range wp.jobQueue {
		wp.processJob(job)
	}
}

func (wp *WorkerPool) processJob(job Job) {
	// 1. Get or create semaphore for the group
	wp.mu.Lock()
	sem, ok := wp.groupSemaphores[job.GroupID]
	if !ok {
		// Default parallelism of 2 for this demo, in real app fetch from DB
		sem = make(chan struct{}, 2) 
		wp.groupSemaphores[job.GroupID] = sem
	}
	wp.mu.Unlock()

	// 2. Acquire group semaphore (Respect Max Parallelism)
	sem <- struct{}{}
	defer func() { <-sem }()

	log.Printf("Worker executing job %s on node %s (Action: %s)", job.ID, job.NodeMac, job.Action)

	// 3. Send command to agent via WebSocket
	err := manager.SendCommand(job.NodeMac, CommandPayload{
		ID:      job.ID,
		Action:  job.Action,
		Packages: job.Packages,
	})

	if err != nil {
		log.Printf("Failed to send command to agent %s: %v", job.NodeMac, err)
		return
	}

	// In a real system, we'd wait for the 'result' message from the agent
	// and update the audit_logs table.
}
