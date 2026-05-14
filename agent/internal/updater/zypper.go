package updater

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
)

// ZypperManager implements the PackageManager interface for zypper (SUSE).
type ZypperManager struct{}

func (m *ZypperManager) CheckUpdates(ctx context.Context) (UpdateResult, error) {
	cmd := exec.CommandContext(ctx, "zypper", "--non-interactive", "refresh")
	
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	if err != nil {
		return UpdateResult{Success: false, Output: out.String(), Error: err}, err
	}

	cmd = exec.CommandContext(ctx, "zypper", "--non-interactive", "list-updates")
	out.Reset()
	cmd.Stdout = &out
	cmd.Stderr = &out

	err = cmd.Run()
	if err != nil {
		return UpdateResult{Success: false, Output: out.String(), Error: err}, err
	}
	return UpdateResult{Success: true, Output: out.String(), Error: nil}, nil
}

func (m *ZypperManager) ApplyUpdates(ctx context.Context, packages []string) (UpdateResult, error) {
	args := []string{"--non-interactive", "update"}
	if len(packages) > 0 {
		args = append([]string{"--non-interactive", "install"}, packages...)
	}

	cmd := exec.CommandContext(ctx, "zypper", args...)
	
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	return UpdateResult{Success: err == nil, Output: out.String(), Error: err}, err
}

func (m *ZypperManager) RebootRequired() bool {
	cmd := exec.Command("zypper", "needs-rebooting")
	err := cmd.Run()
	return err != nil && cmd.ProcessState.ExitCode() == 102
}

func (m *ZypperManager) PreFlightCheck(ctx context.Context) error {
	if err := CheckDiskSpace("/var/lib/patchli", 1024*1024*1024); err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, "pgrep", "-x", "zypper")
	if err := cmd.Run(); err == nil {
		return fmt.Errorf("zypper is currently locked or in use")
	}
	return nil
}

func (m *ZypperManager) Cleanup(ctx context.Context) error {
	return exec.CommandContext(ctx, "zypper", "clean").Run()
}
