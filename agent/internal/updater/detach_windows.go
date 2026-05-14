//go:build windows
// +build windows

package updater

import (
	"os/exec"
)

func detachProcess(cmd *exec.Cmd) {
	// On Windows, DETACHED_PROCESS can be used via SysProcAttr, but omitting it might be fine for a sleep-delete script.
}
