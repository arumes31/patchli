package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// writeTokens replaces old files atomically so existing broad permissions do
// not survive a refresh. The temporary file is restricted before writing secrets.
func writeTokens(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if err := restrictTokenPath(dir, true); err != nil {
		return fmt.Errorf("restrict token directory: %w", err)
	}
	file, err := os.CreateTemp(dir, ".tokens-*")
	if err != nil {
		return err
	}
	name := file.Name()
	defer func() { _ = file.Close(); _ = os.Remove(name) }()
	if err := restrictTokenPath(name, false); err != nil {
		return fmt.Errorf("restrict token file: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}
