package storage_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

func TestAdminSettings_GetMissingReturnsNotFound(t *testing.T) {
	db := newTestDB(t)
	_, err := db.AdminSettings().Get(context.Background(), "nope")
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("Get missing = %v; want ErrNotFound", err)
	}
}

func TestAdminSettings_UpsertGet(t *testing.T) {
	db := newTestDB(t)
	admin := seedUser(t, db, "admin", user.RoleAdmin)

	now := time.Now().UTC().Truncate(time.Second)
	if err := db.AdminSettings().Upsert(context.Background(), "auth.access_token_ttl", "900", admin.ID, now); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	got, err := db.AdminSettings().Get(context.Background(), "auth.access_token_ttl")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Value != "900" || got.UpdatedBy != admin.ID {
		t.Fatalf("row = %+v", got)
	}
	if !got.UpdatedAt.Equal(now) {
		t.Fatalf("UpdatedAt = %v, want %v", got.UpdatedAt, now)
	}
}

func TestAdminSettings_UpsertOverwrites(t *testing.T) {
	db := newTestDB(t)
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	operator := seedUser(t, db, "op", user.RoleOperator)

	ctx := context.Background()
	if err := db.AdminSettings().Upsert(ctx, "k", "v1", admin.ID, time.Now().UTC()); err != nil {
		t.Fatalf("Upsert #1: %v", err)
	}
	later := time.Now().UTC().Add(time.Minute).Truncate(time.Second)
	if err := db.AdminSettings().Upsert(ctx, "k", "v2", operator.ID, later); err != nil {
		t.Fatalf("Upsert #2: %v", err)
	}

	got, err := db.AdminSettings().Get(ctx, "k")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Value != "v2" || got.UpdatedBy != operator.ID {
		t.Fatalf("overwrite failed: %+v", got)
	}
}

func TestAdminSettings_Delete(t *testing.T) {
	db := newTestDB(t)
	admin := seedUser(t, db, "admin", user.RoleAdmin)

	ctx := context.Background()
	if err := db.AdminSettings().Upsert(ctx, "k", "v", admin.ID, time.Now().UTC()); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if err := db.AdminSettings().Delete(ctx, "k"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := db.AdminSettings().Get(ctx, "k"); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("Get after delete = %v; want ErrNotFound", err)
	}

	// Deleting a non-existent key is a no-op (no error).
	if err := db.AdminSettings().Delete(ctx, "nope"); err != nil {
		t.Fatalf("Delete missing: %v", err)
	}
}

func TestAdminSettings_All(t *testing.T) {
	db := newTestDB(t)
	admin := seedUser(t, db, "admin", user.RoleAdmin)

	ctx := context.Background()
	now := time.Now().UTC()
	for _, k := range []string{"c", "a", "b"} {
		if err := db.AdminSettings().Upsert(ctx, k, "\"x\"", admin.ID, now); err != nil {
			t.Fatalf("Upsert %s: %v", k, err)
		}
	}

	rows, err := db.AdminSettings().All(ctx)
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("len(All) = %d, want 3", len(rows))
	}
	want := []string{"a", "b", "c"}
	for i, r := range rows {
		if r.Key != want[i] {
			t.Fatalf("rows[%d].Key = %q, want %q (not sorted?)", i, r.Key, want[i])
		}
	}
}
