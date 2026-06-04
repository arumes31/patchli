package db

import (
	"database/sql"
	"errors"
	"testing"
)

func TestInit(t *testing.T) {
	// Backup original functions
	origSqlOpen := sqlOpen
	origDbPing := dbPing
	origRunMigrationsFunc := runMigrationsFunc
	defer func() {
		sqlOpen = origSqlOpen
		dbPing = origDbPing
		runMigrationsFunc = origRunMigrationsFunc
	}()

	t.Run("OpenError", func(t *testing.T) {
		sqlOpen = func(driverName, dataSourceName string) (*sql.DB, error) {
			return nil, errors.New("open error")
		}
		err := Init("invalid-url")
		if err == nil || err.Error() != "failed to open database: open error" {
			t.Errorf("expected open error, got %v", err)
		}
	})

	t.Run("PingError", func(t *testing.T) {
		sqlOpen = func(driverName, dataSourceName string) (*sql.DB, error) {
			return &sql.DB{}, nil
		}
		dbPing = func(db *sql.DB) error {
			return errors.New("ping error")
		}
		err := Init("url")
		if err == nil || err.Error() != "failed to ping database: ping error" {
			t.Errorf("expected ping error, got %v", err)
		}
	})

	t.Run("MigrationError", func(t *testing.T) {
		sqlOpen = func(driverName, dataSourceName string) (*sql.DB, error) {
			return &sql.DB{}, nil
		}
		dbPing = func(db *sql.DB) error {
			return nil
		}
		runMigrationsFunc = func(dbURL string) error {
			return errors.New("migration error")
		}
		err := Init("valid-url")
		if err == nil || err.Error() != "migration failed: migration error" {
			t.Errorf("expected migration error, got %v", err)
		}
	})

	t.Run("Success", func(t *testing.T) {
		sqlOpen = func(driverName, dataSourceName string) (*sql.DB, error) {
			return &sql.DB{}, nil
		}
		dbPing = func(db *sql.DB) error {
			return nil
		}
		runMigrationsFunc = func(dbURL string) error {
			return nil
		}
		err := Init("valid-url")
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})
}

func TestRunMigrations_OpenError(t *testing.T) {
	origSqlOpen := sqlOpen
	defer func() { sqlOpen = origSqlOpen }()

	sqlOpen = func(driverName, dataSourceName string) (*sql.DB, error) {
		return nil, errors.New("open error")
	}

	err := runMigrations("url")
	if err == nil || err.Error() != "open error" {
		t.Errorf("expected open error, got %v", err)
	}
}

func TestUpdateNodeStatus_NilDB(t *testing.T) {
	oldDB := DB
	defer func() { DB = oldDB }()
	DB = nil
	err := UpdateNodeStatus("mac", "host", "os", "ver", "ker", "stat")
	if err != nil {
		t.Errorf("expected no error when DB is nil, got %v", err)
	}
}

func TestIsJobRunning_NilDB(t *testing.T) {
	oldDB := DB
	defer func() { DB = oldDB }()
	DB = nil
	running, err := IsJobRunning("job-id")
	if err != nil {
		t.Errorf("expected no error when DB is nil, got %v", err)
	}
	if running {
		t.Error("expected running to be false when DB is nil")
	}
}
