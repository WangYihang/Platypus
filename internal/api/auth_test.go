package api

import (
	"testing"
)

func TestAuthCreateAndValidate(t *testing.T) {
	auth := NewAuth()
	token := auth.CreateToken()

	if !auth.ValidateToken(token) {
		t.Fatal("expected token to be valid")
	}
	if auth.ValidateToken("invalid-token") {
		t.Fatal("expected invalid token to fail")
	}
}

func TestAuthSecretCheck(t *testing.T) {
	auth := NewAuth()
	secret := auth.GetSecret()
	if secret == "" {
		t.Fatal("expected non-empty secret")
	}
	if len(secret) != 64 { // 32 bytes = 64 hex chars
		t.Fatalf("expected 64 char secret, got %d", len(secret))
	}
}
