package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	registrationSecret = []byte(os.Getenv("REGISTRATION_SECRET"))
	jwtSecret          = []byte(os.Getenv("JWT_SECRET"))
)

const (
	AccessTokenDuration  = time.Hour * 1
	RefreshTokenDuration = time.Hour * 24 * 7
)

type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

func init() {
	if len(registrationSecret) == 0 || len(jwtSecret) == 0 {
		panic("REGISTRATION_SECRET and JWT_SECRET must be set")
	}
}

func GenerateRegistrationSignature(group string, timestamp string) string {
	h := hmac.New(sha256.New, registrationSecret)
	h.Write([]byte(fmt.Sprintf("%s:%s", group, timestamp)))
	return hex.EncodeToString(h.Sum(nil))
}

func GenerateAgentJWT(macAddr string) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": macAddr,
		"iat": time.Now().Unix(),
		"exp": time.Now().Add(AccessTokenDuration).Unix(),
	})
	return token.SignedString(jwtSecret)
}

func GenerateTokenPair(macAddr string) (*TokenPair, error) {
	accessToken, err := GenerateAgentJWT(macAddr)
	if err != nil {
		return nil, err
	}

	refreshToken := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": macAddr,
		"iat": time.Now().Unix(),
		"exp": time.Now().Add(RefreshTokenDuration).Unix(),
	})
	refreshStr, err := refreshToken.SignedString(jwtSecret)
	if err != nil {
		return nil, err
	}

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshStr,
	}, nil
}

func ValidateToken(tokenStr string) (*jwt.MapClaims, error) {
	token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return jwtSecret, nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		return &claims, nil
	}

	return nil, errors.New("invalid token")
}

func ValidateRegistration(group, timestamp, signature string) bool {
	// Always perform both checks to avoid timing leaks
	t, err := time.Parse(time.RFC3339, timestamp)
	timestampValid := err == nil && time.Since(t) <= 10*time.Minute && time.Until(t) <= 5*time.Minute

	expected := GenerateRegistrationSignature(group, timestamp)
	signatureValid := hmac.Equal([]byte(signature), []byte(expected))

	return timestampValid && signatureValid
}
