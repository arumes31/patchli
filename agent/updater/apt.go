package updater

import (
	"bytes"
	"context"
	"os"
	"os/exec"
)

// AptManager implements the PackageManager interface for apt (Debian/Ubuntu).
type AptManager struct{}

func (m *AptManager) CheckUpdates(ctx context.Context) (UpdateResult, error) {
	cmd := exec.CommandContext(ctx, "apt-get", "update")
	cmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
	
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	if err != nil {
		return UpdateResult{Success: false, Output: out.String(), Error: err}, err
	}

	cmd = exec.CommandContext(ctx, "apt-get", "-s", "upgrade")
	cmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
	
	out.Reset()
	cmd.Stdout = &out
	cmd.Stderr = &out

	err = cmd.Run()
	return UpdateResult{Success: err == nil, Output: out.String(), Error: err}, err
}

func (m *AptManager) ApplyUpdates(ctx context.Context, packages []string) (UpdateResult, error) {
	args := []string{"upgrade", "-y"}
	if len(packages) > 0 {
		args = append([]string{"install", "-y"}, packages...)
	}

	cmd := exec.CommandContext(ctx, "apt-get", args...)
	cmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
	
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	return UpdateResult{Success: err == nil, Output: out.String(), Error: err}, err
}

func (m *AptManager) RebootRequired() bool {
	_, err := os.Stat("/var/run/reboot-required")
	return err == nil
}

func (m *AptManager) PreFlightCheck(ctx context.Context) error {
	// 1. Check disk space (require 1GB)
	if err := CheckDiskSpace(1024 * 1024 * 1024); err != nil {
		return err
	}

	// 2. Process lock detection
	if _, err := os.Stat("/var/lib/dpkg/lock-frontend"); err == nil {
		// Could potentially be locked. Need to check if it's fcntl locked.
		// For simplicity, checking if an apt/dpkg process is running.
		cmd := exec.CommandContext(ctx, "pgrep", "apt|dpkg")
		if err := cmd.Run(); err == nil {
			return fmt.Errorf("package manager is currently locked or in use")
		}
	}
	return nil
}

func (m *AptManager) Cleanup(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "apt-get", "autoremove", "-y")
	cmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
	return cmd.Run()
}
