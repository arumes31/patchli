//go:build !windows

package state

var stateFile = ""

func init() {
	if envDir := os.Getenv("PATCHLI_STATE_DIR"); envDir != "" {
		stateFile = filepath.Join(envDir, "state.json")
	} else {
		stateFile = "/var/lib/patchli/state.json"
	}
}
