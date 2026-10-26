package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestState(t *testing.T) {
	// Create a temporary state file for testing
	tmpDir := t.TempDir()

	oldStateFile := stateFile
	stateFile = filepath.Join(tmpDir, "state.json")
	defer func() { stateFile = oldStateFile }()

	// Test SaveState
	state := State{
		JobID:  "test-job",
		Action: "apply_updates",
		Status: "running",
	}
	if err := SaveState(state); err != nil {
		t.Errorf("SaveState failed: %v", err)
	}

	// Test LoadState
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

	// Test ClearState
	if err := ClearState(); err != nil {
		t.Errorf("ClearState failed: %v", err)
	}
	if _, err := os.Stat(stateFile); !os.IsNotExist(err) {
		t.Error("State file still exists after ClearState")
	}

	// Test LoadState after ClearState
	loaded, err = LoadState()
	if err != nil {
		t.Errorf("LoadState failed after clear: %v", err)
	}
	if loaded != nil {
		t.Error("LoadState should return nil after clear")
	}
}

func TestClearStateIdempotent(t *testing.T) {
	tmpDir := t.TempDir()

	oldStateFile := stateFile
	stateFile = filepath.Join(tmpDir, "state.json")
	defer func() { stateFile = oldStateFile }()

	// ClearState on a non-existent file should not return an error
	if err := ClearState(); err != nil {
		t.Errorf("ClearState should not fail when file doesn't exist: %v", err)
	}

	// Calling ClearState again should still succeed
	if err := ClearState(); err != nil {
		t.Errorf("ClearState should be idempotent: %v", err)
	}
}

func TestLoadStateInvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()

	oldStateFile := stateFile
	stateFile = filepath.Join(tmpDir, "invalid.json")
	defer func() { stateFile = oldStateFile }()

	if err := os.WriteFile(stateFile, []byte("{invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadState()
	if err == nil {
		t.Error("LoadState should have failed for invalid JSON")
	}
}

func TestSaveStateError(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "file-not-directory")
	if err := os.WriteFile(parent, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	previous := stateFile
	stateFile = filepath.Join(parent, "state.json")
	t.Cleanup(func() { stateFile = previous })
	if err := SaveState(State{JobID: "test"}); err == nil {
		t.Fatal("SaveState accepted a regular file as its parent directory")
	}
	data, err := os.ReadFile(parent)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "fixture" {
		t.Fatal("failed save modified the parent fixture")
	}
}

func TestLoadStateError(t *testing.T) {
	tmpDir := t.TempDir()

	oldStateFile := stateFile
	stateFile = filepath.Join(tmpDir, "state.json")
	defer func() { stateFile = oldStateFile }()

	// Write invalid JSON
	if err := os.WriteFile(stateFile, []byte("invalid json"), 0644); err != nil {
		t.Fatalf("Failed to write invalid json: %v", err)
	}

	_, err := LoadState()
	if err == nil {
		t.Error("LoadState should have failed for invalid JSON")
	}
}
