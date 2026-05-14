package updater

import (
	"bytes"
	"context"
	"fmt"
)

// DnfManager implements the PackageManager interface for dnf (RHEL/CentOS 8+).
type DnfManager struct{}

func (m *DnfManager) CheckUpdates(ctx context.Context) (UpdateResult, error) {
	cmd := execCommandContext(ctx, "dnf", "check-update", "-y")
	
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
	cmd := execCommand("dnf", "needs-restarting", "-r")
	err := cmd.Run()
	return err != nil && cmd.ProcessState.ExitCode() == 1 // 1 means reboot required
}

func (m *DnfManager) PreFlightCheck(ctx context.Context) error {
	if err := CheckDiskSpace("/var/lib/patchli", 1024*1024*1024); err != nil {
		return err
	}

	cmd := execCommandContext(ctx, "pgrep", "dnf")
	if err := cmd.Run(); err == nil {
		return fmt.Errorf("dnf is currently locked or in use")
	}
	return nil
}

func (m *DnfManager) Cleanup(ctx context.Context) error {
	return execCommandContext(ctx, "dnf", "autoremove", "-y").Run()
}
