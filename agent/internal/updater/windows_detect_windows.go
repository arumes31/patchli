//go:build windows
// +build windows

package updater

func DetectWindowsManager() (PackageManager, error) {
	return &WindowsManager{}, nil
}
