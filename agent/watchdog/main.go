package main

import (
	"log"
	"os"
	"os/exec"
	"time"
)

func main() {
	agentPath := "/usr/local/bin/patchli-agent"
	if len(os.Args) > 1 {
		agentPath = os.Args[1]
	}

	log.Printf("Starting Patchli Watchdog for %s", agentPath)

	for {
		cmd := exec.Command(agentPath)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		log.Printf("Watchdog: Starting agent...")
		err := cmd.Start()
		if err != nil {
			log.Printf("Watchdog: Failed to start agent: %v. Retrying in 10s...", err)
			time.Sleep(10 * time.Second)
			continue
		}

		// Wait for the agent to exit
		err = cmd.Wait()
		if err != nil {
			log.Printf("Watchdog: Agent exited with error: %v", err)
		} else {
			log.Printf("Watchdog: Agent exited cleanly.")
			// If it exited cleanly (e.g. self-destruct or planned shutdown), we might want to stop the watchdog too.
			// For self-healing, we restart anyway unless instructed otherwise via IPC/signal.
		}

		log.Println("Watchdog: Restarting agent in 5 seconds...")
		time.Sleep(5 * time.Second)
	}
}
