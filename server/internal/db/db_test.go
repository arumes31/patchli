package db

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestUpdateNodeStatus(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()

	// Store original DB and restore it after test
	origDB := DB
	defer func() { DB = origDB }()

	DB = db

	mac := "00:11:22:33:44:55"
	hostname := "test-host"
	osName := "Linux"
	osVersion := "Ubuntu 22.04"
	kernel := "5.15.0"
	status := "Online"

	t.Run("Success", func(t *testing.T) {
		mock.ExpectExec("INSERT INTO nodes").
			WithArgs(mac, hostname, osName, osVersion, kernel, status).
			WillReturnResult(sqlmock.NewResult(1, 1))

		err := UpdateNodeStatus(mac, hostname, osName, osVersion, kernel, status)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("there were unfulfilled expectations: %s", err)
		}
	})

	t.Run("Database Error", func(t *testing.T) {
		mock.ExpectExec("INSERT INTO nodes").
			WithArgs(mac, hostname, osName, osVersion, kernel, status).
			WillReturnError(errors.New("db error"))

		err := UpdateNodeStatus(mac, hostname, osName, osVersion, kernel, status)
		if err == nil {
			t.Error("expected error, got nil")
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("there were unfulfilled expectations: %s", err)
		}
	})

	t.Run("DB is nil", func(t *testing.T) {
		DB = nil
		err := UpdateNodeStatus(mac, hostname, osName, osVersion, kernel, status)
		if err != nil {
			t.Errorf("expected no error when DB is nil, got %v", err)
		}
		DB = db // restore for next tests if any
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

	jobID := "test-job-id"

	t.Run("Running", func(t *testing.T) {
		rows := sqlmock.NewRows([]string{"status"}).AddRow("running")
		mock.ExpectQuery("SELECT status FROM audit_logs").WithArgs(jobID).WillReturnRows(rows)

		running, err := IsJobRunning(jobID)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if !running {
			t.Error("expected job to be running")
		}
	})

	t.Run("Not Running", func(t *testing.T) {
		rows := sqlmock.NewRows([]string{"status"}).AddRow("completed")
		mock.ExpectQuery("SELECT status FROM audit_logs").WithArgs(jobID).WillReturnRows(rows)

		running, err := IsJobRunning(jobID)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if running {
			t.Error("expected job to not be running")
		}
	})

	t.Run("No Rows", func(t *testing.T) {
		mock.ExpectQuery("SELECT status FROM audit_logs").WithArgs(jobID).WillReturnError(sql.ErrNoRows)

		running, err := IsJobRunning(jobID)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if running {
			t.Error("expected job to not be running")
		}
	})

	t.Run("Database Error", func(t *testing.T) {
		mock.ExpectQuery("SELECT status FROM audit_logs").WithArgs(jobID).WillReturnError(errors.New("db error"))

		_, err := IsJobRunning(jobID)
		if err == nil {
			t.Error("expected error, got nil")
		}
	})

	t.Run("DB is nil", func(t *testing.T) {
		DB = nil
		running, err := IsJobRunning(jobID)
		if err != nil {
			t.Errorf("expected no error when DB is nil, got %v", err)
		}
		if running {
			t.Error("expected job to not be running when DB is nil")
		}
		DB = db
	})
}
