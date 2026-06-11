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
		rpmOut, err := execCommand("rpm", "-q", "--last", "kernel").Output()
		if err == nil {
			installedKernel := strings.TrimSpace(string(rpmOut))
			if !strings.Contains(installedKernel, runningKernel) {
				return true
			}
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

