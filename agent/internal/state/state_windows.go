//go:build windows

package state

import (
	"os"
	"path/filepath"
)

func init() {
	pd := os.Getenv("PROGRAMDATA")
	if pd == "" {
		pd = `C:\ProgramData`
	}
	stateFile = filepath.Join(pd, "Patchli", "state.json")
}
