package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/arumes31/patchli/server/internal/db"
)

func TestHealthReflectsDatabaseAvailability(t *testing.T) {
	original := db.DB
	t.Cleanup(func() { db.DB = original })
	for _, tt := range []struct {
		name    string
		pingErr error
		want    int
	}{
		{"available", nil, http.StatusOK},
		{"unavailable", errors.New("connection lost"), http.StatusServiceUnavailable},
	} {
		t.Run(tt.name, func(t *testing.T) {
			database, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = database.Close() }()
			db.DB = database
			mock.ExpectPing().WillReturnError(tt.pingErr)
			response := httptest.NewRecorder()
			handleHealth(response, httptest.NewRequest(http.MethodGet, "/health", nil))
			if response.Code != tt.want {
				t.Fatalf("status %d, want %d", response.Code, tt.want)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
	db.DB = nil
	response := httptest.NewRecorder()
	handleHealth(response, httptest.NewRequest(http.MethodGet, "/health", nil))
	if response.Code != http.StatusOK {
		t.Fatal("optional database should not fail health")
	}
}
