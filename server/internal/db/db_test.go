package db

import (
	"testing"
)

func TestInit_Error(t *testing.T) {
	// Should fail with invalid URL
	err := Init("invalid-url")
	if err == nil {
		t.Error("expected error for invalid URL")
	}
}

func TestUpdateNodeStatus_NoDB(t *testing.T) {
	// Should not panic if DB is nil
	DB = nil
	err := UpdateNodeStatus("mac", "host", "os", "v", "k", "status")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestIsJobRunning_NoDB(t *testing.T) {
	DB = nil
	running, err := IsJobRunning("job1")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if running {
		t.Error("expected running to be false")
	}
}
