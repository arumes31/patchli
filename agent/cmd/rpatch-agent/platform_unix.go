//go:build !windows

package main

import (
	"fmt"
	"os"
)

func restrictTokenPath(path string, directory bool) error {
	mode := os.FileMode(0600)
	if directory {
		mode = 0700
	}
	return os.Chmod(path, mode)
}

func restartWindowsAgent() ([]byte, error) {
	return nil, fmt.Errorf("windows service restart is unavailable on this platform")
}
