package identity

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
)

func TestGetOrGenerate(t *testing.T) {
	// Store the original idFile and restore it after tests
	originalIdFile := idFile
	defer func() { idFile = originalIdFile }()

	t.Run("generate new ID when file doesn't exist", func(t *testing.T) {
		tmpDir := t.TempDir()

		idFile = filepath.Join(tmpDir, "node_id")

		id, err := GetOrGenerate()
		if err != nil {
			t.Errorf("GetOrGenerate() returned error: %v", err)
		}
		if _, parseErr := uuid.Parse(id); parseErr != nil {
			t.Errorf("GetOrGenerate() returned invalid UUID: %s", id)
		}

		// Verify file was created and contains the same ID
		data, err := os.ReadFile(idFile)
		if err != nil {
			t.Fatalf("failed to read id file: %v", err)
		}
		if string(data) != id {
			t.Errorf("file content mismatch: expected %s, got %s", id, string(data))
		}
	})

	t.Run("load existing valid ID", func(t *testing.T) {
		tmpDir := t.TempDir()

		idFile = filepath.Join(tmpDir, "node_id")
		expectedID := uuid.New().String()
		if err := os.WriteFile(idFile, []byte(expectedID), 0600); err != nil {
			t.Fatalf("failed to write mock id: %v", err)
		}

		id, err := GetOrGenerate()
		if err != nil {
			t.Errorf("GetOrGenerate() returned error: %v", err)
		}
		if id != expectedID {
			t.Errorf("GetOrGenerate() mismatch: expected %s, got %s", expectedID, id)
		}
	})

	t.Run("generate new ID when file content is invalid", func(t *testing.T) {
		tmpDir := t.TempDir()

		idFile = filepath.Join(tmpDir, "node_id")
		if err := os.WriteFile(idFile, []byte("invalid-uuid"), 0600); err != nil {
			t.Fatalf("failed to write invalid id: %v", err)
		}

		id, err := GetOrGenerate()
		if err != nil {
			t.Errorf("GetOrGenerate() returned error: %v", err)
		}
		if _, parseErr := uuid.Parse(id); parseErr != nil {
			t.Errorf("GetOrGenerate() returned invalid UUID: %s", id)
		}
		if id == "invalid-uuid" {
			t.Error("GetOrGenerate() should have generated a new ID, but returned the invalid one")
		}

		// Verify file was updated
		data, err := os.ReadFile(idFile)
		if err != nil {
			t.Fatalf("failed to read id file: %v", err)
		}
		if string(data) != id {
			t.Errorf("file content mismatch: expected %s, got %s", id, string(data))
		}
	})
}

func TestGetOrGenerateIdentityPersists(t *testing.T) {
	previous := idFile
	idFile = filepath.Join(t.TempDir(), "node_id")
	t.Cleanup(func() { idFile = previous })
	first, err := GetOrGenerate()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := uuid.Parse(first); err != nil {
		t.Fatalf("invalid generated identity: %v", err)
	}
	persisted, err := os.ReadFile(idFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(persisted) != first {
		t.Fatal("generated identity was not persisted")
	}
	second, err := GetOrGenerate()
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("identity changed across reload: %q -> %q", first, second)
	}
}
