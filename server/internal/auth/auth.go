package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
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

	// TokenAudience is the "aud" claim carried by every agent token.
	TokenAudience = "patchli-agent"
	// TokenTypeAccess and TokenTypeRefresh are the values of the "typ" claim.
	// Validators enforce them so a refresh token cannot be used as an access
	// token (and vice versa).
	TokenTypeAccess  = "access"
	TokenTypeRefresh = "refresh"
)

type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

// InitSecrets loads secrets from environment variables. In production, the
// server main function calls this and panics if either variable is empty.
// During testing, SetTestSecrets can be used instead.
func InitSecrets() {
	registrationSecret = []byte(os.Getenv("REGISTRATION_SECRET"))
	jwtSecret = []byte(os.Getenv("JWT_SECRET"))
	if len(registrationSecret) == 0 || len(jwtSecret) == 0 {
		panic("REGISTRATION_SECRET and JWT_SECRET must be set")
	}
}

// SetTestSecrets sets dummy secrets for unit tests.
func SetTestSecrets() {
	registrationSecret = []byte("test-registration-secret")
	jwtSecret = []byte("test-jwt-secret-that-is-long-enough")
}

func init() {
	// Auto-initialize from env vars if set; otherwise defer to explicit
	// InitSecrets (production) or SetTestSecrets (tests).
	if os.Getenv("REGISTRATION_SECRET") != "" && os.Getenv("JWT_SECRET") != "" {
		registrationSecret = []byte(os.Getenv("REGISTRATION_SECRET"))
		jwtSecret = []byte(os.Getenv("JWT_SECRET"))
	}
}

// GenerateRegistrationSignature signs the registration material. The nonce is
// generated server-side per setup script and can only be redeemed once (see
// db.ConsumeRegistrationNonce), so a leaked script cannot be replayed to mint
// tokens for arbitrary node IDs.
func GenerateRegistrationSignature(group, timestamp, nonce string) string {
	h := hmac.New(sha256.New, registrationSecret)
	_, _ = fmt.Fprintf(h, "%s:%s:%s", group, timestamp, nonce) // hash.Hash.Write never returns an error.
	return hex.EncodeToString(h.Sum(nil))
}

// NewRegistrationNonce returns a random single-use registration nonce.
func NewRegistrationNonce() string {
	return randomHex(16)
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b) // crypto/rand.Read never returns an error.
	return hex.EncodeToString(b)
}

func signAgentToken(macAddr, tokenType string, lifetime time.Duration) (string, error) {
	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": macAddr,
		"aud": TokenAudience,
		"typ": tokenType,
		"jti": randomHex(16),
		"iat": now.Unix(),
		"exp": now.Add(lifetime).Unix(),
	})
	return token.SignedString(jwtSecret)
}

func GenerateAgentJWT(macAddr string) (string, error) {
	return signAgentToken(macAddr, TokenTypeAccess, AccessTokenDuration)
}

func GenerateTokenPair(macAddr string) (*TokenPair, error) {
	accessToken, err := GenerateAgentJWT(macAddr)
	if err != nil {
		return nil, err
	}

	refreshStr, err := signAgentToken(macAddr, TokenTypeRefresh, RefreshTokenDuration)
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

// ValidateAccessToken validates an agent access token: signature, expiry,
// audience and token type.
func ValidateAccessToken(tokenStr string) (*jwt.MapClaims, error) {
	claims, err := ValidateToken(tokenStr)
	if err != nil {
		return nil, err
	}
	if (*claims)["typ"] != TokenTypeAccess || !hasAudience(*claims) {
		return nil, errors.New("not an access token")
	}
	return claims, nil
}

// ValidateRefreshToken validates an agent refresh token. Tokens issued before
// the typ/aud claims were introduced carry neither claim; they are still
// accepted here because refresh tokens are additionally checked against the
// hashed copy stored in the database, which only ever holds refresh tokens.
func ValidateRefreshToken(tokenStr string) (*jwt.MapClaims, error) {
	claims, err := ValidateToken(tokenStr)
	if err != nil {
		return nil, err
	}
	typ, hasType := (*claims)["typ"]
	if hasType && (typ != TokenTypeRefresh || !hasAudience(*claims)) {
		return nil, errors.New("not a refresh token")
	}
	return claims, nil
}

func hasAudience(claims jwt.MapClaims) bool {
	aud, err := claims.GetAudience()
	if err != nil {
		return false
	}
	for _, a := range aud {
		if a == TokenAudience {
			return true
		}
	}
	return false
}

func ValidateRegistration(group, timestamp, nonce, signature string) bool {
	// Always perform both checks to avoid timing leaks
	t, err := time.Parse(time.RFC3339, timestamp)
	timestampValid := err == nil && time.Since(t) <= 10*time.Minute && time.Until(t) <= 5*time.Minute

	expected := GenerateRegistrationSignature(group, timestamp, nonce)
	signatureValid := hmac.Equal([]byte(signature), []byte(expected))

	return timestampValid && signatureValid && nonce != ""
}
