package updater

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
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
	osExecutableFunc   = os.Executable
	goosFunc           = func() string { return runtime.GOOS }
	checkDiskSpaceFunc = CheckDiskSpace
	isWindowsFunc      = func() bool { return os.PathSeparator == '\\' }
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

// isAdmin checks if the current process has administrator/root privileges.
func isAdmin() bool {
	if goosFunc() == "windows" {
		// On Windows, check for administrator status using net session
		cmd := execCommand("net", "session")
		if err := cmd.Run(); err == nil {
			return true
		}
		return false
	}
	// On Unix, check for root
	return geteuidFunc() == 0
}

// SelfDestruct completely uninstalls the agent, removes its configuration, and stops the service.
func SelfDestruct() error {
	if !isAdmin() {
		return fmt.Errorf("self destruct requires root/administrator privileges")
	}

	var errs []error

	if goosFunc() == "windows" {
		// Windows service cleanup
		_ = execCommand("sc.exe", "stop", "patchli-agent").Run()
		_ = execCommand("sc.exe", "delete", "patchli-agent").Run()

		// Remove configuration and state
		pd := os.Getenv("PROGRAMDATA")
		if pd == "" {
			pd = `C:\ProgramData`
		}
		if err := removeAllFunc(pd + `\Patchli`); err != nil && !os.IsNotExist(err) {
			errs = append(errs, err)
		}
	} else {
		// Unix: Remove systemd service if exists
		if _, err := statFunc("/etc/systemd/system/patchli-agent.service"); err == nil {
			_ = execCommand("systemctl", "stop", "patchli-agent").Run()
			_ = execCommand("systemctl", "disable", "patchli-agent").Run()
			if err := removeFunc("/etc/systemd/system/patchli-agent.service"); err != nil {
				errs = append(errs, err)
			}
			_ = execCommand("systemctl", "daemon-reload").Run()
		}

		// Remove configuration and state
		if err := removeAllFunc("/etc/patchli"); err != nil && !os.IsNotExist(err) {
			errs = append(errs, err)
		}
		if err := removeAllFunc("/var/lib/patchli"); err != nil && !os.IsNotExist(err) {
			errs = append(errs, err)
		}
	}

	// Remove binary (spawn a detached process to delete the binary after a delay)
	var cmd *exec.Cmd
	exePath, err := osExecutableFunc()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %v", err)
	}

	if goosFunc() == "windows" {
		// Use proper quoting to avoid injection
		script := fmt.Sprintf(`ping 127.0.0.1 -n 3 > nul & del /F /Q "%s"`, exePath)
		cmd = execCommand("cmd.exe", "/C", script)
	} else {
		// Single-quote the path on Unix to prevent shell injection
		escapedPath := "'" + strings.ReplaceAll(exePath, "'", "'\\''") + "'"
		script := fmt.Sprintf(`sleep 2; rm -f %s`, escapedPath)
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
		return DetectWindowsManager()
	}

	return nil, fmt.Errorf("unsupported or unknown package manager")
}

// DetectWindowsManager returns the WindowsManager if compiled for Windows, else an error.
// This will be implemented conditionally.
