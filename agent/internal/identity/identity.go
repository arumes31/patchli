package identity

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/google/uuid"
)

var idFile string

func init() {
	if runtime.GOOS == "windows" {
		pd := os.Getenv("PROGRAMDATA")
		if pd == "" {
			pd = `C:\ProgramData`
		}
		idFile = filepath.Join(pd, "Patchli", "node_id")
	} else {
		idFile = "/etc/patchli/node_id"
	}
}

func GetOrGenerate() (string, error) {
	// #nosec G304 -- idFile is fixed by the platform implementation; tests replace it with a temporary path.
	if data, err := os.ReadFile(idFile); err == nil && len(data) > 0 {
		parsed := strings.TrimSpace(string(data))
		if _, err := uuid.Parse(parsed); err == nil {
			return parsed, nil
		}
	}

	newID := uuid.New().String()
	dir := filepath.Dir(idFile)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return newID, fmt.Errorf("failed to create %s directory: %v", dir, err)
	}
	if err := os.WriteFile(idFile, []byte(newID), 0600); err != nil {
		return newID, fmt.Errorf("failed to write node ID to %s: %v", idFile, err)
	}
	return newID, nil
}
