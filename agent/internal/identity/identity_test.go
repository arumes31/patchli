package identity

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestGetOrGenerate(t *testing.T) {
	// Setup temporary idFile
	tmpDir := t.TempDir()
	origIdFile := idFile
	idFile = filepath.Join(tmpDir, "node_id")
	defer func() { idFile = origIdFile }()

	t.Run("GenerateNewID", func(t *testing.T) {
		// Ensure file doesn't exist
		os.Remove(idFile)

		id := GetOrGenerate()
		if _, err := uuid.Parse(id); err != nil {
			t.Errorf("GetOrGenerate() returned invalid UUID: %v", err)
		}

		// Verify file was created
		data, err := os.ReadFile(idFile)
		if err != nil {
			t.Fatalf("Failed to read idFile: %v", err)
		}
		if strings.TrimSpace(string(data)) != id {
			t.Errorf("File content %q does not match returned ID %q", string(data), id)
		}
	})

	t.Run("LoadExistingID", func(t *testing.T) {
		existingID := uuid.New().String()
		err := os.WriteFile(idFile, []byte(existingID), 0600)
		if err != nil {
			t.Fatalf("Failed to write existing ID: %v", err)
		}

		id := GetOrGenerate()
		if id != existingID {
			t.Errorf("GetOrGenerate() = %q, want %q", id, existingID)
		}
	})

	t.Run("RegenerateInvalidID", func(t *testing.T) {
		err := os.WriteFile(idFile, []byte("invalid-uuid"), 0600)
		if err != nil {
			t.Fatalf("Failed to write invalid ID: %v", err)
		}

		id := GetOrGenerate()
		if _, err := uuid.Parse(id); err != nil {
			t.Errorf("GetOrGenerate() returned invalid UUID: %v", err)
		}
		if id == "invalid-uuid" {
			t.Error("GetOrGenerate() should not return invalid-uuid")
		}

		// Verify file was updated
		data, err := os.ReadFile(idFile)
		if err != nil {
			t.Fatalf("Failed to read idFile: %v", err)
		}
		if strings.TrimSpace(string(data)) != id {
			t.Errorf("File content %q does not match returned ID %q", string(data), id)
		}
	})
}
