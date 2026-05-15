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

	tests := []struct {
		name     string
		mockStat func(string) (os.FileInfo, error)
		expected string
	}{
		{"apk", func(n string) (os.FileInfo, error) {
			if n == "/sbin/apk" { return nil, nil }
			return nil, os.ErrNotExist
		}, "*updater.ApkManager"},
		{"apt", func(n string) (os.FileInfo, error) {
			if n == "/usr/bin/apt-get" { return nil, nil }
			return nil, os.ErrNotExist
		}, "*updater.AptManager"},
		{"dnf", func(n string) (os.FileInfo, error) {
			if n == "/usr/bin/dnf" { return nil, nil }
			return nil, os.ErrNotExist
		}, "*updater.DnfManager"},
		{"yum", func(n string) (os.FileInfo, error) {
			if n == "/usr/bin/yum" { return nil, nil }
			return nil, os.ErrNotExist
		}, "*updater.YumManager"},
		{"pacman", func(n string) (os.FileInfo, error) {
			if n == "/usr/bin/pacman" { return nil, nil }
			return nil, os.ErrNotExist
		}, "*updater.PacmanManager"},
		{"zypper", func(n string) (os.FileInfo, error) {
			if n == "/usr/bin/zypper" { return nil, nil }
			return nil, os.ErrNotExist
		}, "*updater.ZypperManager"},
		{"unsupported", func(n string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			statFunc = tt.mockStat
			pm, err := DetectPackageManager()
			if tt.name == "unsupported" {
				// On Windows, it will fall back to DetectWindowsManager which returns &WindowsManager{}
				// So if we are on Windows, this test might need adjustment.
				if os.PathSeparator == '\\' {
					if err != nil { t.Errorf("Expected success on Windows, got %v", err) }
					return
				}
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
