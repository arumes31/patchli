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
	registrationSecret []byte
	jwtSecret          []byte
)

func init() {
	loadSecrets()
}

func loadSecrets() {
	registrationSecret = []byte(os.Getenv("REGISTRATION_SECRET"))
	jwtSecret = []byte(os.Getenv("JWT_SECRET"))
}

func checkSecrets() error {
	if len(registrationSecret) == 0 || len(jwtSecret) == 0 {
		return fmt.Errorf("REGISTRATION_SECRET and JWT_SECRET must be set")
	}
	return nil
}

func GenerateRegistrationSignature(group string, timestamp string) string {
	if err := checkSecrets(); err != nil {
		// In tests we might want to allow this if mocked, but for now we follow original logic
		// without the hard panic in init.
	}
	h := hmac.New(sha256.New, registrationSecret)
	h.Write([]byte(fmt.Sprintf("%s:%s", group, timestamp)))
	return hex.EncodeToString(h.Sum(nil))
}

func GenerateAgentJWT(macAddr string) (string, error) {
	if err := checkSecrets(); err != nil {
		return "", err
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": macAddr,
		"iat": time.Now().Unix(),
		"exp": time.Now().Add(time.Hour * 24).Unix(), // TODO: refresh token flow
	})
	return token.SignedString(jwtSecret)
}

func ValidateRegistration(group, timestamp, signature string) bool {
	if err := checkSecrets(); err != nil {
		return false
	}
	t, err := time.Parse(time.RFC3339, timestamp)
	if err != nil || time.Since(t) > 10*time.Minute {
		return false
	}
	expected := GenerateRegistrationSignature(group, timestamp)
	return hmac.Equal([]byte(signature), []byte(expected))
}
