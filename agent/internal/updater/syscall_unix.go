//go:build !windows
// +build !windows

package updater

import "syscall"

func syscallStatfs(path string, stat *syscall.Statfs_t) error {
	return syscall.Statfs(path, stat)
}
