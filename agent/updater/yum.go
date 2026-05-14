package updater

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
)

// YumManager implements the PackageManager interface for yum (Older RHEL/CentOS).
type YumManager struct{}

func (m *YumManager) CheckUpdates(ctx context.Context) (UpdateResult, error) {
	cmd := exec.CommandContext(ctx, "yum", "check-update", "-y")
	
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	if err != nil && cmd.ProcessState.ExitCode() != 100 {
		return UpdateResult{Success: false, Output: out.String(), Error: err}, err
	}

	return UpdateResult{Success: true, Output: out.String(), Error: nil}, nil
}

func (m *YumManager) ApplyUpdates(ctx context.Context, packages []string) (UpdateResult, error) {
	args := []string{"update", "-y"}
	if len(packages) > 0 {
		args = append([]string{"install", "-y"}, packages...)
	}

	cmd := exec.CommandContext(ctx, "yum", args...)
	
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	return UpdateResult{Success: err == nil, Output: out.String(), Error: err}, err
}

func (m *YumManager) RebootRequired() bool {
	cmd := exec.Command("needs-restarting", "-r")
	err := cmd.Run()
	return err != nil && cmd.ProcessState.ExitCode() == 1
}

func (m *YumManager) PreFlightCheck(ctx context.Context) error {
	if err := CheckDiskSpace("/var/lib/patchli", 1024*1024*1024); err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, "pgrep", "yum")
	if err := cmd.Run(); err == nil {
		return fmt.Errorf("yum is currently locked or in use")
	}
	return nil
}

func (m *YumManager) Cleanup(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "sh", "-c", "yum help | grep -q autoremove")
	if err := cmd.Run(); err == nil {
		return exec.CommandContext(ctx, "yum", "autoremove", "-y").Run()
	}
	return exec.CommandContext(ctx, "yum", "clean", "all").Run()
}
