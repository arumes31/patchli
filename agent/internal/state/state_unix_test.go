//go:build !windows

package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultStateFileUnix(t *testing.T) {
	expected := "/var/lib/patchli/state.json"
	if stateFile != expected {
		t.Errorf("expected stateFile to be %s, got %s", expected, stateFile)
	}
}

func TestStatePermissionsUnix(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "patchli-perm-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	oldStateFile := stateFile
	// Use a nested subdirectory to ensure MkdirAll creates it
	statePath := filepath.Join(tmpDir, "nested/dir", "state.json")
	stateFile = statePath
	defer func() { stateFile = oldStateFile }()

	state := State{
		JobID: "perm-test",
	}
	if err := SaveState(state); err != nil {
		t.Fatalf("SaveState failed: %v", err)
	}

	// Check directory permissions (0750)
	dirInfo, err := os.Stat(filepath.Dir(statePath))
	if err != nil {
		t.Fatalf("Failed to stat directory: %v", err)
	}
	// On some systems/filesystems, the actual permissions might be affected by umask
	// but we expect at most 0750 as requested in MkdirAll
	if dirInfo.Mode().Perm() != 0750 {
		t.Errorf("Expected directory permissions 0750, got %o", dirInfo.Mode().Perm())
	}

	// Check file permissions (0600)
	fileInfo, err := os.Stat(statePath)
	if err != nil {
		t.Fatalf("Failed to stat file: %v", err)
	}
	if fileInfo.Mode().Perm() != 0600 {
		t.Errorf("Expected file permissions 0600, got %o", fileInfo.Mode().Perm())
	}
}
