package api_test

import (
	"context"
	"testing"
	"time"

	"github.com/WangYihang/Platypus/internal/api"
	"github.com/WangYihang/Platypus/internal/optoken"
	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

func sessionVerifierSetup(t *testing.T) (*api.TokenVerifier, *storage.DB, string, *storage.UserSession, *user.User) {
	t.Helper()
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	u := &user.User{
		ID: "u-alice", Username: "alice", PasswordHash: "x",
		Role: user.RoleOperator, CreatedAt: time.Now().UTC(),
	}
	if err := db.Users().Create(context.Background(), u); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	id, _, hash, plaintext, err := optoken.Generate(optoken.UserSessionPrefix)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	s := &storage.UserSession{
		TokenID:       id,
		SecretHash:    hash,
		UserID:        u.ID,
		UserAgent:     "Mozilla/5.0",
		CreatedAt:     now,
		ExpiresAt:     now.Add(30 * 24 * time.Hour),
		IdleExpiresAt: now.Add(24 * time.Hour),
	}
	if err := db.AuthTokens().CreateSession(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	cache := optoken.NewCache(64, 30*time.Second)
	return api.NewTokenVerifier(db, cache), db, plaintext, s, u
}

func TestTokenVerifier_Session_Success_BecomesUserPrincipal(t *testing.T) {
	t.Parallel()
	v, _, plaintext, s, u := sessionVerifierSetup(t)
	p, reason, err := v.Verify(context.Background(), plaintext)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if reason != "success" {
		t.Errorf("reason = %q, want success", reason)
	}
	if p == nil {
		t.Fatal("principal nil")
	}
	if p.Kind != api.PrincipalUser {
		t.Errorf("Kind = %v, want PrincipalUser (sessions are humans)", p.Kind)
	}
	if p.UserID != u.ID {
		t.Errorf("UserID = %q, want %q", p.UserID, u.ID)
	}
	if p.Username != u.Username {
		t.Errorf("Username = %q, want %q (must be hydrated from users row)", p.Username, u.Username)
	}
	if p.Role != u.Role {
		t.Errorf("Role = %q, want %q (must be hydrated from users row, not session)", p.Role, u.Role)
	}
	if !optoken.HasScope(p.Scopes, optoken.ScopeHostsExec) {
		t.Errorf("operator scopes missing exec: %v", p.Scopes)
	}
	if p.TokenID != s.TokenID {
		t.Errorf("TokenID = %q, want %q (so audit can join 'this session did X')", p.TokenID, s.TokenID)
	}
}

func TestTokenVerifier_Session_RoleChangeReflectedQuickly(t *testing.T) {
	t.Parallel()
	v, db, plaintext, _, u := sessionVerifierSetup(t)

	// First Verify: operator role.
	p, _, _ := v.Verify(context.Background(), plaintext)
	if p.Role != user.RoleOperator {
		t.Fatalf("priming role = %q", p.Role)
	}

	// Demote the user. Cache TTL is 30s so the very next call may
	// still hit cache; an explicit invalidate (the production path
	// when an admin demotes a user) gives an immediate read.
	if err := db.Users().UpdateRole(context.Background(), u.ID, user.RoleViewer); err != nil {
		t.Fatal(err)
	}
	v.Invalidate(p.TokenID)

	p2, _, _ := v.Verify(context.Background(), plaintext)
	if p2.Role != user.RoleViewer {
		t.Errorf("post-demote role = %q, want viewer", p2.Role)
	}
	if optoken.HasScope(p2.Scopes, optoken.ScopeHostsExec) {
		t.Errorf("post-demote scopes still include exec: %v", p2.Scopes)
	}
}

func TestTokenVerifier_Session_TouchBumpsIdle(t *testing.T) {
	t.Parallel()
	v, db, plaintext, s, _ := sessionVerifierSetup(t)
	originalIdle := s.IdleExpiresAt

	if _, reason, _ := v.Verify(context.Background(), plaintext); reason != "success" {
		t.Fatal("priming failed")
	}
	// Touch bump runs detached; poll until the session's idle window
	// has moved forward (or fail after a short wait).
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		got, _ := db.AuthTokens().GetSession(context.Background(), s.TokenID)
		if got.IdleExpiresAt.After(originalIdle) {
			return // success — idle window advanced
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Error("idle_expires_at not advanced 2s after Verify success")
}

func TestTokenVerifier_Session_AfterRevokeAll_Fails(t *testing.T) {
	t.Parallel()
	v, db, plaintext, s, u := sessionVerifierSetup(t)
	if _, reason, _ := v.Verify(context.Background(), plaintext); reason != "success" {
		t.Fatal("priming failed")
	}
	// Mass-revoke (e.g. password change cascade).
	n, err := db.AuthTokens().RevokeAllSessionsForUser(context.Background(), u.ID, u.ID, "passwd change", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Fatal("RevokeAllSessions touched no rows")
	}
	// The auth handler explicitly invalidates each affected
	// session; the cache only bounds the unhappy path. Mirror that
	// here.
	v.Invalidate(s.TokenID)
	_, reason, _ := v.Verify(context.Background(), plaintext)
	if reason != "revoked" {
		t.Errorf("post-revoke-all reason = %q, want revoked", reason)
	}
}
