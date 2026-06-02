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

	// Store original DB and restore after test
	origDB := DB
	DB = db
	defer func() { DB = origDB }()

	mac := "00:11:22:33:44:55"
	hostname := "test-host"
	osName := "Linux"
	osVersion := "22.04"
	kernel := "5.15.0"
	status := "online"

	mock.ExpectExec("INSERT INTO nodes").
		WithArgs(mac, hostname, osName, osVersion, kernel, status).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err = UpdateNodeStatus(mac, hostname, osName, osVersion, kernel, status)
	if err != nil {
		t.Errorf("error was not expected while updating node status: %s", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestUpdateNodeStatus_NoDB(t *testing.T) {
	// Store original DB and restore after test
	origDB := DB
	DB = nil
	defer func() { DB = origDB }()

	err := UpdateNodeStatus("mac", "host", "os", "ver", "kern", "status")
	if err != nil {
		t.Errorf("error was not expected when DB is nil: %s", err)
	}
}

func TestIsJobRunning(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()

	// Store original DB and restore after test
	origDB := DB
	DB = db
	defer func() { DB = origDB }()

	jobID := "test-job-id"

	t.Run("Job is running", func(t *testing.T) {
		rows := sqlmock.NewRows([]string{"status"}).AddRow("running")
		mock.ExpectQuery("SELECT status FROM audit_logs").
			WithArgs(jobID).
			WillReturnRows(rows)

		running, err := IsJobRunning(jobID)
		if err != nil {
			t.Errorf("error was not expected: %s", err)
		}
		if !running {
			t.Error("expected job to be running")
		}
	})

	t.Run("Job is not running", func(t *testing.T) {
		rows := sqlmock.NewRows([]string{"status"}).AddRow("completed")
		mock.ExpectQuery("SELECT status FROM audit_logs").
			WithArgs(jobID).
			WillReturnRows(rows)

		running, err := IsJobRunning(jobID)
		if err != nil {
			t.Errorf("error was not expected: %s", err)
		}
		if running {
			t.Error("expected job to not be running")
		}
	})

	t.Run("Job not found", func(t *testing.T) {
		mock.ExpectQuery("SELECT status FROM audit_logs").
			WithArgs(jobID).
			WillReturnError(sql.ErrNoRows)

		running, err := IsJobRunning(jobID)
		if err != nil {
			t.Errorf("error was not expected: %s", err)
		}
		if running {
			t.Error("expected job to not be running")
		}
	})

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestIsJobRunning_NoDB(t *testing.T) {
	// Store original DB and restore after test
	origDB := DB
	DB = nil
	defer func() { DB = origDB }()

	running, err := IsJobRunning("job-id")
	if err != nil {
		t.Errorf("error was not expected when DB is nil: %s", err)
	}
	if running {
		t.Error("expected running to be false when DB is nil")
	}
}
