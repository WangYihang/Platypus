package storage_test

import (
	"context"
	"testing"
	"time"

	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

func TestRefreshTokens_CreateAndGet(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	u := seedUser(t, db, "alice", user.RoleAdmin)

	rt := &storage.RefreshToken{
		ID:        "tok-1",
		UserID:    u.ID,
		ExpiresAt: time.Now().Add(24 * time.Hour).UTC(),
	}
	if err := db.RefreshTokens().Create(ctx, rt); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := db.RefreshTokens().Get(ctx, "tok-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.UserID != u.ID {
		t.Fatalf("UserID = %q; want %q", got.UserID, u.ID)
	}
	if got.RevokedAt != nil {
		t.Fatalf("RevokedAt = %v; want nil", got.RevokedAt)
	}
}

func TestRefreshTokens_Revoke(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	u := seedUser(t, db, "alice", user.RoleAdmin)

	rt := &storage.RefreshToken{
		ID:        "tok-1",
		UserID:    u.ID,
		ExpiresAt: time.Now().Add(24 * time.Hour).UTC(),
	}
	if err := db.RefreshTokens().Create(ctx, rt); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := db.RefreshTokens().Revoke(ctx, "tok-1"); err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	got, err := db.RefreshTokens().Get(ctx, "tok-1")
	if err != nil {
		t.Fatalf("Get after Revoke: %v", err)
	}
	if got.RevokedAt == nil {
		t.Fatal("RevokedAt still nil after Revoke")
	}
}

// RevokeAllForUser invalidates every refresh token owned by a user in one
// call — needed on password change to invalidate outstanding logins.
func TestRefreshTokens_RevokeAllForUser(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	u := seedUser(t, db, "alice", user.RoleAdmin)

	for _, id := range []string{"t1", "t2", "t3"} {
		_ = db.RefreshTokens().Create(ctx, &storage.RefreshToken{
			ID:        id,
			UserID:    u.ID,
			ExpiresAt: time.Now().Add(time.Hour).UTC(),
		})
	}
	if err := db.RefreshTokens().RevokeAllForUser(ctx, u.ID); err != nil {
		t.Fatalf("RevokeAllForUser: %v", err)
	}
	for _, id := range []string{"t1", "t2", "t3"} {
		got, _ := db.RefreshTokens().Get(ctx, id)
		if got.RevokedAt == nil {
			t.Errorf("token %q still live after RevokeAllForUser", id)
		}
	}
}

// Deleting the owning user cascades to refresh_tokens via the FK.
func TestRefreshTokens_CascadeOnUserDelete(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	u := seedUser(t, db, "alice", user.RoleAdmin)

	_ = db.RefreshTokens().Create(ctx, &storage.RefreshToken{
		ID:        "tok-1",
		UserID:    u.ID,
		ExpiresAt: time.Now().Add(time.Hour).UTC(),
	})

	if err := db.Users().Delete(ctx, u.ID); err != nil {
		t.Fatalf("Delete user: %v", err)
	}

	_, err := db.RefreshTokens().Get(ctx, "tok-1")
	if err != storage.ErrNotFound {
		t.Fatalf("expected ErrNotFound after cascade; got %v", err)
	}
}

func TestRefreshTokens_Get_NotFound(t *testing.T) {
	db := newTestDB(t)
	_, err := db.RefreshTokens().Get(context.Background(), "missing")
	if err != storage.ErrNotFound {
		t.Fatalf("err = %v; want ErrNotFound", err)
	}
}
