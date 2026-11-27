package main

import "testing"

func TestStartupRejectsMissingAuthenticationSecrets(t *testing.T) {
	t.Setenv("REGISTRATION_SECRET", "")
	t.Setenv("JWT_SECRET", "")
	defer func() {
		if got := recover(); got != "REGISTRATION_SECRET and JWT_SECRET must be set" {
			t.Fatalf("startup should reject missing secrets before serving requests; panic = %v", got)
		}
	}()
	main()
}
