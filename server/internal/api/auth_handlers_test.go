package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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

// signedLogin builds a login request with a fresh registration nonce.
func signedLogin(mac string, timestamp string) LoginRequest {
	nonce := auth.NewRegistrationNonce()
	return LoginRequest{
		MAC:       mac,
		Group:     "default",
		Timestamp: timestamp,
		Nonce:     nonce,
		Signature: auth.GenerateRegistrationSignature("default", timestamp, nonce),
	}
}

func TestHandleAgentLogin_Success(t *testing.T) {
	// Enrollment claims are kept in memory without a database, so use a node
	// ID that is unique per run.
	loginReq := signedLogin(fmt.Sprintf("login-success-%d", time.Now().UnixNano()), time.Now().Format(time.RFC3339))
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

	// The same signed registration cannot be redeemed twice; the node is
	// already claimed by the first redemption.
	replay := httptest.NewRecorder()
	HandleAgentLogin(replay, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body)))
	if replay.Code != http.StatusConflict || strings.Contains(replay.Body.String(), "access_token") {
		t.Fatalf("replayed registration: status=%d body=%s", replay.Code, replay.Body.String())
	}

	// A replayed signature cannot mint tokens for a different node ID either.
	loginReq.MAC += "-other"
	other, _ := json.Marshal(loginReq)
	replay = httptest.NewRecorder()
	HandleAgentLogin(replay, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(other)))
	if replay.Code != http.StatusUnauthorized || !strings.Contains(replay.Body.String(), "already used") {
		t.Fatalf("replayed registration for another node: status=%d body=%s", replay.Code, replay.Body.String())
	}
}

func expectNodeLock(mock sqlmock.Sqlmock, mac string) {
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WithArgs("patchli-node:" + mac).WillReturnResult(sqlmock.NewResult(0, 0))
}

func TestHandleAgentLoginRejectsEnrolledNode(t *testing.T) {
	mock := apiDatabase(t)
	expectNodeLock(mock, "node")
	mock.ExpectQuery("SELECT EXISTS").WithArgs("node").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectRollback()
	body, err := json.Marshal(signedLogin("node", time.Now().Format(time.RFC3339)))
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	HandleAgentLogin(rr, httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body)))
	if rr.Code != http.StatusConflict || strings.Contains(rr.Body.String(), "access_token") {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleAgentLoginRejectsTakeoverWithoutDatabase(t *testing.T) {
	mac := fmt.Sprintf("memory-takeover-%d", time.Now().UnixNano())
	first, err := json.Marshal(signedLogin(mac, time.Now().Format(time.RFC3339)))
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	HandleAgentLogin(rr, httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(first)))
	if rr.Code != http.StatusOK {
		t.Fatalf("first enrollment: status=%d body=%s", rr.Code, rr.Body.String())
	}
	// A different, fresh registration grant cannot claim the same node.
	second, err := json.Marshal(signedLogin(mac, time.Now().Format(time.RFC3339)))
	if err != nil {
		t.Fatal(err)
	}
	rr = httptest.NewRecorder()
	HandleAgentLogin(rr, httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(second)))
	if rr.Code != http.StatusConflict || strings.Contains(rr.Body.String(), "access_token") {
		t.Fatalf("takeover: status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleRevokeEnrollment(t *testing.T) {
	for _, tc := range []struct {
		name, method, body string
		prepare            func(sqlmock.Sqlmock)
		status             int
	}{
		{"method", http.MethodGet, "", nil, http.StatusMethodNotAllowed},
		{"missing mac", http.MethodPost, `{}`, nil, http.StatusBadRequest},
		{"revoked", http.MethodPost, `{"mac":"node"}`, func(m sqlmock.Sqlmock) {
			expectNodeLock(m, "node")
			m.ExpectExec("DELETE FROM refresh_tokens").WithArgs("node").WillReturnResult(sqlmock.NewResult(0, 1))
			m.ExpectExec("DELETE FROM node_enrollments").WithArgs("node").WillReturnResult(sqlmock.NewResult(0, 1))
			m.ExpectCommit()
		}, http.StatusNoContent},
		{"database failure", http.MethodPost, `{"mac":"node"}`, func(m sqlmock.Sqlmock) {
			m.ExpectBegin().WillReturnError(errors.New("unavailable"))
		}, http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mock := apiDatabase(t)
			if tc.prepare != nil {
				tc.prepare(mock)
			}
			rr := httptest.NewRecorder()
			HandleRevokeEnrollment(rr, httptest.NewRequest(tc.method, "/", strings.NewReader(tc.body)))
			if rr.Code != tc.status {
				t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
			}
		})
	}
}

func TestHandleAgentLoginRejectsOversizedBody(t *testing.T) {
	body := `{"mac":"` + strings.Repeat("a", maxAuthBodyBytes) + `"}`
	rr := httptest.NewRecorder()
	HandleAgentLogin(rr, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rr.Code)
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
	body, err := json.Marshal(signedLogin("node", timestamp))
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
	expectNodeLock(mock, "node")
	mock.ExpectQuery("SELECT EXISTS").WithArgs("node").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectExec("DELETE FROM registration_nonces").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO registration_nonces").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO node_enrollments").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO refresh_tokens").WithArgs("node", sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnError(errors.New("storage unavailable"))
	mock.ExpectRollback()
	timestamp := time.Now().Format(time.RFC3339)
	body, err := json.Marshal(signedLogin("node", timestamp))
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
		name                                                   string
		reused, consumeError, invalid, mismatch, access, store bool
		storeError                                             bool
		status                                                 int
		message                                                string
	}{
		{name: "success", status: http.StatusOK},
		{name: "reused token revokes node", reused: true, status: http.StatusUnauthorized, message: "Invalid or expired"},
		{name: "database unavailable", consumeError: true, status: http.StatusInternalServerError, message: "Failed to rotate"},
		{name: "invalid jwt", invalid: true, status: http.StatusUnauthorized, message: "Invalid refresh token payload"},
		{name: "different subject", mismatch: true, status: http.StatusUnauthorized, message: "Invalid refresh token payload"},
		{name: "access token", access: true, status: http.StatusUnauthorized, message: "Invalid refresh token payload"},
		{name: "store failure", storeError: true, status: http.StatusInternalServerError, message: "Failed to rotate"},
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
			if tc.access {
				token = pair.AccessToken
			}
			hash := sha256.Sum256([]byte(token))
			digest := hex.EncodeToString(hash[:])
			if !tc.invalid && !tc.mismatch && !tc.access {
				expectNodeLock(mock, "node")
				consume := mock.ExpectExec("DELETE FROM refresh_tokens WHERE mac_address = \\$1 AND token_hash = \\$2").WithArgs("node", digest)
				switch {
				case tc.consumeError:
					consume.WillReturnError(errors.New("unavailable"))
					mock.ExpectRollback()
				case tc.reused:
					consume.WillReturnResult(sqlmock.NewResult(0, 0))
					mock.ExpectExec("DELETE FROM refresh_tokens WHERE mac_address = \\$1").WithArgs("node").WillReturnResult(sqlmock.NewResult(0, 2))
					mock.ExpectCommit()
				default:
					consume.WillReturnResult(sqlmock.NewResult(0, 1))
					insertion := mock.ExpectExec("INSERT INTO refresh_tokens").WithArgs("node", sqlmock.AnyArg(), sqlmock.AnyArg())
					if tc.storeError {
						insertion.WillReturnError(errors.New("unavailable"))
						mock.ExpectRollback()
					} else {
						insertion.WillReturnResult(sqlmock.NewResult(1, 1))
						mock.ExpectExec("INSERT INTO node_enrollments").WithArgs("node", sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
						mock.ExpectCommit()
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
			access, err := auth.ValidateAccessToken(received.AccessToken)
			if err != nil || (*access)["sub"] != "node" {
				t.Fatalf("bad access token: %v %v", access, err)
			}
			refresh, err := auth.ValidateRefreshToken(received.RefreshToken)
			if err != nil || (*refresh)["sub"] != "node" {
				t.Fatalf("bad refresh token: %v %v", refresh, err)
			}
		})
	}
}

func TestHandleAgentLoginDatabaseFailures(t *testing.T) {
	for _, tc := range []struct {
		name    string
		prepare func(sqlmock.Sqlmock)
	}{
		{"transaction", func(m sqlmock.Sqlmock) {
			m.ExpectBegin().WillReturnError(errors.New("unavailable"))
		}},
		{"enrollment check", func(m sqlmock.Sqlmock) {
			expectNodeLock(m, "node")
			m.ExpectQuery("SELECT EXISTS").WillReturnError(errors.New("unavailable"))
			m.ExpectRollback()
		}},
		{"nonce storage", func(m sqlmock.Sqlmock) {
			expectNodeLock(m, "node")
			m.ExpectQuery("SELECT EXISTS").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
			m.ExpectExec("DELETE FROM registration_nonces").WillReturnError(errors.New("unavailable"))
			m.ExpectRollback()
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.prepare(apiDatabase(t))
			body, err := json.Marshal(signedLogin("node", time.Now().Format(time.RFC3339)))
			if err != nil {
				t.Fatal(err)
			}
			rr := httptest.NewRecorder()
			HandleAgentLogin(rr, httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body)))
			if rr.Code != http.StatusInternalServerError || !strings.Contains(rr.Body.String(), "Failed to record enrollment") {
				t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
			}
		})
	}
}
