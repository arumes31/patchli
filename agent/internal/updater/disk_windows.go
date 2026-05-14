//go:build windows
// +build windows

package updater

import (
	"fmt"
	"os/exec"
)

// CheckDiskSpace ensures there is at least minBytes available on the root drive (C:).
func CheckDiskSpace(path string, minBytes uint64) error {
	script := fmt.Sprintf(`
$disk = Get-WmiObject Win32_LogicalDisk -Filter "DeviceID='C:'"
if ($disk.FreeSpace -lt %d) { exit 1 }
`, minBytes)
	cmd := exec.Command("powershell", "-NoProfile", "-Command", script)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("insufficient disk space on C:")
	}
	return nil
}
