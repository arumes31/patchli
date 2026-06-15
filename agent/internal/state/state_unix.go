//go:build !windows

package state

func init() {
	stateFile = "/var/lib/patchli/state.json"
}
