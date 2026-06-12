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

func getRegistrationSecret() []byte {
	return []byte(os.Getenv("REGISTRATION_SECRET"))
}

func getJWTSecret() []byte {
	return []byte(os.Getenv("JWT_SECRET"))
}

func GenerateRegistrationSignature(group string, timestamp string) string {
	h := hmac.New(sha256.New, getRegistrationSecret())
	h.Write([]byte(fmt.Sprintf("%s:%s", group, timestamp)))
	return hex.EncodeToString(h.Sum(nil))
}

func GenerateAgentJWT(macAddr string) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": macAddr,
		"iat": time.Now().Unix(),
		"exp": time.Now().Add(time.Hour * 24).Unix(), // TODO: refresh token flow
	})
	return token.SignedString(getJWTSecret())
}

func ValidateRegistration(group, timestamp, signature string) bool {
	t, err := time.Parse(time.RFC3339, timestamp)
	if err != nil || time.Since(t) > 10*time.Minute {
		return false
	}
	expected := GenerateRegistrationSignature(group, timestamp)
	return hmac.Equal([]byte(signature), []byte(expected))
}
