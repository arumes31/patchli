package auth

import (
	"github.com/golang-jwt/jwt/v5"
	"testing"
	"time"
)

func TestSecretsInitialization(t *testing.T) {
	oldRegistration, oldJWT := registrationSecret, jwtSecret
	t.Cleanup(func() { registrationSecret, jwtSecret = oldRegistration, oldJWT })
	t.Setenv("REGISTRATION_SECRET", "registration-test-value")
	t.Setenv("JWT_SECRET", "jwt-test-value")
	InitSecrets()
	token, err := GenerateAgentJWT("node")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateToken(token); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JWT_SECRET", "")
	defer func() {
		if recover() == nil {
			t.Error("missing secret must fail startup")
		}
	}()
	InitSecrets()
}

func TestRejectsUntrustedTokens(t *testing.T) {
	cases := []struct {
		name   string
		method jwt.SigningMethod
		secret any
		claims jwt.MapClaims
	}{
		{"expired", jwt.SigningMethodHS256, jwtSecret, jwt.MapClaims{"sub": "node", "exp": time.Now().Add(-time.Hour).Unix()}},
		{"wrong key", jwt.SigningMethodHS256, []byte("attacker-key"), jwt.MapClaims{"sub": "node", "exp": time.Now().Add(time.Hour).Unix()}},
		{"unsigned", jwt.SigningMethodNone, jwt.UnsafeAllowNoneSignatureType, jwt.MapClaims{"sub": "node", "exp": time.Now().Add(time.Hour).Unix()}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			encoded, err := jwt.NewWithClaims(tc.method, tc.claims).SignedString(tc.secret)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ValidateToken(encoded); err == nil {
				t.Fatal("untrusted token accepted")
			}
		})
	}
	if _, err := ValidateToken("malformed"); err == nil {
		t.Fatal("malformed token accepted")
	}
}

func TestRegistrationRejectsFutureAndMalformedTimestamp(t *testing.T) {
	for _, stamp := range []string{"not-a-time", time.Now().Add(time.Hour).Format(time.RFC3339)} {
		if ValidateRegistration("group", stamp, "nonce", GenerateRegistrationSignature("group", stamp, "nonce")) {
			t.Fatalf("invalid time %q accepted", stamp)
		}
	}
}
