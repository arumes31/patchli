package main

import (
	"log"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"
)

var execCommand = exec.Command

func main() {
	agentPath := "/usr/local/bin/patchli-agent"
	if len(os.Args) > 1 {
		agentPath = os.Args[1]
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	runWatchdog(agentPath, sigChan)
}

func runWatchdog(agentPath string, sigChan <-chan os.Signal) {
	log.Printf("Starting Patchli Watchdog for %s", agentPath)

	for {
		cmd := execCommand(agentPath)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		log.Printf("Watchdog: Starting agent...")
		err := cmd.Start()
		if err != nil {
			log.Printf("Watchdog: Failed to start agent: %v. Retrying in 10s...", err)
			select {
			case <-sigChan:
				return
			case <-time.After(10 * time.Second):
				continue
			}
		}

		done := make(chan error, 1)
		go func() {
			done <- cmd.Wait()
		}()

		select {
		case sig := <-sigChan:
			log.Printf("Watchdog received signal: %v", sig)
			if cmd.Process != nil {
				cmd.Process.Signal(sig)
			}
			<-done
			log.Println("Watchdog exiting cleanly.")
			return
		case err := <-done:
			if err != nil {
				log.Printf("Watchdog: Agent exited with error: %v", err)
			} else {
				log.Printf("Watchdog: Agent exited cleanly.")
			}
		}

		log.Println("Watchdog: Restarting agent in 5 seconds...")
		select {
		case <-sigChan:
			return
		case <-time.After(5 * time.Second):
		}
	}
}
