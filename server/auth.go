package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	// In a real app, load these from environment variables or a secure vault
	registrationSecret = []byte(os.Getenv("REGISTRATION_SECRET")) // Pre-shared key for setup
	jwtSecret          = []byte(os.Getenv("JWT_SECRET"))          // For persistent agent auth
)

func init() {
	if len(registrationSecret) == 0 {
		registrationSecret = []byte("default-registration-secret-change-me")
	}
	if len(jwtSecret) == 0 {
		jwtSecret = []byte("default-jwt-secret-change-me")
	}
}

// GenerateRegistrationSignature creates an HMAC signature for a specific group and timestamp.
func GenerateRegistrationSignature(group string, timestamp string) string {
	h := hmac.New(sha256.New, registrationSecret)
	h.Write([]byte(fmt.Sprintf("%s:%s", group, timestamp)))
	return hex.EncodeToString(h.Sum(nil))
}

// GenerateAgentJWT creates a persistent JWT for a registered agent.
func GenerateAgentJWT(macAddr string) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": macAddr,
		"iat": time.Now().Unix(),
		"exp": time.Now().Add(time.Hour * 24 * 365).Unix(), // Long-lived for agents
	})
	return token.SignedString(jwtSecret)
}

// ValidateRegistration checks if the registration request is authentic.
func ValidateRegistration(group, timestamp, signature string) bool {
	// 1. Check if timestamp is within a reasonable window (e.g., 10 minutes)
	t, err := time.Parse(time.RFC3339, timestamp)
	if err != nil || time.Since(t) > 10*time.Minute {
		return false
	}

	// 2. Re-calculate signature
	expected := GenerateRegistrationSignature(group, timestamp)
	return hmac.Equal([]byte(signature), []byte(expected))
}
