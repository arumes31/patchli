package updater

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// DnfManager implements the PackageManager interface for dnf (RHEL/CentOS 8+).
type DnfManager struct{}

func (m *DnfManager) CheckUpdates(ctx context.Context) (UpdateResult, error) {
	cmd := execCommandContext(ctx, "dnf", "check-update")

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	// dnf check-update returns 100 if there are updates available, 0 if not.
	if err != nil && cmd.ProcessState.ExitCode() != 100 {
		return UpdateResult{Success: false, Output: out.String(), Error: err}, err
	}

	return UpdateResult{Success: true, Output: out.String(), Error: nil}, nil
}

func (m *DnfManager) ApplyUpdates(ctx context.Context, packages []string) (UpdateResult, error) {
	args := []string{"upgrade", "-y"}
	if len(packages) > 0 {
		args = append([]string{"install", "-y"}, packages...)
	}

	cmd := execCommandContext(ctx, "dnf", args...)

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	return UpdateResult{Success: err == nil, Output: out.String(), Error: err}, err
}

func (m *DnfManager) RebootRequired() bool {
	cmd := execCommand("needs-restarting", "-r")
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return true
		}
	}

	unameOut, err := execCommand("uname", "-r").Output()
	if err == nil {
		runningKernel := strings.TrimSpace(string(unameOut))
		// Use rpm -q for exact kernel package check instead of strings.Contains
		rpmOut, err := execCommand("rpm", "-q", "kernel-"+runningKernel).Output()
		if err != nil {
			// rpm -q returns non-zero if the package is not installed
			// If the running kernel package is not found, a newer kernel may be installed
			// Check if any kernel package is installed that is different
			lastOut, lastErr := execCommand("rpm", "-q", "--last", "kernel").Output()
			if lastErr == nil {
				lines := strings.Split(strings.TrimSpace(string(lastOut)), "\n")
				if len(lines) > 0 {
					latestKernelLine := strings.Fields(lines[0])
					if len(latestKernelLine) > 0 {
						latestKernel := latestKernelLine[0]
						// kernel-VERSION-ARCH -> extract VERSION-ARCH
						parts := strings.SplitN(latestKernel, "kernel-", 2)
						if len(parts) == 2 && parts[1] != runningKernel {
							return true
						}
					}
				}
			}
		} else {
			// Running kernel package is installed, no reboot needed from this check
			_ = rpmOut
		}
	}

	return false
}

func (m *DnfManager) PreFlightCheck(ctx context.Context) error {
	if err := checkDiskSpaceFunc("/var/lib/patchli", 1024*1024*1024); err != nil {
		return err
	}

	if _, err := statFunc("/var/run/dnf.pid"); err == nil {
		return fmt.Errorf("dnf is currently locked or in use")
	}

	cmd := execCommandContext(ctx, "pgrep", "-x", "dnf")
	if err := cmd.Run(); err == nil {
		return fmt.Errorf("dnf is currently locked or in use")
	}
	return nil
}

func (m *DnfManager) Cleanup(ctx context.Context) error {
	return execCommandContext(ctx, "dnf", "autoremove", "-y").Run()
}
