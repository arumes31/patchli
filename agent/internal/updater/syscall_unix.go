//go:build !windows
// +build !windows

package updater

import "syscall"

var syscallStatfs = func(path string, stat *syscall.Statfs_t) error {
	return syscall.Statfs(path, stat)
}
