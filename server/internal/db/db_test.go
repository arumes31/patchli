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
	defer func() { _ = db.Close() }()

	// Backup original DB and restore after test
	oldDB := DB
	DB = db
	defer func() { DB = oldDB }()

	mac := "00:11:22:33:44:55"
	hostname := "test-node"
	osName := "Ubuntu"
	osVersion := "22.04"
	kernel := "5.15.0"
	status := "online"

	t.Run("Success", func(t *testing.T) {
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

	t.Run("DB Error", func(t *testing.T) {
		mock.ExpectExec("INSERT INTO nodes").
			WithArgs(mac, hostname, osName, osVersion, kernel, status).
			WillReturnError(sql.ErrConnDone)

		err := UpdateNodeStatus(mac, hostname, osName, osVersion, kernel, status)
		if err == nil {
			t.Error("error was expected but not returned")
		}
	})

	t.Run("Nil DB", func(t *testing.T) {
		DB = nil
		err := UpdateNodeStatus(mac, hostname, osName, osVersion, kernel, status)
		if err == nil {
			t.Error("expected error when DB is nil")
		}
		DB = db // Restore for next tests
	})
}

func TestIsJobRunning(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer func() { _ = db.Close() }()

	oldDB := DB
	DB = db
	defer func() { DB = oldDB }()

	jobID := "job-123"

	t.Run("Running", func(t *testing.T) {
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

	t.Run("Not Running", func(t *testing.T) {
		rows := sqlmock.NewRows([]string{"status"}).AddRow("completed")
		mock.ExpectQuery("SELECT status FROM audit_logs").
			WithArgs(jobID).
			WillReturnRows(rows)

		running, err := IsJobRunning(jobID)
		if err != nil {
			t.Errorf("error was not expected: %s", err)
		}
		if running {
			t.Error("expected job NOT to be running")
		}
	})

	t.Run("No Rows", func(t *testing.T) {
		mock.ExpectQuery("SELECT status FROM audit_logs").
			WithArgs(jobID).
			WillReturnError(sql.ErrNoRows)

		running, err := IsJobRunning(jobID)
		if err != nil {
			t.Errorf("error was not expected: %s", err)
		}
		if running {
			t.Error("expected job NOT to be running")
		}
	})

	t.Run("DB Error", func(t *testing.T) {
		mock.ExpectQuery("SELECT status FROM audit_logs").
			WithArgs(jobID).
			WillReturnError(sql.ErrConnDone)

		_, err := IsJobRunning(jobID)
		if err == nil {
			t.Error("error was expected but not returned")
		}
	})

	t.Run("Nil DB", func(t *testing.T) {
		DB = nil
		running, err := IsJobRunning(jobID)
		if err == nil {
			t.Error("expected error when DB is nil")
		}
		if running {
			t.Error("expected false for nil DB")
		}
		DB = db
	})
}
