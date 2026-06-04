package updater

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

// UpdateResult represents the outcome of an update operation.
type UpdateResult struct {
	Success bool
	Output  string
	Error   error
}

var (
	execCommand        = exec.Command
	execCommandContext = exec.CommandContext
	statFunc           = os.Stat
	geteuidFunc        = os.Geteuid
	removeFunc         = os.Remove
	removeAllFunc      = os.RemoveAll
	isWindowsFunc      = func() bool { return os.PathSeparator == '\\' }
	detectWindowsManagerFunc = DetectWindowsManager
	checkDiskSpaceFunc = CheckDiskSpace
)

// PackageManager defines the interface for OS-specific package managers.
type PackageManager interface {
	// CheckUpdates checks for available updates.
	CheckUpdates(ctx context.Context) (UpdateResult, error)
	// ApplyUpdates applies all available updates or specific packages.
	ApplyUpdates(ctx context.Context, packages []string) (UpdateResult, error)
	// RebootRequired checks if a reboot is needed after updates.
	RebootRequired() bool
	// PreFlightCheck verifies conditions (e.g. disk space, process locks) before patching.
	PreFlightCheck(ctx context.Context) error
	// Cleanup removes orphaned packages or caches.
	Cleanup(ctx context.Context) error
}

// SelfDestruct completely uninstalls the agent, removes its configuration, and stops the service.
func SelfDestruct() error {
	if geteuidFunc() != 0 {
		return fmt.Errorf("self destruct requires root privileges")
	}

	var errs []error

	// 1. Remove systemd service if exists
	if _, err := statFunc("/etc/systemd/system/patchli-agent.service"); err == nil {
		_ = execCommand("systemctl", "stop", "patchli-agent").Run()
		_ = execCommand("systemctl", "disable", "patchli-agent").Run()
		if err := removeFunc("/etc/systemd/system/patchli-agent.service"); err != nil {
			errs = append(errs, err)
		}
		_ = execCommand("systemctl", "daemon-reload").Run()
	}

	// 2. Remove configuration and state
	if err := removeAllFunc("/etc/patchli"); err != nil {
		errs = append(errs, err)
	}
	if err := removeAllFunc("/var/lib/patchli"); err != nil {
		errs = append(errs, err)
	}

	// 3. Remove binary (spawn a detached process to delete the binary after a delay)
	var cmd *exec.Cmd
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %v", err)
	}

	if runtime.GOOS == "windows" {
		script := fmt.Sprintf(`ping 127.0.0.1 -n 3 > nul & del /F /Q "%s"`, exePath)
		cmd = execCommand("cmd.exe", "/C", script)
	} else {
		script := fmt.Sprintf(`sleep 2; rm -f "%s"`, exePath)
		cmd = execCommand("sh", "-c", script)
	}
	detachProcess(cmd)
	if err := cmd.Start(); err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return fmt.Errorf("self destruct completed with errors: %v", errs)
	}
	return nil
}

// DetectPackageManager determines the underlying OS package manager.
func DetectPackageManager() (PackageManager, error) {
	// Check for apk (Alpine)
	if _, err := statFunc("/sbin/apk"); err == nil {
		return &ApkManager{}, nil
	}

	// Check for apt (Debian/Ubuntu)
	if _, err := statFunc("/usr/bin/apt-get"); err == nil {
		return &AptManager{}, nil
	}

	// Check for dnf (RHEL/CentOS/Rocky/Alma 8+)
	if _, err := statFunc("/usr/bin/dnf"); err == nil {
		return &DnfManager{}, nil
	}

	// Check for yum (Older RHEL/CentOS)
	if _, err := statFunc("/usr/bin/yum"); err == nil {
		return &YumManager{}, nil
	}

	// Check for pacman (Arch Linux)
	if _, err := statFunc("/usr/bin/pacman"); err == nil {
		return &PacmanManager{}, nil
	}

	// Check for zypper (SUSE)
	if _, err := statFunc("/usr/bin/zypper"); err == nil {
		return &ZypperManager{}, nil
	}

	// Check if running on Windows
	if isWindowsFunc() {
		return detectWindowsManagerFunc()
	}

	return nil, fmt.Errorf("unsupported or unknown package manager")
}

// DetectWindowsManager returns the WindowsManager if compiled for Windows, else an error.
// This will be implemented conditionally.
