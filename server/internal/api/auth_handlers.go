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

type LoginRequest struct {
	MAC       string `json:"mac"`
	Group     string `json:"group"`
	Timestamp string `json:"timestamp"`
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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if !auth.ValidateRegistration(req.Group, req.Timestamp, req.Signature) {
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

	// Store refresh token in DB
	expiresAt := time.Now().Add(auth.RefreshTokenDuration)
	if err := db.StoreRefreshToken(req.MAC, pair.RefreshToken, expiresAt); err != nil {
		http.Error(w, "Failed to store refresh token", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(pair); err != nil { // #nosec G117 -- legitimate encoding of auth tokens
		log.Printf("Error encoding login response: %v", err)
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
	_, _ = w.Write(buf.Bytes()) // #nosec G104
}

func HandleAgentRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req RefreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Verify token in DB
	valid, err := db.VerifyRefreshToken(req.MAC, req.RefreshToken)
	if err != nil || !valid {
		http.Error(w, "Invalid or expired refresh token", http.StatusUnauthorized)
		return
	}

	// Also validate JWT signature and claims
	claims, err := auth.ValidateToken(req.RefreshToken)
	if err != nil || claims == nil || (*claims)["sub"] != req.MAC {
		http.Error(w, "Invalid refresh token payload", http.StatusUnauthorized)
		return
	}

	// Token rotation: delete old one
	if err := db.DeleteRefreshToken(req.MAC, req.RefreshToken); err != nil {
		log.Printf("Failed to delete old refresh token: %v", err)
		http.Error(w, "Failed to rotate refresh token", http.StatusInternalServerError)
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
	if err := json.NewEncoder(&buf).Encode(pair); err != nil { // #nosec G117 -- legitimate encoding of auth tokens
		log.Printf("Error encoding refresh response: %v", err)
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
	_, _ = w.Write(buf.Bytes()) // #nosec G104
}
