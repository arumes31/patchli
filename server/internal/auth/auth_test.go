package auth

import (
	"testing"
	"time"
)

func TestGenerateRegistrationSignature(t *testing.T) {
	sig1 := GenerateRegistrationSignature("default", "2024-01-01T00:00:00Z")
	sig2 := GenerateRegistrationSignature("default", "2024-01-01T00:00:00Z")
	if sig1 != sig2 {
		t.Error("Signatures for same input should be identical")
	}

	sig3 := GenerateRegistrationSignature("other", "2024-01-01T00:00:00Z")
	if sig1 == sig3 {
		t.Error("Signatures for different groups should be different")
	}
}

func TestValidateRegistration(t *testing.T) {
	now := time.Now().Format(time.RFC3339)
	sig := GenerateRegistrationSignature("test", now)

	if !ValidateRegistration("test", now, sig) {
		t.Error("Valid registration failed validation")
	}

	if ValidateRegistration("test", now, "wrong-signature") {
		t.Error("Invalid signature should fail validation")
	}

	if ValidateRegistration("wrong-group", now, sig) {
		t.Error("Wrong group should fail validation")
	}

	oldTime := time.Now().Add(-20 * time.Minute).Format(time.RFC3339)
	oldSig := GenerateRegistrationSignature("test", oldTime)
	if ValidateRegistration("test", oldTime, oldSig) {
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
