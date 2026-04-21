package user_test

import (
	"testing"

	"github.com/WangYihang/Platypus/internal/user"
)

// HashPassword must produce a hash that verifies against the same password
// and rejects a different one. bcrypt randomises salts per call so two
// hashes of the same password must differ byte-for-byte.
func TestHashPassword_RoundTrip(t *testing.T) {
	plain := "correct horse battery staple"

	h1, err := user.HashPassword(plain)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if h1 == "" {
		t.Fatal("HashPassword returned empty string")
	}
	if h1 == plain {
		t.Fatal("HashPassword returned the plaintext")
	}

	if !user.VerifyPassword(h1, plain) {
		t.Fatal("VerifyPassword rejected the correct password")
	}
	if user.VerifyPassword(h1, "wrong") {
		t.Fatal("VerifyPassword accepted a wrong password")
	}

	h2, err := user.HashPassword(plain)
	if err != nil {
		t.Fatalf("HashPassword #2: %v", err)
	}
	if h1 == h2 {
		t.Fatal("two bcrypt hashes of the same password were identical (salt missing?)")
	}
}

func TestHashPassword_EmptyRejected(t *testing.T) {
	if _, err := user.HashPassword(""); err == nil {
		t.Fatal("HashPassword(\"\") should reject empty password")
	}
}

// Roles are a closed set. ParseRole both normalises case and rejects
// unknown values so the DB CHECK constraint is never reached in practice.
func TestParseRole(t *testing.T) {
	for _, tc := range []struct {
		in      string
		want    user.Role
		wantErr bool
	}{
		{"admin", user.RoleAdmin, false},
		{"ADMIN", user.RoleAdmin, false},
		{"operator", user.RoleOperator, false},
		{"viewer", user.RoleViewer, false},
		{"", "", true},
		{"superuser", "", true},
	} {
		got, err := user.ParseRole(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ParseRole(%q) expected error, got %v", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseRole(%q) unexpected error: %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("ParseRole(%q) = %v; want %v", tc.in, got, tc.want)
		}
	}
}
