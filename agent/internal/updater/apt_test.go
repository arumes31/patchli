package updater

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"testing"
)

func TestAptManagerFull(t *testing.T) {
	oldExec := execCommandContext
	defer func() { execCommandContext = oldExec }()
	oldExecNoCtx := execCommand
	defer func() { execCommand = oldExecNoCtx }()
	oldStat := statFunc
	defer func() { statFunc = oldStat }()
	oldDisk := checkDiskSpaceFunc
	defer func() { checkDiskSpaceFunc = oldDisk }()

	execCommand = func(name string, arg ...string) *exec.Cmd {
		return getMockCommandNoCtx(name, arg...)
	}

	m := &AptManager{}
	ctx := context.Background()
	checkDiskSpaceFunc = func(path string, minBytes uint64) error { return nil }

	t.Run("CheckUpdates", func(t *testing.T) {
		execCommandContext = func(ctx context.Context, name string, arg ...string) *exec.Cmd {
			return getMockCommand(ctx, name, arg...)
		}
		if _, err := m.CheckUpdates(ctx); err != nil {
			t.Errorf("CheckUpdates failed: %v", err)
		}
	})

	t.Run("ApplyUpdates", func(t *testing.T) {
		execCommandContext = func(ctx context.Context, name string, arg ...string) *exec.Cmd {
			return getMockCommand(ctx, name, arg...)
		}
		if _, err := m.ApplyUpdates(ctx, nil); err != nil {
			t.Errorf("ApplyUpdates failed: %v", err)
		}
	})

	t.Run("RebootRequired", func(t *testing.T) {
		statFunc = func(name string) (os.FileInfo, error) {
			return nil, nil
		}
		m.RebootRequired()
	})

	t.Run("PreFlightCheck", func(t *testing.T) {
		statFunc = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}
		if err := m.PreFlightCheck(ctx); err != nil {
			t.Errorf("PreFlightCheck failed: %v", err)
		}
	})

	t.Run("PreFlightCheck_Locked", func(t *testing.T) {
		statFunc = func(name string) (os.FileInfo, error) {
			if name == "/var/lib/dpkg/lock-frontend" {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}
		execCommandContext = func(ctx context.Context, name string, arg ...string) *exec.Cmd {
			if name == "pgrep" {
				// pgrep returns 0 when process is found (locked)
				if runtime.GOOS == "windows" {
					return exec.CommandContext(ctx, "cmd.exe", "/c", "exit 0")
				}
				return exec.CommandContext(ctx, "true")
			}
			if runtime.GOOS == "windows" {
				return exec.CommandContext(ctx, "cmd.exe", "/c", "exit 0")
			}
			return exec.CommandContext(ctx, "true")
		}
		err := m.PreFlightCheck(ctx)
		if err == nil {
			t.Errorf("Expected locked error, got nil. statFunc was mocked for lock file.")
		} else if err.Error() != "package manager is currently locked or in use" {
			t.Errorf("Expected locked error, got %v", err)
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
