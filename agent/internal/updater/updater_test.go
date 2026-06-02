package updater

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
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

	// Only testable on non-windows if we want to see the error
	// But since we mocked it, we can force it
	geteuidFunc = func() int { return 1000 }
	err := SelfDestruct()

	// If we are on windows, this should now be nil (no root check)
	// If not on windows, it should be the privilege error
	if os.PathSeparator == '/' { // Simple Unix check
		if err == nil || err.Error() != "self destruct requires root privileges" {
			t.Errorf("Expected root privilege error on Unix, got %v", err)
		}
	} else {
		if err != nil {
			t.Errorf("Expected no root privilege error on Windows, got %v", err)
		}
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
			if n == "/sbin/apk" {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}, "*updater.ApkManager"},
		{"apt", func(n string) (os.FileInfo, error) {
			if n == "/usr/bin/apt-get" {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}, "*updater.AptManager"},
		{"dnf", func(n string) (os.FileInfo, error) {
			if n == "/usr/bin/dnf" {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}, "*updater.DnfManager"},
		{"yum", func(n string) (os.FileInfo, error) {
			if n == "/usr/bin/yum" {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}, "*updater.YumManager"},
		{"pacman", func(n string) (os.FileInfo, error) {
			if n == "/usr/bin/pacman" {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}, "*updater.PacmanManager"},
		{"zypper", func(n string) (os.FileInfo, error) {
			if n == "/usr/bin/zypper" {
				return nil, nil
			}
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
				if os.PathSeparator == '\\' {
					if err != nil {
						t.Errorf("Expected success on Windows, got %v", err)
					}
					return
				}
				if err == nil {
					t.Error("Expected error for unsupported")
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
	oldExec := execCommand
	oldStat := statFunc
	oldRemove := removeFunc
	oldRemoveAll := removeAllFunc
	oldExecutable := executableFunc
	oldDetach := detachProcessFunc
	defer func() {
		geteuidFunc = oldEuid
		execCommand = oldExec
		statFunc = oldStat
		removeFunc = oldRemove
		removeAllFunc = oldRemoveAll
		executableFunc = oldExecutable
		detachProcessFunc = oldDetach
	}()

	tests := []struct {
		name           string
		geteuid        func() int
		stat           func(string) (os.FileInfo, error)
		exec           func(string, ...string) *exec.Cmd
		remove         func(string) error
		removeAll      func(string) error
		executable     func() (string, error)
		detach         func(*exec.Cmd)
		expectedErrSub string
	}{
		{
			name:       "success",
			geteuid:    func() int { return 0 },
			stat:       func(string) (os.FileInfo, error) { return nil, nil },
			exec:       func(n string, a ...string) *exec.Cmd { return getMockCommandNoCtx(n, a...) },
			remove:     func(string) error { return nil },
			removeAll:  func(string) error { return nil },
			executable: func() (string, error) { return "/path/to/exe", nil },
			detach:     func(*exec.Cmd) {},
		},
		{
			name:           "remove_service_fail",
			geteuid:        func() int { return 0 },
			stat:           func(string) (os.FileInfo, error) { return nil, nil },
			exec:           func(n string, a ...string) *exec.Cmd { return getMockCommandNoCtx(n, a...) },
			remove:         func(string) error { return errors.New("remove failed") },
			removeAll:      func(string) error { return nil },
			executable:     func() (string, error) { return "/path/to/exe", nil },
			detach:         func(*exec.Cmd) {},
			expectedErrSub: "remove failed",
		},
		{
			name:    "remove_config_fail",
			geteuid: func() int { return 0 },
			stat:    func(string) (os.FileInfo, error) { return nil, nil },
			exec:    func(n string, a ...string) *exec.Cmd { return getMockCommandNoCtx(n, a...) },
			remove:  func(string) error { return nil },
			removeAll: func(path string) error {
				if path == "/etc/patchli" {
					return errors.New("remove config failed")
				}
				return nil
			},
			executable:     func() (string, error) { return "/path/to/exe", nil },
			detach:         func(*exec.Cmd) {},
			expectedErrSub: "remove config failed",
		},
		{
			name:    "remove_state_fail",
			geteuid: func() int { return 0 },
			stat:    func(string) (os.FileInfo, error) { return nil, nil },
			exec:    func(n string, a ...string) *exec.Cmd { return getMockCommandNoCtx(n, a...) },
			remove:  func(string) error { return nil },
			removeAll: func(path string) error {
				if path == "/var/lib/patchli" {
					return errors.New("remove state failed")
				}
				return nil
			},
			executable:     func() (string, error) { return "/path/to/exe", nil },
			detach:         func(*exec.Cmd) {},
			expectedErrSub: "remove state failed",
		},
		{
			name:           "executable_fail",
			geteuid:        func() int { return 0 },
			stat:           func(string) (os.FileInfo, error) { return nil, nil },
			exec:           func(n string, a ...string) *exec.Cmd { return getMockCommandNoCtx(n, a...) },
			remove:         func(string) error { return nil },
			removeAll:      func(string) error { return nil },
			executable:     func() (string, error) { return "", errors.New("exec path failed") },
			detach:         func(*exec.Cmd) {},
			expectedErrSub: "exec path failed",
		},
		{
			name:    "cmd_start_fail",
			geteuid: func() int { return 0 },
			stat:    func(string) (os.FileInfo, error) { return nil, nil },
			exec: func(n string, a ...string) *exec.Cmd {
				return exec.Command("non-existent-command-12345")
			},
			remove:         func(string) error { return nil },
			removeAll:      func(string) error { return nil },
			executable:     func() (string, error) { return "/path/to/exe", nil },
			detach:         func(*exec.Cmd) {},
			expectedErrSub: "executable file not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			geteuidFunc = tt.geteuid
			statFunc = tt.stat
			execCommand = tt.exec
			removeFunc = tt.remove
			removeAllFunc = tt.removeAll
			executableFunc = tt.executable
			detachProcessFunc = tt.detach

			err := SelfDestruct()
			if tt.expectedErrSub == "" {
				if err != nil {
					t.Errorf("Expected no error, got %v", err)
				}
			} else {
				if err == nil {
					t.Errorf("Expected error containing %q, got nil", tt.expectedErrSub)
				} else if !strings.Contains(err.Error(), tt.expectedErrSub) {
					t.Errorf("Expected error containing %q, got %v", tt.expectedErrSub, err)
				}
			}
		})
	}
}
