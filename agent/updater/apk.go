package updater

import (
	"bytes"
	"context"
	"os/exec"
)

// ApkManager implements the PackageManager interface for apk (Alpine).
type ApkManager struct{}

func (m *ApkManager) CheckUpdates(ctx context.Context) (UpdateResult, error) {
	cmd := exec.CommandContext(ctx, "apk", "update")
	
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	if err != nil {
		return UpdateResult{Success: false, Output: out.String(), Error: err}, err
	}

	cmd = exec.CommandContext(ctx, "apk", "version", "-l", "<")
	out.Reset()
	cmd.Stdout = &out
	cmd.Stderr = &out

	err = cmd.Run()
	return UpdateResult{Success: err == nil, Output: out.String(), Error: err}, err
}

func (m *ApkManager) ApplyUpdates(ctx context.Context, packages []string) (UpdateResult, error) {
	args := []string{"upgrade"}
	if len(packages) > 0 {
		args = append([]string{"add"}, packages...)
	}

	cmd := exec.CommandContext(ctx, "apk", args...)
	
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	return UpdateResult{Success: err == nil, Output: out.String(), Error: err}, err
}

func (m *ApkManager) RebootRequired() bool {
	// Alpine doesn't have a standard reboot-required file like Debian.
	// This would require a more sophisticated check, e.g. comparing kernel versions.
	// For simplicity, we return false or implement a custom check.
	return false
}
