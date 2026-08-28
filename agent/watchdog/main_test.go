package main

import (
	"os"
	"os/exec"
	"runtime"
	"testing"
	"time"
)

func TestRunWatchdog_AgentExits(t *testing.T) {
	newCommand := func(name string, args ...string) *exec.Cmd {
		if runtime.GOOS == "windows" {
			return exec.Command("powershell", "-Command", "exit 0")
		}
		return exec.Command("true")
	}

	sigChan := make(chan os.Signal, 1)
	done := make(chan struct{})
	go func() {
		runWatchdog("dummy-agent", sigChan, newCommand)
		close(done)
	}()
	sigChan <- os.Interrupt
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("watchdog did not stop")
	}
}

func TestRunWatchdog_StartError(t *testing.T) {
	newCommand := func(name string, args ...string) *exec.Cmd {
		return exec.Command("invalid-command-that-does-not-exist")
	}

	sigChan := make(chan os.Signal, 1)
	done := make(chan struct{})
	go func() {
		runWatchdog("dummy-agent", sigChan, newCommand)
		close(done)
	}()
	sigChan <- os.Interrupt
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("watchdog did not stop")
	}
}

func TestParseArgs(t *testing.T) {
	tests := []struct {
		args     []string
		expected string
	}{
		{[]string{"watchdog"}, "/usr/local/bin/patchli-agent"},
		{[]string{"watchdog", "/custom/path"}, "/custom/path"},
	}

	for _, tt := range tests {
		res := parseArgs(tt.args)
		if res != tt.expected {
			t.Errorf("Expected %s, got %s", tt.expected, res)
		}
	}
}
