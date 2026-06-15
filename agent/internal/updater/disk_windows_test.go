//go:build windows

package updater

import (
	"os/exec"
	"testing"
)

func TestCheckDiskSpace_Windows(t *testing.T) {
	oldExec := execCommand
	defer func() { execCommand = oldExec }()

	t.Run("sufficient space", func(t *testing.T) {
		execCommand = func(name string, arg ...string) *exec.Cmd {
			return exec.Command("cmd.exe", "/c", "exit 0") // simulate success (exit 0 = sufficient space)
		}
		err := CheckDiskSpace("C:", 100*1024*1024)
		if err != nil {
			t.Errorf("Expected no error for sufficient space, got: %v", err)
		}
	})

	t.Run("insufficient space", func(t *testing.T) {
		execCommand = func(name string, arg ...string) *exec.Cmd {
			return exec.Command("cmd.exe", "/c", "exit 1") // simulate failure (exit 1 = insufficient space)
		}
		err := CheckDiskSpace("C:", 100*1024*1024*1024*1024) // absurdly large
		if err == nil {
			t.Error("Expected error for insufficient space, got nil")
		}
	})

	t.Run("wmi failure", func(t *testing.T) {
		execCommand = func(name string, arg ...string) *exec.Cmd {
			return exec.Command("cmd.exe", "/c", "exit 2") // simulate unexpected failure
		}
		err := CheckDiskSpace("C:", 100*1024*1024)
		if err == nil {
			t.Error("Expected error for WMI failure, got nil")
		}
	})

	t.Run("empty volume defaults to C", func(t *testing.T) {
		execCommand = func(name string, arg ...string) *exec.Cmd {
			return exec.Command("cmd.exe", "/c", "exit 0")
		}
		err := CheckDiskSpace("", 100*1024*1024) // empty path should default to C:
		if err != nil {
			t.Errorf("Expected no error with default volume, got: %v", err)
		}
	})

	t.Run("volume name extracted from path", func(t *testing.T) {
		execCommand = func(name string, arg ...string) *exec.Cmd {
			return exec.Command("cmd.exe", "/c", "exit 0")
		}
		err := CheckDiskSpace("D:\\some\\path", 100*1024*1024)
		if err != nil {
			t.Errorf("Expected no error for D: drive, got: %v", err)
		}
	})
}
