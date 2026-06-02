package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/arumes31/patchli/server/internal/auth"
)

func TestHandleAgentLogin_Unauthorized(t *testing.T) {
	os.Setenv("REGISTRATION_SECRET", "testsecret")
	os.Setenv("JWT_SECRET", "atleast16charslongsecret")

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
	os.Setenv("REGISTRATION_SECRET", "testsecret")
	os.Setenv("JWT_SECRET", "atleast16charslongsecret")

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
