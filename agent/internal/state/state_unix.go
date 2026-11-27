//go:build !windows

package state

func init() {
	stateFile = "/var/lib/rpatch/state.json"
}
