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
	err := syscall.Statfs(path, &stat)
	if err != nil {
		return fmt.Errorf("failed to check disk space: %v", err)
	}

	var available uint64
	// #nosec G115 -- stat.Bsize is guaranteed to be positive and casting int64 to uint64 is safe here
	bsize := uint64(stat.Bsize)
	if bsize > 0 && stat.Bavail > math.MaxUint64/bsize {
		available = math.MaxUint64
	} else {
		available = stat.Bavail * bsize
	}

	if available < minBytes {
		return fmt.Errorf("insufficient disk space: required %d bytes, available %d bytes", minBytes, available)
	}
	return nil
}
