package updater

import (
	"fmt"
	"os"
	"os/exec"
	"testing"
)

func TestUpdateResult(t *testing.T) {
	res := UpdateResult{Success: true, Output: "done"}
	if !res.Success || res.Output != "done" {
		t.Errorf("Expected success and 'done' output, got Success:%v, Output:%s", res.Success, res.Output)
	}
}

func TestSelfDestructPrivileges(t *testing.T) {
	oldEuid := geteuidFunc
	defer func() { geteuidFunc = oldEuid }()

	geteuidFunc = func() int { return 1000 }
	err := SelfDestruct()
	if err == nil || err.Error() != "self destruct requires root privileges" {
		t.Errorf("Expected root privilege error, got %v", err)
	}
}

func TestDetectPackageManager(t *testing.T) {
	oldStat := statFunc
	defer func() { statFunc = oldStat }()
	oldIsWindows := isWindowsFunc
	defer func() { isWindowsFunc = oldIsWindows }()

	tests := []struct {
		name        string
		mockStat    func(string) (os.FileInfo, error)
		isWindows   bool
		expected    string
		expectError bool
	}{
		{"apk", func(n string) (os.FileInfo, error) {
			if n == "/sbin/apk" { return nil, nil }
			return nil, os.ErrNotExist
		}, false, "*updater.ApkManager", false},
		{"apt", func(n string) (os.FileInfo, error) {
			if n == "/usr/bin/apt-get" { return nil, nil }
			return nil, os.ErrNotExist
		}, false, "*updater.AptManager", false},
		{"dnf", func(n string) (os.FileInfo, error) {
			if n == "/usr/bin/dnf" { return nil, nil }
			return nil, os.ErrNotExist
		}, false, "*updater.DnfManager", false},
		{"yum", func(n string) (os.FileInfo, error) {
			if n == "/usr/bin/yum" { return nil, nil }
			return nil, os.ErrNotExist
		}, false, "*updater.YumManager", false},
		{"pacman", func(n string) (os.FileInfo, error) {
			if n == "/usr/bin/pacman" { return nil, nil }
			return nil, os.ErrNotExist
		}, false, "*updater.PacmanManager", false},
		{"zypper", func(n string) (os.FileInfo, error) {
			if n == "/usr/bin/zypper" { return nil, nil }
			return nil, os.ErrNotExist
		}, false, "*updater.ZypperManager", false},
		{"windows", func(n string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}, true, "*updater.WindowsManager", false},
		{"unsupported", func(n string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}, false, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			statFunc = tt.mockStat
			isWindowsFunc = func() bool { return tt.isWindows }
			pm, err := DetectPackageManager()

			// DetectWindowsManager is platform-dependent in its implementation (via build tags),
			// but we are mocking isWindowsFunc. On non-Windows platforms, DetectWindowsManager
			// returns an error, which we should check if we are simulating Windows.
			if tt.name == "windows" && os.PathSeparator != '\\' {
				if err == nil || err.Error() != "not running on Windows" {
					t.Errorf("Expected 'not running on Windows' error, got %v", err)
				}
				return
			}

			if tt.expectError {
				if err == nil { t.Error("Expected error for unsupported") }
				return
			}
			if err != nil {
				t.Fatalf("Failed to detect %s: %v", tt.name, err)
			}
			if fmt.Sprintf("%T", pm) != tt.expected {
				t.Errorf("Expected %s, got %T", tt.expected, pm)
			}
		})
	}
}

func TestSelfDestruct(t *testing.T) {
	oldEuid := geteuidFunc
	defer func() { geteuidFunc = oldEuid }()
	oldExec := execCommand
	defer func() { execCommand = oldExec }()
	oldStat := statFunc
	defer func() { statFunc = oldStat }()
	oldRemove := removeFunc
	defer func() { removeFunc = oldRemove }()
	oldRemoveAll := removeAllFunc
	defer func() { removeAllFunc = oldRemoveAll }()

	geteuidFunc = func() int { return 0 } // Mock root
	statFunc = func(name string) (os.FileInfo, error) {
		return nil, nil // Pretend everything exists
	}
	execCommand = func(name string, arg ...string) *exec.Cmd {
		return getMockCommandNoCtx(name, arg...)
	}
	removeFunc = func(name string) error { return nil }
	removeAllFunc = func(name string) error { return nil }

	err := SelfDestruct()
	if err != nil {
		t.Errorf("SelfDestruct failed: %v", err)
	}
}
