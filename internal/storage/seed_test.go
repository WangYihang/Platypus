package storage_test

import (
	"context"
	"errors"
	"testing"

	"github.com/WangYihang/Platypus/internal/storage"
)

// EnsureSystemUser creates the row on first call and returns it on
// subsequent calls without error or duplicate inserts. Idempotency is
// the contract the server main relies on across restarts.
func TestEnsureSystemUser_Idempotent(t *testing.T) {
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	ctx := context.Background()

	id, err := storage.EnsureSystemUser(ctx, db)
	if err != nil {
		t.Fatalf("EnsureSystemUser (first): %v", err)
	}
	if id != storage.SystemUserID {
		t.Fatalf("id = %q; want %q", id, storage.SystemUserID)
	}

	u, err := db.Users().GetByID(ctx, storage.SystemUserID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if u.PasswordHash != "" {
		t.Fatalf("system user password_hash should be empty so the account is unreachable via login; got %q", u.PasswordHash)
	}

	// Second call: must not duplicate-insert (users.username UNIQUE)
	// and must return the same id.
	if _, err := storage.EnsureSystemUser(ctx, db); err != nil {
		t.Fatalf("EnsureSystemUser (second): %v", err)
	}
}

// EnsureDefaultProject writes a row whose id is the literal
// DefaultProjectID so cfg.Mesh.ProjectID = "default" resolves to it,
// and whose created_by FK points at the system user.
func TestEnsureDefaultProject_FKAndIdempotent(t *testing.T) {
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	ctx := context.Background()

	// projects.created_by FKs to users(id) — without the system user
	// the Create must fail.
	if _, err := storage.EnsureDefaultProject(ctx, db, "nonexistent"); err == nil {
		t.Fatalf("want FK error when creator user doesn't exist")
	}

	sysID, err := storage.EnsureSystemUser(ctx, db)
	if err != nil {
		t.Fatalf("EnsureSystemUser: %v", err)
	}

	p1, err := storage.EnsureDefaultProject(ctx, db, sysID)
	if err != nil {
		t.Fatalf("EnsureDefaultProject (first): %v", err)
	}
	if p1.ID != storage.DefaultProjectID {
		t.Fatalf("project id = %q; want %q", p1.ID, storage.DefaultProjectID)
	}
	if p1.Slug != "default" {
		t.Fatalf("project slug = %q; want %q", p1.Slug, "default")
	}

	// Second call returns the existing row (no UNIQUE violation on
	// slug) and preserves created_at.
	p2, err := storage.EnsureDefaultProject(ctx, db, sysID)
	if err != nil {
		t.Fatalf("EnsureDefaultProject (second): %v", err)
	}
	if !p1.CreatedAt.Equal(p2.CreatedAt) {
		t.Fatalf("created_at changed between calls: %v vs %v", p1.CreatedAt, p2.CreatedAt)
	}
}

// Sanity check: ErrNotFound is what GetByID returns for a missing row,
// which EnsureSystemUser/EnsureDefaultProject rely on to distinguish
// "create it" from "real error" branches.
func TestGetByID_NotFoundSentinel(t *testing.T) {
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	_, err = db.Users().GetByID(context.Background(), "nope")
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("err = %v; want ErrNotFound", err)
	}
}
