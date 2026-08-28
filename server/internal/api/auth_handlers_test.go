package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/arumes31/patchli/server/internal/auth"
	"github.com/arumes31/patchli/server/internal/db"
)

func TestHandleAgentLogin_Unauthorized(t *testing.T) {
	loginReq := LoginRequest{
		MAC:       "00:11:22:33:44:55",
		Group:     "default",
		Timestamp: time.Now().Format(time.RFC3339),
		Signature: "wrong-signature",
	}
	body, _ := json.Marshal(loginReq)

	req, _ := http.NewRequest("POST", "/api/v1/auth/login", bytes.NewBuffer(body))
	rr := httptest.NewRecorder()

	HandleAgentLogin(rr, req)

	if status := rr.Code; status != http.StatusUnauthorized {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusUnauthorized)
	}
}

func TestHandleAgentLogin_Success(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	db.DB = database
	t.Cleanup(func() {
		db.DB = nil
		_ = database.Close()
	})
	mock.ExpectExec("INSERT INTO refresh_tokens").WithArgs("00:11:22:33:44:55", sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(1, 1))

	timestamp := time.Now().Format(time.RFC3339)
	signature := auth.GenerateRegistrationSignature("default", timestamp)

	loginReq := LoginRequest{
		MAC:       "00:11:22:33:44:55",
		Group:     "default",
		Timestamp: timestamp,
		Signature: signature,
	}
	body, _ := json.Marshal(loginReq)

	req, _ := http.NewRequest("POST", "/api/v1/auth/login", bytes.NewBuffer(body))
	rr := httptest.NewRecorder()

	HandleAgentLogin(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	var pair auth.TokenPair
	if err := json.Unmarshal(rr.Body.Bytes(), &pair); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}
	if pair.AccessToken == "" || pair.RefreshToken == "" {
		t.Error("Returned tokens should not be empty")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
