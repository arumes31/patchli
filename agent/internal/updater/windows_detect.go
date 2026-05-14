//go:build !windows
// +build !windows

package updater

import "fmt"

func DetectWindowsManager() (PackageManager, error) {
	return nil, fmt.Errorf("not running on Windows")
}
