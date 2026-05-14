package identity

import (
	"log"
	"os"
	"strings"

	"github.com/google/uuid"
)

var idFile = "/etc/patchli/node_id"

func GetOrGenerate() string {
	if data, err := os.ReadFile(idFile); err == nil && len(data) > 0 {
		return strings.TrimSpace(string(data))
	}

	newID := uuid.New().String()
	if err := os.MkdirAll("/etc/patchli", 0750); err != nil {
		log.Printf("Warning: failed to create /etc/patchli directory: %v", err)
	} else if err := os.WriteFile(idFile, []byte(newID), 0600); err != nil {
		log.Printf("Warning: failed to write node ID to %s: %v", idFile, err)
	}
	return newID
}
