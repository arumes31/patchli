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

func init() {
	if len(registrationSecret) == 0 || len(jwtSecret) == 0 {
		panic("REGISTRATION_SECRET and JWT_SECRET must be set")
	}
}

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

func GenerateRegistrationSignature(group string, timestamp string) string {
	h := hmac.New(sha256.New, registrationSecret)
	h.Write([]byte(fmt.Sprintf("%s:%s", group, timestamp)))
	return hex.EncodeToString(h.Sum(nil))
}

func GenerateAgentJWT(macAddr string) (TokenResponse, error) {
	// Access Token (1 hour)
	accessToken := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": macAddr,
		"iat": time.Now().Unix(),
		"exp": time.Now().Add(time.Hour).Unix(),
		"typ": "access",
	})
	accessTokenStr, err := accessToken.SignedString(jwtSecret)
	if err != nil {
		return TokenResponse{}, err
	}

	// Refresh Token (30 days)
	refreshToken := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": macAddr,
		"iat": time.Now().Unix(),
		"exp": time.Now().Add(time.Hour * 24 * 30).Unix(),
		"typ": "refresh",
	})
	refreshTokenStr, err := refreshToken.SignedString(jwtSecret)
	if err != nil {
		return TokenResponse{}, err
	}

	return TokenResponse{
		AccessToken:  accessTokenStr,
		RefreshToken: refreshTokenStr,
	}, nil
}

func ValidateRefreshToken(tokenStr string) (string, error) {
	token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return jwtSecret, nil
	})

	if err != nil {
		return "", err
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		if typ, ok := claims["typ"].(string); !ok || typ != "refresh" {
			return "", fmt.Errorf("invalid token type")
		}
		sub, ok := claims["sub"].(string)
		if !ok {
			return "", fmt.Errorf("invalid subject")
		}
		return sub, nil
	}

	return "", fmt.Errorf("invalid token")
}

func ValidateRegistration(group, timestamp, signature string) bool {
	t, err := time.Parse(time.RFC3339, timestamp)
	if err != nil || time.Since(t) > 10*time.Minute {
		return false
	}
	expected := GenerateRegistrationSignature(group, timestamp)
	return hmac.Equal([]byte(signature), []byte(expected))
}
