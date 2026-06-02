package auth

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
	registrationSecret = []byte(os.Getenv("REGISTRATION_SECRET"))
	jwtSecret          = []byte(os.Getenv("JWT_SECRET"))
)

func GenerateRegistrationSignature(group string, timestamp string) string {
	if len(registrationSecret) == 0 {
		return ""
	}
	h := hmac.New(sha256.New, registrationSecret)
	h.Write([]byte(fmt.Sprintf("%s:%s", group, timestamp)))
	return hex.EncodeToString(h.Sum(nil))
}

func GenerateAgentJWT(macAddr string) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": macAddr,
		"iat": time.Now().Unix(),
		"exp": time.Now().Add(time.Hour * 24).Unix(), // TODO: refresh token flow
	})
	if len(jwtSecret) == 0 {
		return "", fmt.Errorf("JWT_SECRET not set")
	}
	return token.SignedString(jwtSecret)
}

func ValidateRegistration(group, timestamp, signature string) bool {
	if len(registrationSecret) == 0 {
		return false
	}
	t, err := time.Parse(time.RFC3339, timestamp)
	if err != nil || time.Since(t) > 10*time.Minute {
		return false
	}
	expected := GenerateRegistrationSignature(group, timestamp)
	return hmac.Equal([]byte(signature), []byte(expected))
}
