//go:build !windows

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTokenPermissionsAreTightened(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "credentials")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "tokens.json")
	if err := os.WriteFile(path, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := writeTokens(path, []byte("new")); err != nil {
		t.Fatal(err)
	}
	for p, want := range map[string]os.FileMode{path: 0600, dir: 0700} {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != want {
			t.Errorf("%s: got permissions %o, want %o", p, info.Mode().Perm(), want)
		}
	}
}
