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

	cmd = execCommandContext(ctx, "apk", "upgrade", "--simulate")
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

	cmd := execCommandContext(ctx, "apk", args...)
	
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	return UpdateResult{Success: err == nil, Output: out.String(), Error: err}, err
}

func (m *ApkManager) RebootRequired() bool {
	_, err := statFunc("/run/reboot-required")
	return err == nil
}

func (m *ApkManager) PreFlightCheck(ctx context.Context) error {
	// Check disk space (require 500MB for Alpine)
	if err := CheckDiskSpace("/var/lib/patchli", 500*1024*1024); err != nil {
		return err
	}

	// 2. Process lock detection
	if _, err := statFunc("/lib/apk/db/lock"); err == nil {
		cmd := execCommandContext(ctx, "pgrep", "apk")
		if err := cmd.Run(); err == nil {
			return fmt.Errorf("apk package manager is currently locked or in use")
		}
	}
	return nil
}

func (m *ApkManager) Cleanup(ctx context.Context) error {
	cmd := execCommandContext(ctx, "apk", "cache", "clean")
	return cmd.Run()
}
