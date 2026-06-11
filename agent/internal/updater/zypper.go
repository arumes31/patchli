package updater

import (
	"bytes"
	"context"
	"fmt"
	
)

// ZypperManager implements the PackageManager interface for zypper (SUSE).
type ZypperManager struct{}

func (m *ZypperManager) CheckUpdates(ctx context.Context) (UpdateResult, error) {
	cmd := execCommandContext(ctx, "zypper", "--non-interactive", "refresh")
	
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	if err != nil {
		return UpdateResult{Success: false, Output: out.String(), Error: err}, err
	}

	cmd = execCommandContext(ctx, "zypper", "--non-interactive", "list-updates")
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

	cmd := execCommandContext(ctx, "zypper", args...)
	
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	return UpdateResult{Success: err == nil, Output: out.String(), Error: err}, err
}

func (m *ZypperManager) RebootRequired() bool {
	cmd := execCommandContext(context.Background(), "zypper", "needs-rebooting")
	err := cmd.Run()
	return err != nil && cmd.ProcessState.ExitCode() == 102
}

func (m *ZypperManager) PreFlightCheck(ctx context.Context) error {
	if err := checkDiskSpaceFunc("/var/lib/patchli", 1024*1024*1024); err != nil {
		return err
	}

	if _, err := statFunc("/var/run/zypp.pid"); err == nil {
		return fmt.Errorf("zypper is currently locked")
	}
	return nil
}

func (m *ZypperManager) Cleanup(ctx context.Context) error {
	return execCommandContext(ctx, "zypper", "clean", "-a").Run()
}

