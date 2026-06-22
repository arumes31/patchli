package api

import (
	"encoding/json"
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
	_ = json.NewEncoder(w).Encode(pair) // #nosec G104 -- Error is unhandled intentionally.
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
	if err != nil || (*claims)["sub"] != req.MAC {
		http.Error(w, "Invalid refresh token payload", http.StatusUnauthorized)
		return
	}

	// Token rotation: delete old one
	_ = db.DeleteRefreshToken(req.MAC, req.RefreshToken)

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
	_ = json.NewEncoder(w).Encode(pair) // #nosec G104 -- Error is unhandled intentionally.
}
