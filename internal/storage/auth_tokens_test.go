package storage_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/WangYihang/Platypus/internal/optoken"
	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
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

// makeUser inserts a row in users so AAT FKs are satisfiable.
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

func makeProject(t *testing.T, db *storage.DB, id string) {
	t.Helper()
	makeUser(t, db, "owner-"+id)
	_, err := db.Exec(`
		INSERT INTO projects (id, name, slug, created_by, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		id, id, id, "owner-"+id, time.Now().UTC(),
	)
	if err != nil {
		t.Fatalf("seed project %s: %v", id, err)
	}
}

func sampleAAT(tokenID, userID string) *storage.AAT {
	now := time.Now().UTC()
	return &storage.AAT{
		TokenID:    tokenID,
		SecretHash: []byte("dummy-hash-32-bytes-padded-here-"),
		UserID:     userID,
		Name:       "test-token",
		Role:       user.RoleOperator,
		Scopes:     []string{optoken.ScopeHostsRead, optoken.ScopeFilesRead},
		CreatedAt:  now,
		ExpiresAt:  now.Add(24 * time.Hour),
	}
}

func TestCreateAAT_Roundtrip(t *testing.T) {
	t.Parallel()
	db := newAuthDB(t)
	makeUser(t, db, "u1")
	ctx := context.Background()

	a := sampleAAT("aat_test1", "u1")
	a.Description = "for unit tests"
	if err := db.AuthTokens().CreateAAT(ctx, a); err != nil {
		t.Fatalf("CreateAAT: %v", err)
	}

	got, err := db.AuthTokens().GetAAT(ctx, a.TokenID)
	if err != nil {
		t.Fatalf("GetAAT: %v", err)
	}
	if got.TokenID != a.TokenID || got.UserID != a.UserID || got.Name != a.Name {
		t.Errorf("roundtrip mismatch: got %+v want %+v", got, a)
	}
	if got.Role != user.RoleOperator {
		t.Errorf("role = %q, want operator", got.Role)
	}
	if len(got.Scopes) != 2 {
		t.Errorf("scopes len = %d, want 2 (got %v)", len(got.Scopes), got.Scopes)
	}
	if got.Description != "for unit tests" {
		t.Errorf("description = %q", got.Description)
	}
	if got.Revoked {
		t.Error("freshly-created AAT marked revoked")
	}
}

func TestCreateAAT_GlobalAndProjectScoped(t *testing.T) {
	t.Parallel()
	db := newAuthDB(t)
	makeUser(t, db, "u1")
	makeProject(t, db, "p1")
	ctx := context.Background()

	global := sampleAAT("aat_global", "u1")
	if err := db.AuthTokens().CreateAAT(ctx, global); err != nil {
		t.Fatalf("global CreateAAT: %v", err)
	}
	scoped := sampleAAT("aat_scoped", "u1")
	scoped.ProjectID = "p1"
	if err := db.AuthTokens().CreateAAT(ctx, scoped); err != nil {
		t.Fatalf("scoped CreateAAT: %v", err)
	}

	g, _ := db.AuthTokens().GetAAT(ctx, "aat_global")
	if g.ProjectID != "" {
		t.Errorf("global ProjectID = %q, want empty", g.ProjectID)
	}
	s, _ := db.AuthTokens().GetAAT(ctx, "aat_scoped")
	if s.ProjectID != "p1" {
		t.Errorf("scoped ProjectID = %q, want p1", s.ProjectID)
	}
}

func TestCreateAAT_RejectsDuplicateID(t *testing.T) {
	t.Parallel()
	db := newAuthDB(t)
	makeUser(t, db, "u1")
	ctx := context.Background()

	a := sampleAAT("aat_dup", "u1")
	if err := db.AuthTokens().CreateAAT(ctx, a); err != nil {
		t.Fatalf("first CreateAAT: %v", err)
	}
	if err := db.AuthTokens().CreateAAT(ctx, a); err == nil {
		t.Error("second CreateAAT with same id = nil err, want PRIMARY KEY violation")
	}
}

func TestGetAAT_NotFound(t *testing.T) {
	t.Parallel()
	db := newAuthDB(t)
	ctx := context.Background()

	_, err := db.AuthTokens().GetAAT(ctx, "aat_nope")
	if !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestVerify_Success(t *testing.T) {
	t.Parallel()
	db := newAuthDB(t)
	makeUser(t, db, "u1")
	ctx := context.Background()

	_, secret, hash, _, err := optoken.Generate(optoken.AATPrefix)
	if err != nil {
		t.Fatalf("optoken.Generate: %v", err)
	}
	a := sampleAAT("aat_v1", "u1")
	a.SecretHash = hash
	a.Scopes = []string{optoken.ScopeHostsRead, optoken.ScopeHostsExec}
	if err := db.AuthTokens().CreateAAT(ctx, a); err != nil {
		t.Fatalf("CreateAAT: %v", err)
	}

	v, reason, err := db.AuthTokens().Verify(ctx, a.TokenID, secret, optoken.KindAAT, time.Now().UTC())
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if reason != "success" {
		t.Errorf("reason = %q, want success", reason)
	}
	if v == nil {
		t.Fatal("Verify returned nil Verified on success")
	}
	if v.TokenID != a.TokenID || v.Kind != optoken.KindAAT || v.UserID != "u1" {
		t.Errorf("Verified mismatch: %+v", v)
	}
	if v.Role != user.RoleOperator {
		t.Errorf("Role = %q, want operator", v.Role)
	}
	if !optoken.HasScope(v.Scopes, optoken.ScopeHostsRead) ||
		!optoken.HasScope(v.Scopes, optoken.ScopeHostsExec) {
		t.Errorf("scopes missing expected entries: %v", v.Scopes)
	}
}

func TestVerify_UnknownToken(t *testing.T) {
	t.Parallel()
	db := newAuthDB(t)
	ctx := context.Background()
	_, reason, err := db.AuthTokens().Verify(ctx, "aat_doesnotexist", []byte("anything"), optoken.KindAAT, time.Now().UTC())
	if err != nil {
		t.Fatalf("Verify on missing id returned err: %v", err)
	}
	if reason != "unknown" {
		t.Errorf("reason = %q, want unknown", reason)
	}
}

func TestVerify_WrongKind(t *testing.T) {
	t.Parallel()
	db := newAuthDB(t)
	makeUser(t, db, "u1")
	ctx := context.Background()
	_, secret, hash, _, _ := optoken.Generate(optoken.AATPrefix)
	a := sampleAAT("aat_wrongkind", "u1")
	a.SecretHash = hash
	if err := db.AuthTokens().CreateAAT(ctx, a); err != nil {
		t.Fatal(err)
	}
	// Verifying an AAT row as a user_session must miss — protects
	// against a verifier-dispatch bug ever conflating kinds.
	_, reason, err := db.AuthTokens().Verify(ctx, a.TokenID, secret, optoken.KindUserSession, time.Now().UTC())
	if err != nil {
		t.Fatalf("Verify err: %v", err)
	}
	if reason != "unknown" {
		t.Errorf("reason = %q, want unknown (kind filter must reject)", reason)
	}
}

func TestVerify_InvalidSecret(t *testing.T) {
	t.Parallel()
	db := newAuthDB(t)
	makeUser(t, db, "u1")
	ctx := context.Background()
	_, _, hash, _, _ := optoken.Generate(optoken.AATPrefix)
	a := sampleAAT("aat_badsecret", "u1")
	a.SecretHash = hash
	if err := db.AuthTokens().CreateAAT(ctx, a); err != nil {
		t.Fatal(err)
	}
	_, reason, err := db.AuthTokens().Verify(ctx, a.TokenID, []byte("not-the-secret"), optoken.KindAAT, time.Now().UTC())
	if err != nil {
		t.Fatalf("Verify err: %v", err)
	}
	if reason != "invalid_secret" {
		t.Errorf("reason = %q, want invalid_secret", reason)
	}
}

func TestVerify_Expired(t *testing.T) {
	t.Parallel()
	db := newAuthDB(t)
	makeUser(t, db, "u1")
	ctx := context.Background()
	_, secret, hash, _, _ := optoken.Generate(optoken.AATPrefix)
	a := sampleAAT("aat_expired", "u1")
	a.SecretHash = hash
	a.ExpiresAt = time.Now().UTC().Add(-1 * time.Minute)
	if err := db.AuthTokens().CreateAAT(ctx, a); err != nil {
		t.Fatal(err)
	}
	_, reason, err := db.AuthTokens().Verify(ctx, a.TokenID, secret, optoken.KindAAT, time.Now().UTC())
	if err != nil {
		t.Fatalf("Verify err: %v", err)
	}
	if reason != "expired" {
		t.Errorf("reason = %q, want expired", reason)
	}
}

func TestVerify_Revoked(t *testing.T) {
	t.Parallel()
	db := newAuthDB(t)
	makeUser(t, db, "actor")
	makeUser(t, db, "u1")
	ctx := context.Background()
	_, secret, hash, _, _ := optoken.Generate(optoken.AATPrefix)
	a := sampleAAT("aat_revoked", "u1")
	a.SecretHash = hash
	if err := db.AuthTokens().CreateAAT(ctx, a); err != nil {
		t.Fatal(err)
	}
	if err := db.AuthTokens().Revoke(ctx, a.TokenID, "actor", "leaked", time.Now().UTC()); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	_, reason, err := db.AuthTokens().Verify(ctx, a.TokenID, secret, optoken.KindAAT, time.Now().UTC())
	if err != nil {
		t.Fatalf("Verify err: %v", err)
	}
	if reason != "revoked" {
		t.Errorf("reason = %q, want revoked", reason)
	}

	got, _ := db.AuthTokens().GetAAT(ctx, a.TokenID)
	if !got.Revoked || got.RevokedAt == nil {
		t.Errorf("revoked metadata not set: revoked=%v at=%v", got.Revoked, got.RevokedAt)
	}
	if got.RevokedByUser != "actor" || got.RevokedReason != "leaked" {
		t.Errorf("revoked actor/reason mismatch: by=%q reason=%q", got.RevokedByUser, got.RevokedReason)
	}
}

func TestRevoke_Idempotent(t *testing.T) {
	t.Parallel()
	db := newAuthDB(t)
	makeUser(t, db, "actor")
	makeUser(t, db, "u1")
	ctx := context.Background()
	a := sampleAAT("aat_idem", "u1")
	if err := db.AuthTokens().CreateAAT(ctx, a); err != nil {
		t.Fatal(err)
	}
	if err := db.AuthTokens().Revoke(ctx, a.TokenID, "actor", "first", time.Now().UTC()); err != nil {
		t.Fatalf("first Revoke: %v", err)
	}
	// Second revoke is a no-op, never an error.
	if err := db.AuthTokens().Revoke(ctx, a.TokenID, "actor", "second", time.Now().UTC()); err != nil {
		t.Fatalf("second Revoke must be idempotent, got: %v", err)
	}
	got, _ := db.AuthTokens().GetAAT(ctx, a.TokenID)
	if got.RevokedReason != "first" {
		t.Errorf("second Revoke clobbered reason: %q, want \"first\"", got.RevokedReason)
	}
}

func TestRevoke_Unknown(t *testing.T) {
	t.Parallel()
	db := newAuthDB(t)
	ctx := context.Background()
	err := db.AuthTokens().Revoke(ctx, "aat_nope", "actor", "x", time.Now().UTC())
	if !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestTouchLastUsed(t *testing.T) {
	t.Parallel()
	db := newAuthDB(t)
	makeUser(t, db, "u1")
	ctx := context.Background()
	a := sampleAAT("aat_touch", "u1")
	if err := db.AuthTokens().CreateAAT(ctx, a); err != nil {
		t.Fatal(err)
	}
	when := time.Now().UTC().Truncate(time.Second)
	if err := db.AuthTokens().TouchLastUsed(ctx, a.TokenID, "10.0.0.1", "ua/1.0", nil, when); err != nil {
		t.Fatalf("TouchLastUsed: %v", err)
	}
	got, _ := db.AuthTokens().GetAAT(ctx, a.TokenID)
	if got.LastUsedAt == nil || !got.LastUsedAt.Equal(when) {
		t.Errorf("LastUsedAt not set; got %v want %v", got.LastUsedAt, when)
	}
	if got.LastUsedIP != "10.0.0.1" {
		t.Errorf("LastUsedIP = %q", got.LastUsedIP)
	}
}

func TestListAATsByCreator(t *testing.T) {
	t.Parallel()
	db := newAuthDB(t)
	makeUser(t, db, "alice")
	makeUser(t, db, "bob")
	makeUser(t, db, "actor")
	ctx := context.Background()

	for _, id := range []string{"aat_a1", "aat_a2", "aat_a3"} {
		a := sampleAAT(id, "alice")
		if err := db.AuthTokens().CreateAAT(ctx, a); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.AuthTokens().CreateAAT(ctx, sampleAAT("aat_b1", "bob")); err != nil {
		t.Fatal(err)
	}
	// Revoke one of alice's so we can test the includeRevoked filter.
	if err := db.AuthTokens().Revoke(ctx, "aat_a3", "actor", "x", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}

	active, err := db.AuthTokens().ListAATsByCreator(ctx, "alice", false)
	if err != nil {
		t.Fatalf("ListAATsByCreator: %v", err)
	}
	if len(active) != 2 {
		t.Errorf("active alice AATs = %d, want 2", len(active))
	}

	all, err := db.AuthTokens().ListAATsByCreator(ctx, "alice", true)
	if err != nil {
		t.Fatalf("ListAATsByCreator includeRevoked: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("all alice AATs = %d, want 3", len(all))
	}
	for _, a := range all {
		if a.UserID != "alice" {
			t.Errorf("creator filter leaked %q's AAT %q", a.UserID, a.TokenID)
		}
	}
}

func TestListAATsByProject(t *testing.T) {
	t.Parallel()
	db := newAuthDB(t)
	makeUser(t, db, "u1")
	makeProject(t, db, "p1")
	makeProject(t, db, "p2")
	ctx := context.Background()

	in := func(id, project string) {
		a := sampleAAT(id, "u1")
		a.ProjectID = project
		if err := db.AuthTokens().CreateAAT(ctx, a); err != nil {
			t.Fatal(err)
		}
	}
	in("aat_p1a", "p1")
	in("aat_p1b", "p1")
	in("aat_p2a", "p2")
	// Plus a global AAT — must not show up on either project list.
	if err := db.AuthTokens().CreateAAT(ctx, sampleAAT("aat_global", "u1")); err != nil {
		t.Fatal(err)
	}

	got, err := db.AuthTokens().ListAATsByProject(ctx, "p1", false)
	if err != nil {
		t.Fatalf("ListAATsByProject: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("p1 AATs = %d, want 2", len(got))
	}
	for _, a := range got {
		if a.ProjectID != "p1" {
			t.Errorf("project filter leaked %q (project=%q)", a.TokenID, a.ProjectID)
		}
	}
}
