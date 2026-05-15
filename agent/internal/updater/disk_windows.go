//go:build windows

package updater

import (
	"fmt"
	"path/filepath"
)

// CheckDiskSpace ensures there is at least minBytes available on the root drive of the path.
func CheckDiskSpace(path string, minBytes uint64) error {
	volume := filepath.VolumeName(path)
	if volume == "" {
		volume = "C:"
	}
	script := fmt.Sprintf(`
$disk = Get-WmiObject Win32_LogicalDisk -Filter "DeviceID='%s'"
if ($disk.FreeSpace -lt %d) { exit 1 }
`, volume, minBytes)
	cmd := execCommand("powershell", "-NoProfile", "-Command", script)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("insufficient disk space on %s", volume)
	}
	return nil
}

