package state

import (
	"os"
	"path/filepath"
	"testing"
	"runtime"
)

func TestState(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "patchli-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	oldStateFile := stateFile
	stateFile = filepath.Join(tmpDir, "state.json")
	defer func() { stateFile = oldStateFile }()

	state := State{
		JobID:  "test-job",
		Action: "apply_updates",
		Status: "running",
	}
	if err := SaveState(state); err != nil {
		t.Errorf("SaveState failed: %v", err)
	}

	loaded, err := LoadState()
	if err != nil {
		t.Errorf("LoadState failed: %v", err)
	}
	if loaded == nil {
		t.Fatal("LoadState returned nil")
	}
	if loaded.JobID != state.JobID {
		t.Errorf("Expected JobID %s, got %s", state.JobID, loaded.JobID)
	}

	if err := ClearState(); err != nil {
		t.Errorf("ClearState failed: %v", err)
	}
	if _, err := os.Stat(stateFile); !os.IsNotExist(err) {
		t.Error("State file still exists after ClearState")
	}

	loaded, err = LoadState()
	if err != nil {
		t.Errorf("LoadState failed after clear: %v", err)
	}
	if loaded != nil {
		t.Error("LoadState should return nil after clear")
	}
}

func TestLoadStateInvalidJSON(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "patchli-test-*")
	defer os.RemoveAll(tmpDir)

	oldStateFile := stateFile
	stateFile = filepath.Join(tmpDir, "invalid.json")
	defer func() { stateFile = oldStateFile }()

	_ = os.WriteFile(stateFile, []byte("{invalid"), 0644)
	_, err := LoadState()
	if err == nil {
		t.Error("LoadState should have failed for invalid JSON")
	}
}

func TestSaveStateError(t *testing.T) {
	oldStateFile := stateFile
	if runtime.GOOS == "windows" {
		stateFile = "Z:\\invalid\\path\\?:|/state.json"
	} else {
		// On Linux, use a path that is a directory
		tmpDir, _ := os.MkdirTemp("", "patchli-dir-*")
		defer os.RemoveAll(tmpDir)
		stateFile = tmpDir
	}
	defer func() { stateFile = oldStateFile }()

	state := State{JobID: "test"}
	err := SaveState(state)
	if err == nil {
		t.Error("SaveState should have failed for invalid path")
	}
}
