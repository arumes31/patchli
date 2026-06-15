//go:build !windows

package updater

import (
	"syscall"
	"testing"
)

func TestCheckDiskSpace_Unix(t *testing.T) {
	oldStatfs := syscallStatfs
	defer func() { syscallStatfs = oldStatfs }()

	t.Run("sufficient space", func(t *testing.T) {
		syscallStatfs = func(path string, stat *syscall.Statfs_t) error {
			stat.Bsize = 4096
			stat.Bavail = 100000 // 4096 * 100000 = 409,600,000 bytes
			return nil
		}
		err := CheckDiskSpace("/some/path", 100*1024*1024) // 100MB
		if err != nil {
			t.Errorf("Expected no error for sufficient space, got: %v", err)
		}
	})

	t.Run("insufficient space", func(t *testing.T) {
		syscallStatfs = func(path string, stat *syscall.Statfs_t) error {
			stat.Bsize = 4096
			stat.Bavail = 100 // 4096 * 100 = 409,600 bytes
			return nil
		}
		err := CheckDiskSpace("/some/path", 100*1024*1024) // 100MB
		if err == nil {
			t.Error("Expected error for insufficient space, got nil")
		}
	})

	t.Run("statfs error", func(t *testing.T) {
		syscallStatfs = func(path string, stat *syscall.Statfs_t) error {
			return syscall.ENOENT
		}
		err := CheckDiskSpace("/nonexistent/path", 100*1024*1024)
		if err == nil {
			t.Error("Expected error for statfs failure, got nil")
		}
	})

	t.Run("exact space match", func(t *testing.T) {
		syscallStatfs = func(path string, stat *syscall.Statfs_t) error {
			stat.Bsize = 1024
			stat.Bavail = 1024 // 1024 * 1024 = 1,048,576 bytes = exactly 1MB
			return nil
		}
		err := CheckDiskSpace("/some/path", 1024*1024) // exactly 1MB
		if err != nil {
			t.Errorf("Expected no error for exact space match, got: %v", err)
		}
	})

	t.Run("overflow edge case", func(t *testing.T) {
		syscallStatfs = func(path string, stat *syscall.Statfs_t) error {
			stat.Bsize = 4096
			stat.Bavail = ^uint64(0) // MaxUint64 - should trigger overflow protection
			return nil
		}
		err := CheckDiskSpace("/some/path", 1) // minimal requirement
		if err != nil {
			t.Errorf("Expected no error with overflow protection, got: %v", err)
		}
	})

	t.Run("zero bsize", func(t *testing.T) {
		syscallStatfs = func(path string, stat *syscall.Statfs_t) error {
			stat.Bsize = 0
			stat.Bavail = 100000
			return nil
		}
		err := CheckDiskSpace("/some/path", 1)
		if err == nil {
			t.Error("Expected error for zero block size, got nil")
		}
	})
}
