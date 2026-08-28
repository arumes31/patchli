//go:build !windows

package updater

import (
	"fmt"
	"math"
	"syscall"
)

// CheckDiskSpace ensures there is at least minBytes available on the given path.
func CheckDiskSpace(path string, minBytes uint64) error {
	var stat syscall.Statfs_t
	err := syscallStatfs(path, &stat)
	if err != nil {
		return fmt.Errorf("failed to check disk space: %v", err)
	}

	if stat.Bsize <= 0 {
		return fmt.Errorf("invalid filesystem block size: %d", stat.Bsize)
	}
	blockSize := uint64(stat.Bsize)
	var available uint64
	if stat.Bavail > math.MaxUint64/blockSize {
		available = math.MaxUint64
	} else {
		available = stat.Bavail * blockSize
	}

	if available < minBytes {
		return fmt.Errorf("insufficient disk space: required %d bytes, available %d bytes", minBytes, available)
	}
	return nil
}
