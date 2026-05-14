package updater

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
)

// PacmanManager implements the PackageManager interface for pacman (Arch Linux).
type PacmanManager struct{}

func (m *PacmanManager) CheckUpdates(ctx context.Context) (UpdateResult, error) {
	cmd := exec.CommandContext(ctx, "pacman", "-Sy")
	
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	if err != nil {
		return UpdateResult{Success: false, Output: out.String(), Error: err}, err
	}

	cmd = exec.CommandContext(ctx, "pacman", "-Qu")
	out.Reset()
	cmd.Stdout = &out
	cmd.Stderr = &out

	err = cmd.Run()
	if err != nil {
		return UpdateResult{Success: false, Output: out.String(), Error: err}, err
	}
	return UpdateResult{Success: true, Output: out.String(), Error: nil}, nil
}

func (m *PacmanManager) ApplyUpdates(ctx context.Context, packages []string) (UpdateResult, error) {
	args := []string{"-Su", "--noconfirm"}
	if len(packages) > 0 {
		args = append([]string{"-S", "--noconfirm"}, packages...)
	}

	cmd := exec.CommandContext(ctx, "pacman", args...)
	
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	return UpdateResult{Success: err == nil, Output: out.String(), Error: err}, err
}

func (m *PacmanManager) RebootRequired() bool {
	// Arch Linux usually requires reboot if kernel or systemd is updated.
	// We can check if /boot/vmlinuz-linux is newer than current running kernel,
	// but a simpler check is looking for a reboot-required flag if managed by external tools.
	// For raw pacman, there is no built-in reboot-required file.
	return false
}

func (m *PacmanManager) PreFlightCheck(ctx context.Context) error {
	if err := CheckDiskSpace("/var/lib/patchli", 1024*1024*1024); err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, "pgrep", "pacman")
	if err := cmd.Run(); err == nil {
		return fmt.Errorf("pacman is currently locked or in use")
	}
	return nil
}

func (m *PacmanManager) Cleanup(ctx context.Context) error {
	return exec.CommandContext(ctx, "pacman", "-Sc", "--noconfirm").Run()
}
