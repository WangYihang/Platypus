package storage_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/WangYihang/Platypus/internal/optoken"
	"github.com/WangYihang/Platypus/internal/storage"
)

func samplePAT(tokenID, userID string) *storage.PAT {
	now := time.Now().UTC()
	return &storage.PAT{
		TokenID:    tokenID,
		SecretHash: []byte("dummy-hash-32-bytes-padded-here-"),
		UserID:     userID,
		Name:       "test-pat",
		Scopes:     []string{optoken.ScopeHostsRead, optoken.ScopeFilesRead},
		CreatedAt:  now,
		ExpiresAt:  now.Add(90 * 24 * time.Hour),
	}
}

func TestCreatePAT_Roundtrip(t *testing.T) {
	t.Parallel()
	db := newAuthDB(t)
	makeUser(t, db, "u1")
	ctx := context.Background()

	p := samplePAT("pat_test1", "u1")
	p.Description = "for unit tests"
	if err := db.AuthTokens().CreatePAT(ctx, p); err != nil {
		t.Fatalf("CreatePAT: %v", err)
	}

	got, err := db.AuthTokens().GetPAT(ctx, p.TokenID)
	if err != nil {
		t.Fatalf("GetPAT: %v", err)
	}
	if got.TokenID != p.TokenID || got.UserID != p.UserID || got.Name != p.Name {
		t.Errorf("roundtrip mismatch: got %+v want %+v", got, p)
	}
	if len(got.Scopes) != 2 {
		t.Errorf("scopes len = %d, want 2 (got %v)", len(got.Scopes), got.Scopes)
	}
	if got.Description != "for unit tests" {
		t.Errorf("description = %q", got.Description)
	}
	if got.Revoked {
		t.Error("freshly-created PAT marked revoked")
	}
}

func TestCreatePAT_RejectsEmptyName(t *testing.T) {
	t.Parallel()
	db := newAuthDB(t)
	makeUser(t, db, "u1")
	ctx := context.Background()

	p := samplePAT("pat_noname", "u1")
	p.Name = ""
	if err := db.AuthTokens().CreatePAT(ctx, p); err == nil {
		t.Fatal("CreatePAT with empty Name = nil err, want validation error")
	}
}

func TestCreatePAT_RejectsEmptyScopes(t *testing.T) {
	t.Parallel()
	db := newAuthDB(t)
	makeUser(t, db, "u1")
	ctx := context.Background()

	p := samplePAT("pat_noscope", "u1")
	p.Scopes = nil
	if err := db.AuthTokens().CreatePAT(ctx, p); err == nil {
		t.Fatal("CreatePAT with no scopes = nil err, want validation error")
	}
}

func TestGetPAT_NotFound(t *testing.T) {
	t.Parallel()
	db := newAuthDB(t)
	ctx := context.Background()
	_, err := db.AuthTokens().GetPAT(ctx, "pat_nope")
	if !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestGetPAT_RejectsSessionRow(t *testing.T) {
	t.Parallel()
	db := newAuthDB(t)
	makeUser(t, db, "u1")
	ctx := context.Background()

	// Insert a session row, then try to fetch it as a PAT — kind
	// filter must hide it.
	s := sampleSession("pst_typecheck", "u1")
	if err := db.AuthTokens().CreateSession(ctx, s); err != nil {
		t.Fatal(err)
	}
	_, err := db.AuthTokens().GetPAT(ctx, s.TokenID)
	if !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("GetPAT on session row: err = %v, want ErrNotFound", err)
	}
}

func TestListPATsForUser(t *testing.T) {
	t.Parallel()
	db := newAuthDB(t)
	makeUser(t, db, "alice")
	makeUser(t, db, "bob")
	makeUser(t, db, "actor")
	ctx := context.Background()

	for _, id := range []string{"pat_a1", "pat_a2", "pat_a3"} {
		p := samplePAT(id, "alice")
		if err := db.AuthTokens().CreatePAT(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.AuthTokens().CreatePAT(ctx, samplePAT("pat_b1", "bob")); err != nil {
		t.Fatal(err)
	}
	// Revoke one of alice's so the includeRevoked filter has work.
	if err := db.AuthTokens().Revoke(ctx, "pat_a3", "actor", "leaked", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}

	active, err := db.AuthTokens().ListPATsForUser(ctx, "alice", false)
	if err != nil {
		t.Fatalf("ListPATsForUser: %v", err)
	}
	if len(active) != 2 {
		t.Errorf("active alice PATs = %d, want 2", len(active))
	}

	all, err := db.AuthTokens().ListPATsForUser(ctx, "alice", true)
	if err != nil {
		t.Fatalf("ListPATsForUser includeRevoked: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("all alice PATs = %d, want 3", len(all))
	}
	for _, a := range all {
		if a.UserID != "alice" {
			t.Errorf("creator filter leaked %q's PAT %q", a.UserID, a.TokenID)
		}
	}
}

func TestVerifyPAT_Success(t *testing.T) {
	t.Parallel()
	db := newAuthDB(t)
	makeUser(t, db, "u1")
	ctx := context.Background()

	_, secret, hash, _, err := optoken.Generate(optoken.PATPrefix)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	p := samplePAT("pat_verify", "u1")
	p.SecretHash = hash
	if err := db.AuthTokens().CreatePAT(ctx, p); err != nil {
		t.Fatalf("CreatePAT: %v", err)
	}

	v, reason, err := db.AuthTokens().Verify(ctx, p.TokenID, secret, optoken.KindPAT, time.Now().UTC())
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if reason != "success" || v == nil {
		t.Fatalf("reason=%q v=%+v", reason, v)
	}
	if v.Kind != optoken.KindPAT || v.UserID != "u1" {
		t.Errorf("Verified mismatch: %+v", v)
	}
	if !optoken.HasScope(v.Scopes, optoken.ScopeHostsRead) {
		t.Errorf("scopes missing hosts:read: %v", v.Scopes)
	}
}
