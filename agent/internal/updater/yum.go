package updater

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// YumManager implements the PackageManager interface for yum (Older RHEL/CentOS).
type YumManager struct{}

func (m *YumManager) CheckUpdates(ctx context.Context) (UpdateResult, error) {
	cmd := execCommandContext(ctx, "yum", "check-update", "-y")
	
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
		args = append(args, packages...)
	}

	cmd := execCommandContext(ctx, "yum", args...)
	
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	return UpdateResult{Success: err == nil, Output: out.String(), Error: err}, err
}

func (m *YumManager) RebootRequired() bool {
	if _, err := statFunc("/var/run/reboot-required"); err == nil {
		return true
	}
	
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

func (m *YumManager) PreFlightCheck(ctx context.Context) error {
	if err := CheckDiskSpace("/var/lib/patchli", 1024*1024*1024); err != nil {
		return err
	}
	if _, err := statFunc("/var/run/yum.pid"); err == nil {
		return fmt.Errorf("yum is currently locked or in use")
	}

	cmd := execCommandContext(ctx, "pgrep", "-x", "yum")
	if err := cmd.Run(); err == nil {
		return fmt.Errorf("yum is currently locked or in use")
	}
	return nil
}

func (m *YumManager) Cleanup(ctx context.Context) error {
	cmd := execCommandContext(ctx, "sh", "-c", "yum help | grep -q autoremove")
	if err := cmd.Run(); err == nil {
		return execCommandContext(ctx, "yum", "autoremove", "-y").Run()
	}
	return execCommandContext(ctx, "yum", "clean", "all").Run()
}

