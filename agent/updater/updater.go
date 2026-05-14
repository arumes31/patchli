package updater

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// UpdateResult represents the outcome of an update operation.
type UpdateResult struct {
	Success bool
	Output  string
	Error   error
}

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

// CheckDiskSpace ensures there is at least minBytes available on the root partition.
func CheckDiskSpace(minBytes uint64) error {
	var stat syscall.Statfs_t
	err := syscall.Statfs("/", &stat)
	if err != nil {
		return fmt.Errorf("failed to check disk space: %v", err)
	}

	// Available blocks * size per block
	available := stat.Bavail * uint64(stat.Bsize)
	if available < minBytes {
		return fmt.Errorf("insufficient disk space: required %d bytes, available %d bytes", minBytes, available)
	}
	return nil
}

// SelfDestruct completely uninstalls the agent, removes its configuration, and stops the service.
func SelfDestruct() error {
	// 1. Remove systemd service if exists
	if _, err := os.Stat("/etc/systemd/system/patchli-agent.service"); err == nil {
		exec.Command("systemctl", "stop", "patchli-agent").Run()
		exec.Command("systemctl", "disable", "patchli-agent").Run()
		os.Remove("/etc/systemd/system/patchli-agent.service")
		exec.Command("systemctl", "daemon-reload").Run()
	}

	// 2. Remove configuration and state
	os.RemoveAll("/etc/patchli")
	os.RemoveAll("/var/lib/patchli")

	// 3. Remove binary (this will fail if the binary is currently executing, so it should be the last step, or handled by an external script)
	// A robust self-destruct might spawn a detached process to delete the binary.
	go func() {
		os.Remove("/usr/local/bin/patchli-agent")
		os.Exit(0)
	}()

	return nil
}

// DetectPackageManager determines the underlying OS package manager.
func DetectPackageManager() (PackageManager, error) {
	// Check for apk (Alpine)
	if _, err := os.Stat("/sbin/apk"); err == nil {
		return &ApkManager{}, nil
	}
	
	// Check for apt (Debian/Ubuntu)
	if _, err := os.Stat("/usr/bin/apt-get"); err == nil {
		return &AptManager{}, nil
	}

	// Check for dnf (RHEL/CentOS/Rocky/Alma 8+)
	if _, err := os.Stat("/usr/bin/dnf"); err == nil {
		return &DnfManager{}, nil
	}

	// Check for yum (Older RHEL/CentOS)
	if _, err := os.Stat("/usr/bin/yum"); err == nil {
		return &YumManager{}, nil
	}

	// Check for pacman (Arch Linux)
	if _, err := os.Stat("/usr/bin/pacman"); err == nil {
		return &PacmanManager{}, nil
	}

	// Check for zypper (SUSE)
	if _, err := os.Stat("/usr/bin/zypper"); err == nil {
		return &ZypperManager{}, nil
	}

	// Check if running on Windows
	if os.PathSeparator == '\\' {
		return DetectWindowsManager()
	}

	return nil, fmt.Errorf("unsupported or unknown package manager")
}

// DetectWindowsManager returns the WindowsManager if compiled for Windows, else an error.
// This will be implemented conditionally.
