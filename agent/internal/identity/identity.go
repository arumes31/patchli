package identity

import (
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/google/uuid"
)

var idFile string

func init() {
	if runtime.GOOS == "windows" {
		idFile = filepath.Join(os.Getenv("PROGRAMDATA"), "Patchli", "node_id")
	} else {
		idFile = "/etc/patchli/node_id"
	}
}

func GetOrGenerate() string {
	/* #nosec G304 */
	if data, err := os.ReadFile(idFile); err == nil && len(data) > 0 {
		parsed := strings.TrimSpace(string(data))
		if _, err := uuid.Parse(parsed); err == nil {
			return parsed
		}
	}

	newID := uuid.New().String()
	dir := filepath.Dir(idFile)
	if err := os.MkdirAll(dir, 0750); err != nil {
		log.Printf("Warning: failed to create %s directory: %v", dir, err)
	} else if err := os.WriteFile(idFile, []byte(newID), 0600); err != nil {
		log.Printf("Warning: failed to write node ID to %s: %v", idFile, err)
	}
	return newID
}
