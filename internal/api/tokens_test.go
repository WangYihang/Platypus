package api

import (
	"strings"
	"testing"
	"time"

	"github.com/WangYihang/Platypus/internal/user"
)

func testIssuer(t *testing.T) *TokenIssuer {
	t.Helper()
	// Tests use tiny non-empty secrets; the issuer doesn't care about length,
	// only that the same key is used for sign and verify.
	issuer, err := NewTokenIssuer("access-secret", "refresh-secret",
		5*time.Minute, 24*time.Hour)
	if err != nil {
		t.Fatalf("NewTokenIssuer: %v", err)
	}
	return issuer
}

func TestTokenIssuer_AccessRoundTrip(t *testing.T) {
	issuer := testIssuer(t)

	tok, err := issuer.IssueAccess(AccessClaims{
		UserID:   "u1",
		Username: "alice",
		Role:     user.RoleAdmin,
	})
	if err != nil {
		t.Fatalf("IssueAccess: %v", err)
	}
	if !strings.Contains(tok, ".") {
		t.Fatalf("access token is not a JWT: %q", tok)
	}

	got, err := issuer.ParseAccess(tok)
	if err != nil {
		t.Fatalf("ParseAccess: %v", err)
	}
	if got.UserID != "u1" || got.Username != "alice" || got.Role != user.RoleAdmin {
		t.Fatalf("claims roundtrip mismatch: %+v", got)
	}
}

// An access token must not verify against the refresh key and vice versa —
// otherwise a stolen refresh token could impersonate a logged-in user.
func TestTokenIssuer_KeysAreSegregated(t *testing.T) {
	issuer := testIssuer(t)

	access, _ := issuer.IssueAccess(AccessClaims{UserID: "u1", Role: user.RoleAdmin})
	refresh, _ := issuer.IssueRefresh(RefreshClaims{UserID: "u1", TokenID: "tk1"})

	if _, err := issuer.ParseAccess(refresh); err == nil {
		t.Fatal("ParseAccess accepted a refresh token")
	}
	if _, err := issuer.ParseRefresh(access); err == nil {
		t.Fatal("ParseRefresh accepted an access token")
	}
}

func TestTokenIssuer_ExpiredAccess(t *testing.T) {
	// TTL of 0 means the token is already past expiry the moment it's issued;
	// Parse should reject it.
	issuer, err := NewTokenIssuer("a", "b", 0, time.Hour)
	if err != nil {
		t.Fatalf("NewTokenIssuer: %v", err)
	}
	tok, _ := issuer.IssueAccess(AccessClaims{UserID: "u1", Role: user.RoleAdmin})

	if _, err := issuer.ParseAccess(tok); err == nil {
		t.Fatal("expected ParseAccess to reject an expired token")
	}
}

func TestTokenIssuer_RejectsTampered(t *testing.T) {
	issuer := testIssuer(t)
	tok, _ := issuer.IssueAccess(AccessClaims{UserID: "u1", Role: user.RoleAdmin})

	// Flip the last character of the signature; the altered token must fail.
	bad := tok[:len(tok)-1] + string(tok[len(tok)-1]^0x01)
	if _, err := issuer.ParseAccess(bad); err == nil {
		t.Fatal("expected ParseAccess to reject a tampered token")
	}
}

func TestNewTokenIssuer_EmptyKeyRejected(t *testing.T) {
	if _, err := NewTokenIssuer("", "refresh", time.Minute, time.Hour); err == nil {
		t.Fatal("expected empty access key to be rejected")
	}
	if _, err := NewTokenIssuer("access", "", time.Minute, time.Hour); err == nil {
		t.Fatal("expected empty refresh key to be rejected")
	}
}
