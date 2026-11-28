package updater

import (
	"bytes"
	"context"
	"fmt"
	"os"

	"strings"
)

// AptManager implements the PackageManager interface for apt (Debian/Ubuntu).
type AptManager struct{}

func (m *AptManager) CheckUpdates(ctx context.Context) (UpdateResult, error) {
	cmd := execCommandContext(ctx, "apt-get", "update")
	cmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	if err != nil {
		return UpdateResult{Success: false, Output: out.String(), Error: err}, err
	}

	// Simulate the same upgrade ApplyUpdates runs, so the count includes
	// packages that need new dependencies (new kernels).
	cmd = execCommandContext(ctx, "apt-get", "-s", "upgrade", "--with-new-pkgs")
	cmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")

	out.Reset()
	cmd.Stdout = &out
	cmd.Stderr = &out

	err = cmd.Run()
	if err != nil {
		return UpdateResult{Success: false, Output: out.String(), Error: err}, err
	}
	return UpdateResult{Success: err == nil, Output: out.String(), Error: err}, err
}

func (m *AptManager) ApplyUpdates(ctx context.Context, packages []string) (UpdateResult, error) {
	// Feature 1: Dependency-aware ordering (Kernel should typically be last)
	ordered := orderPackages(packages)

	// --with-new-pkgs installs new dependencies such as a new kernel ABI,
	// which plain "upgrade" holds back; it never removes packages. Keep
	// local config files instead of prompting.
	args := append([]string{"-y", "-o", "Dpkg::Options::=--force-confdef", "-o", "Dpkg::Options::=--force-confold"}, "upgrade", "--with-new-pkgs")
	if len(ordered) > 0 {
		args = append([]string{"-y", "-o", "Dpkg::Options::=--force-confdef", "-o", "Dpkg::Options::=--force-confold", "install"}, ordered...)
	}

	var out bytes.Buffer
	update := execCommandContext(ctx, "apt-get", "update")
	update.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
	update.Stdout = &out
	update.Stderr = &out
	if err := update.Run(); err != nil {
		return UpdateResult{Success: false, Output: out.String(), Error: err}, err
	}

	cmd := execCommandContext(ctx, "apt-get", args...)
	cmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive", "NEEDRESTART_MODE=a")
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	return UpdateResult{Success: err == nil, Output: out.String(), Error: err}, err
}

func orderPackages(packages []string) []string {
	if len(packages) == 0 {
		return packages
	}

	var normal, late []string
	for _, p := range packages {
		if strings.Contains(p, "linux-image") || strings.Contains(p, "kernel") {
			late = append(late, p)
		} else {
			normal = append(normal, p)
		}
	}
	return append(normal, late...)
}

func (m *AptManager) RebootRequired() bool {
	_, err := statFunc("/var/run/reboot-required")
	return err == nil
}

func (m *AptManager) PreFlightCheck(ctx context.Context) error {
	// 1. Check disk space (require 1GB)
	if err := checkDiskSpaceFunc("/var/lib/rpatch", 1024*1024*1024); err != nil {
		return err
	}

	// 2. Process lock detection
	if _, err := statFunc("/var/lib/dpkg/lock-frontend"); err == nil {
		if err := execCommandContext(ctx, "pgrep", "-x", "apt-get").Run(); err == nil {
			return fmt.Errorf("package manager is currently locked or in use")
		}
		if err := execCommandContext(ctx, "pgrep", "-x", "dpkg").Run(); err == nil {
			return fmt.Errorf("package manager is currently locked or in use")
		}
	}
	return nil
}

func (m *AptManager) Cleanup(ctx context.Context) error {
	cmd := execCommandContext(ctx, "apt-get", "autoremove", "-y")
	cmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
	return cmd.Run()
}
