package auth

import (
	"os"
	"testing"
	"time"
)

func init() {
	os.Setenv("REGISTRATION_SECRET", "testsecret")
	os.Setenv("JWT_SECRET", "atleast16charslongsecret")
}

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
	resp, err := GenerateAgentJWT("00:11:22:33:44:55")
	if err != nil {
		t.Fatalf("Failed to generate JWT: %v", err)
	}
	if resp.AccessToken == "" {
		t.Error("Generated access token is empty")
	}
	if resp.RefreshToken == "" {
		t.Error("Generated refresh token is empty")
	}

	sub, err := ValidateRefreshToken(resp.RefreshToken)
	if err != nil {
		t.Fatalf("Failed to validate refresh token: %v", err)
	}
	if sub != "00:11:22:33:44:55" {
		t.Errorf("Expected sub 00:11:22:33:44:55, got %s", sub)
	}

	_, err = ValidateRefreshToken(resp.AccessToken)
	if err == nil {
		t.Error("AccessToken should not be valid as RefreshToken")
	}
}
