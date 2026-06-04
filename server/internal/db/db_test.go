package db

import (
	"database/sql"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestUpdateNodeStatus(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()

	// Save original DB and restore after test
	origDB := DB
	defer func() { DB = origDB }()
	DB = db

	mac := "00:11:22:33:44:55"
	hostname := "test-host"
	osName := "Linux"
	osVersion := "22.04"
	kernel := "5.15"
	status := "online"

	t.Run("successful update", func(t *testing.T) {
		mock.ExpectExec("INSERT INTO nodes").
			WithArgs(mac, hostname, osName, osVersion, kernel, status).
			WillReturnResult(sqlmock.NewResult(1, 1))

		err := UpdateNodeStatus(mac, hostname, osName, osVersion, kernel, status)
		if err != nil {
			t.Errorf("error was not expected: %s", err)
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("there were unfulfilled expectations: %s", err)
		}
	})

	t.Run("DB is nil", func(t *testing.T) {
		DB = nil
		err := UpdateNodeStatus(mac, hostname, osName, osVersion, kernel, status)
		if err != nil {
			t.Errorf("error was not expected when DB is nil: %s", err)
		}
	})
}

func TestIsJobRunning(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()

	origDB := DB
	defer func() { DB = origDB }()
	DB = db

	jobID := "job-123"

	t.Run("job is running", func(t *testing.T) {
		rows := sqlmock.NewRows([]string{"status"}).AddRow("running")
		mock.ExpectQuery("SELECT status FROM audit_logs").
			WithArgs(jobID).
			WillReturnRows(rows)

		running, err := IsJobRunning(jobID)
		if err != nil {
			t.Errorf("error was not expected: %s", err)
		}
		if !running {
			t.Errorf("expected job to be running")
		}
	})

	t.Run("job is not running", func(t *testing.T) {
		rows := sqlmock.NewRows([]string{"status"}).AddRow("completed")
		mock.ExpectQuery("SELECT status FROM audit_logs").
			WithArgs(jobID).
			WillReturnRows(rows)

		running, err := IsJobRunning(jobID)
		if err != nil {
			t.Errorf("error was not expected: %s", err)
		}
		if running {
			t.Errorf("expected job to not be running")
		}
	})

	t.Run("no rows found", func(t *testing.T) {
		mock.ExpectQuery("SELECT status FROM audit_logs").
			WithArgs(jobID).
			WillReturnError(sql.ErrNoRows)

		running, err := IsJobRunning(jobID)
		if err != nil {
			t.Errorf("error was not expected: %s", err)
		}
		if running {
			t.Errorf("expected job to not be running")
		}
	})

	t.Run("DB is nil", func(t *testing.T) {
		DB = nil
		running, err := IsJobRunning(jobID)
		if err != nil {
			t.Errorf("error was not expected when DB is nil: %s", err)
		}
		if running {
			t.Errorf("expected job to not be running when DB is nil")
		}
	})
}

func TestInitError(t *testing.T) {
	err := Init("invalid-connection-string")
	if err == nil {
		t.Error("expected error when initializing with invalid connection string")
	}
}
