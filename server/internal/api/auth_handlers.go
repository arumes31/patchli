package api

import (
	"bytes"
	"encoding/json"
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

	// Never hand out tokens for a node ID that is already enrolled; otherwise
	// a registration signature could be used to take over an existing node.
	// A reinstalled node must wait for its old refresh token to expire or have
	// it revoked by an operator.
	enrolled, err := db.HasActiveRefreshToken(req.MAC)
	if err != nil {
		http.Error(w, "Failed to check enrollment", http.StatusInternalServerError)
		return
	}
	if enrolled {
		http.Error(w, "Node is already enrolled", http.StatusConflict)
		return
	}

	// Each registration nonce can be redeemed exactly once.
	fresh, err := db.ConsumeRegistrationNonce(req.Nonce, t.Add(10*time.Minute))
	if err != nil {
		http.Error(w, "Failed to record registration", http.StatusInternalServerError)
		return
	}
	if !fresh {
		http.Error(w, "Registration signature already used", http.StatusUnauthorized)
		return
	}

	pair, err := auth.GenerateTokenPair(req.MAC)
	if err != nil {
		http.Error(w, "Failed to generate tokens", http.StatusInternalServerError)
		return
	}

	// Store refresh token in DB
	expiresAt := time.Now().Add(auth.RefreshTokenDuration)
	if err := db.StoreRefreshToken(req.MAC, pair.RefreshToken, expiresAt); err != nil {
		http.Error(w, "Failed to store refresh token", http.StatusInternalServerError)
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

	// Token rotation: atomically consume the old token so it can be redeemed
	// at most once.
	consumed, err := db.ConsumeRefreshToken(req.MAC, req.RefreshToken)
	if err != nil {
		log.Printf("Failed to rotate refresh token: %v", err)
		http.Error(w, "Failed to rotate refresh token", http.StatusInternalServerError)
		return
	}
	if !consumed {
		// A correctly signed, unexpired refresh token that is no longer stored
		// has already been rotated: treat it as token theft and revoke every
		// refresh token of the node.
		if err := db.DeleteAllRefreshTokens(req.MAC); err != nil {
			log.Printf("Failed to revoke refresh tokens after reuse: %v", err)
		}
		http.Error(w, "Invalid or expired refresh token", http.StatusUnauthorized)
		return
	}

	// Generate new pair
	pair, err := auth.GenerateTokenPair(req.MAC)
	if err != nil {
		http.Error(w, "Failed to generate new tokens", http.StatusInternalServerError)
		return
	}

	// Store new refresh token
	expiresAt := time.Now().Add(auth.RefreshTokenDuration)
	if err := db.StoreRefreshToken(req.MAC, pair.RefreshToken, expiresAt); err != nil {
		http.Error(w, "Failed to store new refresh token", http.StatusInternalServerError)
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
