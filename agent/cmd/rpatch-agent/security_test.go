package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestRefreshRejectsUntrustedTLS(t *testing.T) {
	received := false
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { received = true }))
	defer server.Close()
	if _, err := refreshTokens(context.Background(), server.URL, "node", "secret"); err == nil {
		t.Fatal("untrusted controller TLS certificate accepted")
	}
	if received {
		t.Fatal("credentials reached an untrusted controller")
	}
}

func TestWriteTokensReplacesExistingFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "credentials")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "tokens.json")
	if err := os.WriteFile(path, []byte("old-secret"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := writeTokens(path, []byte("new-secret")); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new-secret" {
		t.Fatalf("got %q", data)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "tokens.json" {
		t.Fatalf("temporary credential files remain: %v", entries)
	}
}
