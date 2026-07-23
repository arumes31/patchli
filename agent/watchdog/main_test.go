package main

import (
	"os"
	"os/exec"
	"runtime"
	"testing"
	"time"
)

func TestRunWatchdog_AgentExits(t *testing.T) {
	// Mock execCommand to exit immediately
	oldExec := execCommand
	execCommand = func(name string, args ...string) *exec.Cmd {
		if runtime.GOOS == "windows" {
			return exec.Command("powershell", "-Command", "exit 0")
		}
		return exec.Command("true")
	}
	defer func() { execCommand = oldExec }()

	sigChan := make(chan os.Signal, 1)
	go runWatchdog("dummy-agent", sigChan)

	time.Sleep(100 * time.Millisecond)
	sigChan <- os.Interrupt
}

func TestRunWatchdog_StartError(t *testing.T) {
	oldExec := execCommand
	execCommand = func(name string, args ...string) *exec.Cmd {
		return exec.Command("invalid-command-that-does-not-exist")
	}
	defer func() { execCommand = oldExec }()

	sigChan := make(chan os.Signal, 1)
	go runWatchdog("dummy-agent", sigChan)

	time.Sleep(100 * time.Millisecond)
	sigChan <- os.Interrupt
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
