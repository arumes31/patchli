package updater

import (
	"bytes"
	"context"
	"fmt"
)

// ApkManager implements the PackageManager interface for apk (Alpine).
type ApkManager struct{}

func (m *ApkManager) CheckUpdates(ctx context.Context) (UpdateResult, error) {
	cmd := execCommandContext(ctx, "apk", "update")

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	if err != nil {
		return UpdateResult{Success: false, Output: out.String(), Error: err}, err
	}

	cmd = execCommandContext(ctx, "apk", "version", "-l", "<")
	out.Reset()
	cmd.Stdout = &out
	cmd.Stderr = &out

	err = cmd.Run()
	if err != nil {
		return UpdateResult{Success: false, Output: out.String(), Error: err}, err
	}
	return UpdateResult{Success: true, Output: out.String(), Error: nil}, nil
}

func (m *ApkManager) ApplyUpdates(ctx context.Context, packages []string) (UpdateResult, error) {
	args := []string{"upgrade"}
	if len(packages) > 0 {
		args = append([]string{"add"}, packages...)
	}

	cmd := execCommandContext(ctx, "apk", args...)

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	return UpdateResult{Success: err == nil, Output: out.String(), Error: err}, err
}

func (m *ApkManager) RebootRequired() bool {
	// Alpine doesn't have a standard reboot-required file like Debian.
	return false
}

func (m *ApkManager) PreFlightCheck(ctx context.Context) error {
	// Check disk space (require 500MB for Alpine)
	if err := checkDiskSpaceFunc("/var/lib/patchli", 500*1024*1024); err != nil {
		return err
	}

	if _, err := statFunc("/var/run/apk.lock"); err == nil {
		return fmt.Errorf("package manager is currently locked")
	}
	return nil
}

func (m *ApkManager) Cleanup(ctx context.Context) error {
	cmd := execCommandContext(ctx, "apk", "cache", "clean")
	return cmd.Run()
}
