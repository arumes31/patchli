package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
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

	// We don't have a real DB in this unit test, but db.DB is nil,
	// and our StoreRefreshToken handles nil DB gracefully by returning nil error.
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
}

func apiDatabase(t *testing.T) sqlmock.Sqlmock {
	t.Helper()
	conn, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	old := db.DB
	db.DB = conn
	t.Cleanup(func() {
		db.DB = old
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
		_ = conn.Close()
	})
	return mock
}

func TestAuthHandlersRejectMalformedRequests(t *testing.T) {
	for _, h := range []struct {
		name    string
		handler http.HandlerFunc
	}{
		{"login", HandleAgentLogin}, {"refresh", HandleAgentRefresh},
	} {
		t.Run(h.name, func(t *testing.T) {
			for _, tc := range []struct {
				name, method, body string
				status             int
			}{
				{"method", http.MethodGet, "", http.StatusMethodNotAllowed},
				{"malformed json", http.MethodPost, "{", http.StatusBadRequest},
			} {
				t.Run(tc.name, func(t *testing.T) {
					rr := httptest.NewRecorder()
					h.handler(rr, httptest.NewRequest(tc.method, "/", strings.NewReader(tc.body)))
					if rr.Code != tc.status {
						t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
					}
				})
			}
		})
	}
}

func TestHandleAgentLoginRejectsStaleSignedRequest(t *testing.T) {
	timestamp := time.Now().Add(-7 * time.Minute).Format(time.RFC3339)
	body, err := json.Marshal(LoginRequest{MAC: "node", Group: "default", Timestamp: timestamp, Signature: auth.GenerateRegistrationSignature("default", timestamp)})
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	HandleAgentLogin(rr, httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body)))
	if rr.Code != http.StatusUnauthorized || !strings.Contains(rr.Body.String(), "Stale") {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleAgentLoginStorageFailure(t *testing.T) {
	mock := apiDatabase(t)
	mock.ExpectExec("INSERT INTO refresh_tokens").WithArgs("node", sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnError(errors.New("storage unavailable"))
	timestamp := time.Now().Format(time.RFC3339)
	body, err := json.Marshal(LoginRequest{MAC: "node", Group: "default", Timestamp: timestamp, Signature: auth.GenerateRegistrationSignature("default", timestamp)})
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	HandleAgentLogin(rr, httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body)))
	if rr.Code != http.StatusInternalServerError || strings.Contains(rr.Body.String(), "access_token") {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleAgentRefreshRotation(t *testing.T) {
	for _, tc := range []struct {
		name                                                            string
		missing, queryError, invalid, mismatch, deleteError, storeError bool
		status                                                          int
		message                                                         string
	}{
		{name: "success", status: http.StatusOK},
		{name: "unknown token", missing: true, status: http.StatusUnauthorized, message: "Invalid or expired"},
		{name: "database unavailable", queryError: true, status: http.StatusUnauthorized, message: "Invalid or expired"},
		{name: "invalid jwt", invalid: true, status: http.StatusUnauthorized, message: "Invalid refresh token payload"},
		{name: "different subject", mismatch: true, status: http.StatusUnauthorized, message: "Invalid refresh token payload"},
		{name: "delete failure", deleteError: true, status: http.StatusInternalServerError, message: "Failed to rotate"},
		{name: "store failure", storeError: true, status: http.StatusInternalServerError, message: "Failed to store"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mock := apiDatabase(t)
			subject := "node"
			if tc.mismatch {
				subject = "other-node"
			}
			pair, err := auth.GenerateTokenPair(subject)
			if err != nil {
				t.Fatal(err)
			}
			token := pair.RefreshToken
			if tc.invalid {
				token = "not-a-jwt"
			}
			hash := sha256.Sum256([]byte(token))
			digest := hex.EncodeToString(hash[:])
			query := mock.ExpectQuery("SELECT EXISTS").WithArgs("node", digest)
			if tc.queryError {
				query.WillReturnError(errors.New("unavailable"))
			} else {
				query.WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(!tc.missing))
			}
			if !tc.queryError && !tc.missing && !tc.invalid && !tc.mismatch {
				deletion := mock.ExpectExec("DELETE FROM refresh_tokens").WithArgs("node", digest)
				if tc.deleteError {
					deletion.WillReturnError(errors.New("unavailable"))
				} else {
					deletion.WillReturnResult(sqlmock.NewResult(0, 1))
					insertion := mock.ExpectExec("INSERT INTO refresh_tokens").WithArgs("node", sqlmock.AnyArg(), sqlmock.AnyArg())
					if tc.storeError {
						insertion.WillReturnError(errors.New("unavailable"))
					} else {
						insertion.WillReturnResult(sqlmock.NewResult(1, 1))
					}
				}
			}
			body, err := json.Marshal(RefreshRequest{MAC: "node", RefreshToken: token})
			if err != nil {
				t.Fatal(err)
			}
			rr := httptest.NewRecorder()
			HandleAgentRefresh(rr, httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body)))
			if rr.Code != tc.status {
				t.Fatalf("status=%d want=%d body=%s", rr.Code, tc.status, rr.Body.String())
			}
			if tc.status != http.StatusOK {
				if !strings.Contains(rr.Body.String(), tc.message) {
					t.Fatalf("body=%s want=%s", rr.Body.String(), tc.message)
				}
				return
			}
			if rr.Header().Get("Content-Type") != "application/json" {
				t.Fatal("expected json response")
			}
			var received auth.TokenPair
			if err := json.Unmarshal(rr.Body.Bytes(), &received); err != nil {
				t.Fatal(err)
			}
			for _, token := range []string{received.AccessToken, received.RefreshToken} {
				claims, err := auth.ValidateToken(token)
				if err != nil {
					t.Fatal(err)
				}
				if (*claims)["sub"] != "node" {
					t.Fatalf("wrong subject: %v", claims)
				}
			}
		})
	}
}
