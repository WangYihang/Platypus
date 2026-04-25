package storage_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/WangYihang/Platypus/internal/optoken"
	"github.com/WangYihang/Platypus/internal/storage"
)

func sampleSession(tokenID, userID string) *storage.UserSession {
	now := time.Now().UTC()
	return &storage.UserSession{
		TokenID:       tokenID,
		SecretHash:    []byte("dummy-hash-32-bytes-padded-here-"),
		UserID:        userID,
		CreatedAt:     now,
		ExpiresAt:     now.Add(30 * 24 * time.Hour),
		IdleExpiresAt: now.Add(24 * time.Hour),
		UserAgent:     "Mozilla/5.0 test",
	}
}

func TestCreateSession_Roundtrip(t *testing.T) {
	t.Parallel()
	db := newAuthDB(t)
	makeUser(t, db, "u1")
	ctx := context.Background()

	s := sampleSession("pst_test1", "u1")
	if err := db.AuthTokens().CreateSession(ctx, s); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	got, err := db.AuthTokens().GetSession(ctx, s.TokenID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got.TokenID != s.TokenID || got.UserID != s.UserID {
		t.Errorf("identity mismatch: %+v", got)
	}
	if got.UserAgent != "Mozilla/5.0 test" {
		t.Errorf("UserAgent = %q", got.UserAgent)
	}
	if got.IdleExpiresAt.IsZero() {
		t.Error("IdleExpiresAt zero on freshly-created session")
	}
}

func TestCreateSession_RejectsAATKind(t *testing.T) {
	t.Parallel()
	db := newAuthDB(t)
	makeUser(t, db, "u1")
	ctx := context.Background()
	// GetAAT must NOT find a session row — type discipline check.
	s := sampleSession("pst_typetest", "u1")
	if err := db.AuthTokens().CreateSession(ctx, s); err != nil {
		t.Fatal(err)
	}
	_, err := db.AuthTokens().GetAAT(ctx, s.TokenID)
	if !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("GetAAT on session row: err = %v, want ErrNotFound", err)
	}
}

func TestVerify_UserSession_Success(t *testing.T) {
	t.Parallel()
	db := newAuthDB(t)
	makeUser(t, db, "u1")
	ctx := context.Background()

	_, secret, hash, _, err := optoken.Generate(optoken.UserSessionPrefix)
	if err != nil {
		t.Fatal(err)
	}
	s := sampleSession("pst_v1", "u1")
	s.SecretHash = hash
	if err := db.AuthTokens().CreateSession(ctx, s); err != nil {
		t.Fatal(err)
	}
	v, reason, err := db.AuthTokens().Verify(ctx, s.TokenID, secret, optoken.KindUserSession, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if reason != "success" {
		t.Errorf("reason = %q, want success", reason)
	}
	if v == nil || v.Kind != optoken.KindUserSession || v.UserID != "u1" {
		t.Errorf("Verified mismatch: %+v", v)
	}
	if v.IdleExpiresAt.IsZero() {
		t.Error("IdleExpiresAt zero on session Verified")
	}
}

func TestVerify_UserSession_IdleExpired(t *testing.T) {
	t.Parallel()
	db := newAuthDB(t)
	makeUser(t, db, "u1")
	ctx := context.Background()
	_, secret, hash, _, _ := optoken.Generate(optoken.UserSessionPrefix)
	s := sampleSession("pst_idle", "u1")
	s.SecretHash = hash
	s.IdleExpiresAt = time.Now().UTC().Add(-time.Minute) // already past idle
	if err := db.AuthTokens().CreateSession(ctx, s); err != nil {
		t.Fatal(err)
	}
	_, reason, err := db.AuthTokens().Verify(ctx, s.TokenID, secret, optoken.KindUserSession, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if reason != "idle_expired" {
		t.Errorf("reason = %q, want idle_expired", reason)
	}
}

func TestTouchLastUsed_BumpsIdle(t *testing.T) {
	t.Parallel()
	db := newAuthDB(t)
	makeUser(t, db, "u1")
	ctx := context.Background()
	s := sampleSession("pst_bump", "u1")
	if err := db.AuthTokens().CreateSession(ctx, s); err != nil {
		t.Fatal(err)
	}
	newIdle := time.Now().UTC().Add(48 * time.Hour).Truncate(time.Second)
	when := time.Now().UTC().Truncate(time.Second)
	if err := db.AuthTokens().TouchLastUsed(ctx, s.TokenID, "10.0.0.1", "agent/1", &newIdle, when); err != nil {
		t.Fatal(err)
	}
	got, err := db.AuthTokens().GetSession(ctx, s.TokenID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.IdleExpiresAt.Equal(newIdle) {
		t.Errorf("IdleExpiresAt = %v, want %v", got.IdleExpiresAt, newIdle)
	}
	if got.LastUsedAt == nil || !got.LastUsedAt.Equal(when) {
		t.Errorf("LastUsedAt mismatch: %v", got.LastUsedAt)
	}
}

func TestListSessionsForUser(t *testing.T) {
	t.Parallel()
	db := newAuthDB(t)
	makeUser(t, db, "alice")
	makeUser(t, db, "bob")
	makeUser(t, db, "actor")
	ctx := context.Background()

	for _, id := range []string{"pst_a1", "pst_a2", "pst_a3"} {
		if err := db.AuthTokens().CreateSession(ctx, sampleSession(id, "alice")); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.AuthTokens().CreateSession(ctx, sampleSession("pst_b1", "bob")); err != nil {
		t.Fatal(err)
	}
	// Revoke one of alice's. ListSessionsForUser returns only active.
	if err := db.AuthTokens().Revoke(ctx, "pst_a3", "actor", "logout", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	got, err := db.AuthTokens().ListSessionsForUser(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Errorf("len = %d, want 2 (active alice sessions)", len(got))
	}
	for _, s := range got {
		if s.UserID != "alice" {
			t.Errorf("leaked session for %q", s.UserID)
		}
		if s.Revoked {
			t.Error("ListSessionsForUser returned revoked row")
		}
	}
}

func TestRevokeAllSessionsForUser(t *testing.T) {
	t.Parallel()
	db := newAuthDB(t)
	makeUser(t, db, "alice")
	makeUser(t, db, "bob")
	ctx := context.Background()

	if err := db.AuthTokens().CreateSession(ctx, sampleSession("pst_a1", "alice")); err != nil {
		t.Fatal(err)
	}
	if err := db.AuthTokens().CreateSession(ctx, sampleSession("pst_a2", "alice")); err != nil {
		t.Fatal(err)
	}
	if err := db.AuthTokens().CreateSession(ctx, sampleSession("pst_b1", "bob")); err != nil {
		t.Fatal(err)
	}
	// Also create an AAT for alice — must NOT be revoked.
	a := sampleAAT("aat_alice", "alice")
	if err := db.AuthTokens().CreateAAT(ctx, a); err != nil {
		t.Fatal(err)
	}

	n, err := db.AuthTokens().RevokeAllSessionsForUser(ctx, "alice", "alice", "password change", time.Now().UTC())
	if err != nil {
		t.Fatalf("RevokeAllSessionsForUser: %v", err)
	}
	if n != 2 {
		t.Errorf("revoked count = %d, want 2", n)
	}
	// Bob's session untouched.
	bobSessions, _ := db.AuthTokens().ListSessionsForUser(ctx, "bob")
	if len(bobSessions) != 1 {
		t.Errorf("bob's sessions affected: len=%d, want 1", len(bobSessions))
	}
	// Alice's AAT untouched.
	aliceAAT, _ := db.AuthTokens().GetAAT(ctx, "aat_alice")
	if aliceAAT.Revoked {
		t.Error("RevokeAllSessions also touched AATs (kind filter missing?)")
	}
}
