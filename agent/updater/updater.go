package updater

import (
	"context"
	"fmt"
	"os"
)

// UpdateResult represents the outcome of an update operation.
type UpdateResult struct {
	Success bool
	Output  string
	Error   error
}

// PackageManager defines the interface for OS-specific package managers.
type PackageManager interface {
	// CheckUpdates checks for available updates.
	CheckUpdates(ctx context.Context) (UpdateResult, error)
	// ApplyUpdates applies all available updates or specific packages.
	ApplyUpdates(ctx context.Context, packages []string) (UpdateResult, error)
	// RebootRequired checks if a reboot is needed after updates.
	RebootRequired() bool
}

// DetectPackageManager determines the underlying OS package manager.
func DetectPackageManager() (PackageManager, error) {
	// Check for apk (Alpine)
	if _, err := os.Stat("/sbin/apk"); err == nil {
		return &ApkManager{}, nil
	}
	
	// Check for apt (Debian/Ubuntu)
	if _, err := os.Stat("/usr/bin/apt-get"); err == nil {
		return &AptManager{}, nil
	}

	return nil, fmt.Errorf("unsupported or unknown package manager")
}
