//go:build !windows
// +build !windows

package updater

import (
	"fmt"
	"syscall"
)

// CheckDiskSpace ensures there is at least minBytes available on the given path.
func CheckDiskSpace(path string, minBytes uint64) error {
	var stat syscall.Statfs_t
	err := syscall.Statfs(path, &stat)
	if err != nil {
		return fmt.Errorf("failed to check disk space: %v", err)
	}

	available := stat.Bavail * uint64(stat.Bsize)
	if available < minBytes {
		return fmt.Errorf("insufficient disk space: required %d bytes, available %d bytes", minBytes, available)
	}
	return nil
}
