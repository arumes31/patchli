package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestState(t *testing.T) {
	// Create a temporary state file for testing
	tmpDir, err := os.MkdirTemp("", "patchli-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

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
	tmpDir, err := os.MkdirTemp("", "patchli-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

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
	tmpDir, err := os.MkdirTemp("", "patchli-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

	oldStateFile := stateFile
	stateFile = filepath.Join(tmpDir, "invalid.json")
	defer func() { stateFile = oldStateFile }()

	_ = os.WriteFile(stateFile, []byte("{invalid"), 0644)
	_, err = LoadState()
	if err == nil {
		t.Error("LoadState should have failed for invalid JSON")
	}
}

func TestSaveStateError(t *testing.T) {
	// Place the state path below a regular file so MkdirAll fails on every OS.
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0600); err != nil {
		t.Fatal(err)
	}
	oldStateFile := stateFile
	stateFile = filepath.Join(blocker, "state.json")
	defer func() { stateFile = oldStateFile }()

	state := State{JobID: "test"}
	err := SaveState(state)
	if err == nil {
		t.Error("SaveState should have failed for invalid path")
	}
}

func TestLoadStateError(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "patchli-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

	oldStateFile := stateFile
	stateFile = filepath.Join(tmpDir, "state.json")
	defer func() { stateFile = oldStateFile }()

	// Write invalid JSON
	if err := os.WriteFile(stateFile, []byte("invalid json"), 0644); err != nil {
		t.Fatalf("Failed to write invalid json: %v", err)
	}

	_, err = LoadState()
	if err == nil {
		t.Error("LoadState should have failed for invalid JSON")
	}
}
