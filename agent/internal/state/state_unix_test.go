//go:build !windows

package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStateFilePathUnix(t *testing.T) {
	expected := "/var/lib/patchli/state.json"
	if stateFile != expected {
		t.Errorf("Expected stateFile to be %s, got %s", expected, stateFile)
	}
}

func TestSaveStatePermissionsUnix(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "patchli-perm-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	oldStateFile := stateFile
	stateFile = filepath.Join(tmpDir, "subdir", "state.json")
	defer func() { stateFile = oldStateFile }()

	state := State{
		JobID: "perm-test",
	}

	if err := SaveState(state); err != nil {
		t.Fatalf("SaveState failed: %v", err)
	}

	// Check directory permissions
	dirInfo, err := os.Stat(filepath.Dir(stateFile))
	if err != nil {
		t.Fatalf("Failed to stat directory: %v", err)
	}
	if mode := dirInfo.Mode().Perm(); mode != 0750 {
		t.Errorf("Expected directory permissions 0750, got %o", mode)
	}

	// Check file permissions
	fileInfo, err := os.Stat(stateFile)
	if err != nil {
		t.Fatalf("Failed to stat file: %v", err)
	}
	if mode := fileInfo.Mode().Perm(); mode != 0600 {
		t.Errorf("Expected file permissions 0600, got %o", mode)
	}
}
