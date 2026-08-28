package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfigRequiresTLSAndStrongAdminToken(t *testing.T) {
	directory := t.TempDir()
	certFile := filepath.Join(directory, "tls.crt")
	keyFile := filepath.Join(directory, "tls.key")
	for _, path := range []string{certFile, keyFile} {
		if err := os.WriteFile(path, []byte("test"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	values := map[string]string{
		"TLS_CERT_FILE": certFile,
		"TLS_KEY_FILE":  keyFile,
		"BASE_URL":      "https://patchli.example.test",
		"ADMIN_TOKEN":   strings.Repeat("a", 32),
		"DB_URL":        "postgres://patchli@postgres/patchli",
	}
	getenv := func(key string) string { return values[key] }
	if _, err := loadConfig(getenv); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}

	values["BASE_URL"] = "http://patchli.example.test"
	if _, err := loadConfig(getenv); err == nil {
		t.Fatal("plaintext BASE_URL accepted")
	}
	values["BASE_URL"] = "https://patchli.example.test"
	values["ADMIN_TOKEN"] = "short"
	if _, err := loadConfig(getenv); err == nil {
		t.Fatal("short ADMIN_TOKEN accepted")
	}
}
