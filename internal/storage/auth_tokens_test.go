package storage_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/WangYihang/Platypus/internal/optoken"
	"github.com/WangYihang/Platypus/internal/storage"
)

func newAuthDB(t *testing.T) *storage.DB {
	t.Helper()
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// makeUser inserts a row in users so other tables' user FKs are satisfiable.
func makeUser(t *testing.T, db *storage.DB, id string) {
	t.Helper()
	_, err := db.Exec(`
		INSERT INTO users (id, username, password_hash, role, created_at)
		VALUES (?, ?, 'x', 'admin', ?)`,
		id, id, time.Now().UTC(),
	)
	if err != nil {
		t.Fatalf("seed user %s: %v", id, err)
	}
}

// The kind-agnostic auth_tokens behaviour (Verify dispatch, Revoke
// idempotency, TouchLastUsed) is exercised against the user-session
// shape because that's the only kind currently stored in this table.
// AAT was retired in migration 17.

func TestVerify_UnknownToken(t *testing.T) {
	t.Parallel()
	db := newAuthDB(t)
	ctx := context.Background()
	_, reason, err := db.AuthTokens().Verify(ctx, "pst_doesnotexist", []byte("anything"), optoken.KindUserSession, time.Now().UTC())
	if err != nil {
		t.Fatalf("Verify on missing id returned err: %v", err)
	}
	if reason != "unknown" {
		t.Errorf("reason = %q, want unknown", reason)
	}
}

func TestRevoke_Unknown(t *testing.T) {
	t.Parallel()
	db := newAuthDB(t)
	ctx := context.Background()
	err := db.AuthTokens().Revoke(ctx, "pst_nope", "actor", "x", time.Now().UTC())
	if !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestRevoke_Idempotent(t *testing.T) {
	t.Parallel()
	db := newAuthDB(t)
	makeUser(t, db, "actor")
	makeUser(t, db, "u1")
	ctx := context.Background()

	s := sampleSession("pst_idem", "u1")
	if err := db.AuthTokens().CreateSession(ctx, s); err != nil {
		t.Fatal(err)
	}
	if err := db.AuthTokens().Revoke(ctx, s.TokenID, "actor", "first", time.Now().UTC()); err != nil {
		t.Fatalf("first Revoke: %v", err)
	}
	// Second revoke is a no-op, never an error.
	if err := db.AuthTokens().Revoke(ctx, s.TokenID, "actor", "second", time.Now().UTC()); err != nil {
		t.Fatalf("second Revoke must be idempotent, got: %v", err)
	}
	// Reason from the first call must survive.
	got, _ := db.AuthTokens().GetSession(ctx, s.TokenID)
	if got.RevokedReason != "first" {
		t.Errorf("second Revoke clobbered reason: %q, want \"first\"", got.RevokedReason)
	}
}
