package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	registrationSecret []byte
	jwtSecret          []byte
)

const (
	AccessTokenDuration  = time.Hour * 1
	RefreshTokenDuration = time.Hour * 24 * 7
	tokenIssuer          = "patchli"
	tokenAudience        = "patchli-agent"
	accessTokenUse       = "access"
	refreshTokenUse      = "refresh"
	minimumSecretBytes   = 32
)

type Claims struct {
	TokenUse string `json:"token_use"`
	jwt.RegisteredClaims
}

type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

// InitSecrets loads and validates independent high-entropy signing secrets.
func InitSecrets() error {
	registration := os.Getenv("REGISTRATION_SECRET")
	jwtSigning := os.Getenv("JWT_SECRET")
	if len(registration) < minimumSecretBytes || len(jwtSigning) < minimumSecretBytes {
		return fmt.Errorf("REGISTRATION_SECRET and JWT_SECRET must each contain at least %d bytes", minimumSecretBytes)
	}
	if strings.TrimSpace(registration) != registration || strings.TrimSpace(jwtSigning) != jwtSigning {
		return errors.New("signing secrets must not have leading or trailing whitespace")
	}
	if registration == jwtSigning {
		return errors.New("REGISTRATION_SECRET and JWT_SECRET must be distinct")
	}
	registrationSecret = []byte(registration)
	jwtSecret = []byte(jwtSigning)
	return nil
}

// SetTestSecrets sets dummy secrets for unit tests.
func SetTestSecrets() {
	registrationSecret = []byte("test-registration-secret-32-bytes-long")
	jwtSecret = []byte("test-jwt-secret-that-is-long-enough")
}

func GenerateRegistrationSignature(group string, timestamp string) string {
	h := hmac.New(sha256.New, registrationSecret)
	_, _ = fmt.Fprintf(h, "%s:%s", group, timestamp)
	return hex.EncodeToString(h.Sum(nil))
}

func GenerateAgentJWT(macAddr string) (string, error) {
	return generateToken(macAddr, accessTokenUse, AccessTokenDuration)
}

func GenerateTokenPair(macAddr string) (*TokenPair, error) {
	accessToken, err := GenerateAgentJWT(macAddr)
	if err != nil {
		return nil, err
	}

	refreshStr, err := generateToken(macAddr, refreshTokenUse, RefreshTokenDuration)
	if err != nil {
		return nil, err
	}

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshStr,
	}, nil
}

func generateToken(subject, tokenUse string, duration time.Duration) (string, error) {
	now := time.Now()
	claims := Claims{
		TokenUse: tokenUse,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    tokenIssuer,
			Subject:   subject,
			Audience:  jwt.ClaimStrings{tokenAudience}, // #nosec G101 -- public JWT audience, not a credential.
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now.Add(-30 * time.Second)),
			ExpiresAt: jwt.NewNumericDate(now.Add(duration)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(jwtSecret)
}

func validateToken(tokenString, expectedUse string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, errors.New("unexpected signing method")
		}
		return jwtSecret, nil
	}, jwt.WithIssuer(tokenIssuer), jwt.WithAudience(tokenAudience), jwt.WithExpirationRequired(), jwt.WithIssuedAt())
	if err != nil || !token.Valid || claims.TokenUse != expectedUse || claims.Subject == "" {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}

// ValidateAccessToken verifies an access token and returns its bound agent subject.
func ValidateAccessToken(tokenString string) (*Claims, error) {
	return validateToken(tokenString, accessTokenUse)
}

// ValidateRefreshToken verifies a refresh token and returns its bound agent subject.
func ValidateRefreshToken(tokenString string) (*Claims, error) {
	return validateToken(tokenString, refreshTokenUse)
}

func ValidateRegistration(group, timestamp, signature string) bool {
	// Always perform both checks to avoid timing leaks
	t, err := time.Parse(time.RFC3339, timestamp)
	timestampValid := err == nil && time.Since(t) <= 10*time.Minute && time.Until(t) <= 5*time.Minute

	expected := GenerateRegistrationSignature(group, timestamp)
	signatureValid := hmac.Equal([]byte(signature), []byte(expected))

	return timestampValid && signatureValid
}
