package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestMain(m *testing.M) {
	SetTestSecrets()
	m.Run()
}

func TestGenerateRegistrationSignature(t *testing.T) {
	sig1 := GenerateRegistrationSignature("default", "2024-01-01T00:00:00Z", "n1")
	sig2 := GenerateRegistrationSignature("default", "2024-01-01T00:00:00Z", "n1")
	if sig1 != sig2 {
		t.Error("Signatures for same input should be identical")
	}

	sig3 := GenerateRegistrationSignature("other", "2024-01-01T00:00:00Z", "n1")
	if sig1 == sig3 {
		t.Error("Signatures for different groups should be different")
	}

	sig4 := GenerateRegistrationSignature("default", "2024-01-01T00:00:00Z", "n2")
	if sig1 == sig4 {
		t.Error("Signatures for different nonces should be different")
	}
}

func TestValidateRegistration(t *testing.T) {
	now := time.Now().Format(time.RFC3339)
	sig := GenerateRegistrationSignature("test", now, "nonce")

	if !ValidateRegistration("test", now, "nonce", sig) {
		t.Error("Valid registration failed validation")
	}

	if ValidateRegistration("test", now, "nonce", "wrong-signature") {
		t.Error("Invalid signature should fail validation")
	}

	if ValidateRegistration("wrong-group", now, "nonce", sig) {
		t.Error("Wrong group should fail validation")
	}

	if ValidateRegistration("test", now, "other-nonce", sig) {
		t.Error("Wrong nonce should fail validation")
	}

	emptyNonceSig := GenerateRegistrationSignature("test", now, "")
	if ValidateRegistration("test", now, "", emptyNonceSig) {
		t.Error("Empty nonce should fail validation")
	}

	oldTime := time.Now().Add(-20 * time.Minute).Format(time.RFC3339)
	oldSig := GenerateRegistrationSignature("test", oldTime, "nonce")
	if ValidateRegistration("test", oldTime, "nonce", oldSig) {
		t.Error("Expired timestamp should fail validation")
	}
}

func TestGenerateAgentJWT(t *testing.T) {
	token, err := GenerateAgentJWT("00:11:22:33:44:55")
	if err != nil {
		t.Fatalf("Failed to generate JWT: %v", err)
	}
	if token == "" {
		t.Error("Generated token is empty")
	}

	claims, err := ValidateToken(token)
	if err != nil {
		t.Fatalf("Failed to validate token: %v", err)
	}
	if (*claims)["sub"] != "00:11:22:33:44:55" {
		t.Errorf("Expected sub claim to be 00:11:22:33:44:55, got %v", (*claims)["sub"])
	}
}

func TestGenerateTokenPair(t *testing.T) {
	mac := "AA:BB:CC:DD:EE:FF"
	pair, err := GenerateTokenPair(mac)
	if err != nil {
		t.Fatalf("Failed to generate token pair: %v", err)
	}

	if pair.AccessToken == "" || pair.RefreshToken == "" {
		t.Error("Generated tokens should not be empty")
	}

	accessClaims, err := ValidateToken(pair.AccessToken)
	if err != nil {
		t.Fatalf("Failed to validate access token: %v", err)
	}
	if (*accessClaims)["sub"] != mac {
		t.Errorf("Expected sub claim to be %s, got %v", mac, (*accessClaims)["sub"])
	}

	refreshClaims, err := ValidateToken(pair.RefreshToken)
	if err != nil {
		t.Fatalf("Failed to validate refresh token: %v", err)
	}
	if (*refreshClaims)["sub"] != mac {
		t.Errorf("Expected sub claim to be %s, got %v", mac, (*refreshClaims)["sub"])
	}
}

func TestTokenTypesAreEnforced(t *testing.T) {
	pair, err := GenerateTokenPair("node")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateAccessToken(pair.AccessToken); err != nil {
		t.Fatalf("access token rejected: %v", err)
	}
	if _, err := ValidateAccessToken(pair.RefreshToken); err == nil {
		t.Fatal("refresh token accepted as access token")
	}
	if _, err := ValidateRefreshToken(pair.RefreshToken); err != nil {
		t.Fatalf("refresh token rejected: %v", err)
	}
	if _, err := ValidateRefreshToken(pair.AccessToken); err == nil {
		t.Fatal("access token accepted as refresh token")
	}

	second, err := GenerateTokenPair("node")
	if err != nil {
		t.Fatal(err)
	}
	if second.RefreshToken == pair.RefreshToken {
		t.Fatal("refresh tokens must be unique")
	}
}

func TestLegacyUntypedTokens(t *testing.T) {
	legacy, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "node", "iat": time.Now().Unix(), "exp": time.Now().Add(time.Hour).Unix(),
	}).SignedString(jwtSecret)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateAccessToken(legacy); err == nil {
		t.Fatal("untyped token accepted as access token")
	}
	if _, err := ValidateRefreshToken(legacy); err != nil {
		t.Fatalf("legacy refresh token rejected: %v", err)
	}
}
