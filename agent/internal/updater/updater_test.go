package updater

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
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
	oldGoos := goosFunc
	defer func() { goosFunc = oldGoos }()
	oldExec := execCommand
	defer func() { execCommand = oldExec }()

	// Test Unix non-root
	goosFunc = func() string { return "linux" }
	geteuidFunc = func() int { return 1000 }
	err := SelfDestruct()
	if err == nil || err.Error() != "self destruct requires root/administrator privileges" {
		t.Errorf("Expected root privilege error, got %v", err)
	}

	// Test Windows non-admin
	goosFunc = func() string { return "windows" }
	execCommand = func(name string, arg ...string) *exec.Cmd {
		if name == "net" {
			return exec.Command("false") // simulates non-admin
		}
		return getMockCommandNoCtx(name, arg...)
	}
	err = SelfDestruct()
	if err == nil || err.Error() != "self destruct requires root/administrator privileges" {
		t.Errorf("Expected admin privilege error, got %v", err)
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
			if n == "/sbin/apk" {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}, false, "*updater.ApkManager", false},
		{"apt", func(n string) (os.FileInfo, error) {
			if n == "/usr/bin/apt-get" {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}, false, "*updater.AptManager", false},
		{"dnf", func(n string) (os.FileInfo, error) {
			if n == "/usr/bin/dnf" {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}, false, "*updater.DnfManager", false},
		{"yum", func(n string) (os.FileInfo, error) {
			if n == "/usr/bin/yum" {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}, false, "*updater.YumManager", false},
		{"pacman", func(n string) (os.FileInfo, error) {
			if n == "/usr/bin/pacman" {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}, false, "*updater.PacmanManager", false},
		{"zypper", func(n string) (os.FileInfo, error) {
			if n == "/usr/bin/zypper" {
				return nil, nil
			}
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
			if tt.name == "windows" && runtime.GOOS != "windows" {
				t.Skip("Windows package manager is available only in a Windows build")
			}
			statFunc = tt.mockStat
			isWindowsFunc = func() bool { return tt.isWindows }
			pm, err := DetectPackageManager()
			if tt.expectError {
				if err == nil {
					t.Error("Expected error, got nil")
				}
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
	oldExe := osExecutableFunc
	defer func() { osExecutableFunc = oldExe }()
	oldGoos := goosFunc
	defer func() { goosFunc = oldGoos }()

	geteuidFunc = func() int { return 0 }
	osExecutableFunc = func() (string, error) { return "/path/to/exe", nil }

	t.Run("FullSuccessLinux", func(t *testing.T) {
		goosFunc = func() string { return "linux" }
		statFunc = func(name string) (os.FileInfo, error) {
			if name == "/etc/systemd/system/patchli-agent.service" {
				return nil, nil
			}
			return nil, os.ErrNotExist
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
	})

	t.Run("FullSuccessWindows", func(t *testing.T) {
		goosFunc = func() string { return "windows" }
		statFunc = func(name string) (os.FileInfo, error) { return nil, os.ErrNotExist }
		execCommand = func(name string, arg ...string) *exec.Cmd {
			return getMockCommandNoCtx(name, arg...)
		}
		removeFunc = func(name string) error { return nil }
		removeAllFunc = func(name string) error { return nil }

		err := SelfDestruct()
		if err != nil {
			t.Errorf("SelfDestruct failed: %v", err)
		}
	})

	t.Run("ExecutableFailure", func(t *testing.T) {
		osExecutableFunc = func() (string, error) { return "", fmt.Errorf("exe error") }
		err := SelfDestruct()
		if err == nil || err.Error() != "failed to get executable path: exe error" {
			t.Errorf("Expected executable error, got %v", err)
		}
		osExecutableFunc = func() (string, error) { return "/path/to/exe", nil }
	})

	t.Run("PartialFailures", func(t *testing.T) {
		goosFunc = func() string { return "linux" }
		statFunc = func(name string) (os.FileInfo, error) {
			if name == "/etc/systemd/system/patchli-agent.service" {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}
		execCommand = func(name string, arg ...string) *exec.Cmd {
			return getMockCommandNoCtx(name, arg...)
		}
		removeFunc = func(name string) error {
			if name == "/etc/systemd/system/patchli-agent.service" {
				return fmt.Errorf("remove error")
			}
			return nil
		}
		removeAllFunc = func(name string) error {
			if name == "/etc/patchli" {
				return fmt.Errorf("removeAll error")
			}
			return nil
		}

		err := SelfDestruct()
		if err == nil {
			t.Error("Expected failure due to removal errors")
		} else {
			if !strings.Contains(err.Error(), "remove error") || !strings.Contains(err.Error(), "removeAll error") {
				t.Errorf("Expected specific errors in aggregated error message, got: %v", err)
			}
		}
	})

	t.Run("StartFailure", func(t *testing.T) {
		goosFunc = func() string { return "linux" }
		statFunc = func(name string) (os.FileInfo, error) { return nil, os.ErrNotExist }
		removeFunc = func(name string) error { return nil }
		removeAllFunc = func(name string) error { return nil }
		execCommand = func(name string, arg ...string) *exec.Cmd {
			if name == "sh" || name == "cmd.exe" {
				return exec.Command("nonexistent-command-that-fails-to-start")
			}
			return getMockCommandNoCtx(name, arg...)
		}

		err := SelfDestruct()
		if err == nil {
			t.Error("Expected failure due to cmd.Start failure")
		}
	})
}

func TestSelfDestruct_SystemdFailures(t *testing.T) {
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
	oldExe := osExecutableFunc
	defer func() { osExecutableFunc = oldExe }()
	oldGoos := goosFunc
	defer func() { goosFunc = oldGoos }()

	geteuidFunc = func() int { return 0 }
	osExecutableFunc = func() (string, error) { return "/path/to/exe", nil }
	goosFunc = func() string { return "linux" }
	statFunc = func(name string) (os.FileInfo, error) {
		if name == "/etc/systemd/system/patchli-agent.service" {
			return nil, nil
		}
		return nil, os.ErrNotExist
	}
	execCommand = func(name string, arg ...string) *exec.Cmd {
		return getMockCommandNoCtx(name, arg...)
	}
	removeFunc = func(name string) error { return nil }
	removeAllFunc = func(name string) error { return nil }

	err := SelfDestruct()
	if err != nil {
		t.Errorf("Expected success even if systemctl fails (it ignores Run() errors), got %v", err)
	}
}
