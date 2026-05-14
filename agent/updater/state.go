package updater

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// State represents the persistent state of a patch job.
type State struct {
	JobID     string   `json:"job_id"`
	Action    string   `json:"action"`
	Packages  []string `json:"packages"`
	Status    string   `json:"status"` // "running", "completed", "failed"
	StartTime int64    `json:"start_time"`
}

const stateFile = "/var/lib/patchli/state.json"

// SaveState persists the current job state to disk.
func SaveState(state State) error {
	dir := filepath.Dir(stateFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.Marshal(state)
	if err != nil {
		return err
	}

	return os.WriteFile(stateFile, data, 0644)
}

// LoadState reads the last known job state from disk.
func LoadState() (*State, error) {
	if _, err := os.Stat(stateFile); os.IsNotExist(err) {
		return nil, nil
	}

	data, err := os.ReadFile(stateFile)
	if err != nil {
		return nil, err
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}

	return &state, nil
}

// ClearState removes the state file after a job is successfully completed and reported.
func ClearState() error {
	return os.Remove(stateFile)
}
