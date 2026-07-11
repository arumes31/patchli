//go:build windows
// +build windows

package updater

import (
	"context"
	"os/exec"
	"testing"
)

func TestWindowsManager(t *testing.T) {
	oldExec := execCommandContext
	defer func() { execCommandContext = oldExec }()
	oldExecNoCtx := execCommand
	defer func() { execCommand = oldExecNoCtx }()

	m := &WindowsManager{}
	ctx := context.Background()

	t.Run("CheckUpdates", func(t *testing.T) {
		execCommandContext = func(ctx context.Context, name string, arg ...string) *exec.Cmd {
			return getMockCommand(ctx, name, arg...)
		}
		_, err := m.CheckUpdates(ctx)
		if err != nil {
			t.Errorf("CheckUpdates failed: %v", err)
		}
	})

	t.Run("ApplyUpdates", func(t *testing.T) {
		execCommandContext = func(ctx context.Context, name string, arg ...string) *exec.Cmd {
			return getMockCommand(ctx, name, arg...)
		}
		_, err := m.ApplyUpdates(ctx, []string{"KB123"})
		if err != nil {
			t.Errorf("ApplyUpdates failed: %v", err)
		}
	})

	t.Run("RebootRequired", func(t *testing.T) {
		execCommand = func(name string, arg ...string) *exec.Cmd {
			// Mocking PowerShell output for True
			return exec.Command("cmd.exe", "/c", "echo True")
		}
		if !m.RebootRequired() {
			t.Error("Expected RebootRequired to be true")
		}
	})

	t.Run("PreFlightCheck", func(t *testing.T) {
		execCommandContext = func(ctx context.Context, name string, arg ...string) *exec.Cmd {
			return getMockCommand(ctx, name, arg...)
		}
		if err := m.PreFlightCheck(ctx); err != nil {
			t.Errorf("PreFlightCheck failed: %v", err)
		}
	})

	t.Run("Cleanup", func(t *testing.T) {
		execCommandContext = func(ctx context.Context, name string, arg ...string) *exec.Cmd {
			return getMockCommand(ctx, name, arg...)
		}
		if err := m.Cleanup(ctx); err != nil {
			t.Errorf("Cleanup failed: %v", err)
		}
	})
}

func TestCheckDiskSpace(t *testing.T) {
	oldExec := execCommand
	defer func() { execCommand = oldExec }()

	t.Run("Success", func(t *testing.T) {
		execCommand = func(name string, arg ...string) *exec.Cmd {
			return exec.Command("cmd.exe", "/c", "exit 0")
		}
		if err := CheckDiskSpace("C:", 100); err != nil {
			t.Errorf("Expected success, got %v", err)
		}
	})

	t.Run("Failure", func(t *testing.T) {
		execCommand = func(name string, arg ...string) *exec.Cmd {
			return exec.Command("cmd.exe", "/c", "exit 1")
		}
		if err := CheckDiskSpace("C:", 100); err == nil {
			t.Error("Expected error, got nil")
		}
	})
}
