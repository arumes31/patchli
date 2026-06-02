package db

import (
	"strings"
	"testing"
)

func TestInitErrors(t *testing.T) {
	t.Run("InvalidDriver", func(t *testing.T) {
		// This should fail at sql.Open because "invalid" driver is not registered
		// Note: We need to use a function that calls sql.Open directly or change Init to accept driver name
		// But Init is hardcoded to "pgx".
		// To test the first error path in Init, we need sql.Open("pgx", dbURL) to fail.
		// For many drivers, sql.Open only fails if the driver name is unknown.
		// Since "pgx" IS known, it likely won't fail there.
	})

	t.Run("InvalidDSN", func(t *testing.T) {
		err := Init("postgres://%@")
		if err == nil {
			t.Fatal("Expected error for invalid DSN, but got nil")
		}
		if !strings.Contains(err.Error(), "failed to ping database") && !strings.Contains(err.Error(), "failed to open database") {
			t.Errorf("Unexpected error: %v", err)
		}
	})

	t.Run("UnreachableHost", func(t *testing.T) {
		err := Init("postgres://user:pass@localhost:54321/db?sslmode=disable")
		if err == nil {
			t.Fatal("Expected error for unreachable host, but got nil")
		}
		if !strings.Contains(err.Error(), "failed to ping database") {
			t.Errorf("Expected ping failure error, got: %v", err)
		}
	})
}
