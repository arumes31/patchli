package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/arumes31/patchli/server/internal/auth"
	"github.com/arumes31/patchli/server/internal/db"
)

// maxAuthBodyBytes caps login/refresh request bodies.
const maxAuthBodyBytes = 64 << 10

type LoginRequest struct {
	MAC       string `json:"mac"`
	Group     string `json:"group"`
	Timestamp string `json:"timestamp"`
	Nonce     string `json:"nonce"`
	Signature string `json:"signature"`
}

type RevokeRequest struct {
	MAC string `json:"mac"`
}

type RefreshRequest struct {
	MAC          string `json:"mac"`
	RefreshToken string `json:"refresh_token"`
}

func HandleAgentLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req LoginRequest
	r.Body = http.MaxBytesReader(w, r.Body, maxAuthBodyBytes)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.MAC == "" {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if !auth.ValidateRegistration(req.Group, req.Timestamp, req.Nonce, req.Signature) {
		http.Error(w, "Invalid registration signature", http.StatusUnauthorized)
		return
	}

	// Verify timestamp freshness to prevent replay attacks
	t, err := time.Parse(time.RFC3339, req.Timestamp)
	if err != nil || time.Since(t) > 5*time.Minute || time.Until(t) > 5*time.Minute {
		http.Error(w, "Stale or invalid timestamp", http.StatusUnauthorized)
		return
	}

	pair, err := auth.GenerateTokenPair(req.MAC)
	if err != nil {
		http.Error(w, "Failed to generate tokens", http.StatusInternalServerError)
		return
	}

	// Claim the node ID, redeem the nonce and store the refresh token as one
	// step. A node ID that is already enrolled is never handed out again, even
	// while its refresh token is being rotated; otherwise a registration grant
	// could take over an existing node. A reinstalled node must wait for the
	// old enrollment to expire or have it revoked by an operator.
	expiresAt := time.Now().Add(auth.RefreshTokenDuration)
	switch err := db.EnrollNode(req.MAC, req.Nonce, t.Add(10*time.Minute), pair.RefreshToken, expiresAt); {
	case errors.Is(err, db.ErrAlreadyEnrolled):
		http.Error(w, "Node is already enrolled", http.StatusConflict)
		return
	case errors.Is(err, db.ErrNonceUsed):
		http.Error(w, "Registration signature already used", http.StatusUnauthorized)
		return
	case err != nil:
		log.Printf("Failed to enroll node: %v", err)
		http.Error(w, "Failed to record enrollment", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(pair); err != nil { // #nosec G117 -- Authenticated login/refresh must return the token pair to its client.
		log.Printf("Error encoding login response: %v", err)
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
	if _, err := w.Write(buf.Bytes()); err != nil {
		log.Printf("Response write failed: %v", err)
	}
}

func HandleAgentRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req RefreshRequest
	r.Body = http.MaxBytesReader(w, r.Body, maxAuthBodyBytes)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate JWT signature, type and subject first
	claims, err := auth.ValidateRefreshToken(req.RefreshToken)
	if err != nil || claims == nil || (*claims)["sub"] != req.MAC {
		http.Error(w, "Invalid refresh token payload", http.StatusUnauthorized)
		return
	}

	pair, err := auth.GenerateTokenPair(req.MAC)
	if err != nil {
		http.Error(w, "Failed to generate new tokens", http.StatusInternalServerError)
		return
	}

	// Token rotation: the old token is redeemed at most once. A correctly
	// signed, unexpired refresh token that is no longer stored has already
	// been rotated, so it is treated as theft and every refresh token of the
	// node is revoked, including replacements issued by concurrent requests.
	expiresAt := time.Now().Add(auth.RefreshTokenDuration)
	rotated, err := db.RotateRefreshToken(req.MAC, req.RefreshToken, pair.RefreshToken, expiresAt)
	if err != nil {
		log.Printf("Failed to rotate refresh token: %v", err)
		http.Error(w, "Failed to rotate refresh token", http.StatusInternalServerError)
		return
	}
	if !rotated {
		http.Error(w, "Invalid or expired refresh token", http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(pair); err != nil { // #nosec G117 -- Authenticated login/refresh must return the token pair to its client.
		log.Printf("Error encoding refresh response: %v", err)
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
	if _, err := w.Write(buf.Bytes()); err != nil {
		log.Printf("Response write failed: %v", err)
	}
}

// HandleRevokeEnrollment lets an operator release a node's enrollment and
// revoke its refresh tokens so a reinstalled node can enroll again.
func HandleRevokeEnrollment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req RevokeRequest
	r.Body = http.MaxBytesReader(w, r.Body, maxAuthBodyBytes)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.MAC == "" {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := db.RevokeEnrollment(req.MAC); err != nil {
		log.Printf("Failed to revoke enrollment: %v", err)
		http.Error(w, "Failed to revoke enrollment", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
