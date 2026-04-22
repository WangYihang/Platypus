package storage_test

import (
	"context"
	"testing"
	"time"

	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

func newTestDB(t *testing.T) *storage.DB {
	t.Helper()
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// newTestDBFile opens a temp-file SQLite database. Required for tests
// that exercise multiple connections concurrently — the modernc.org
// driver's ":memory:" gives each pool connection its own database, so
// concurrent writes see "no such table" on the unmigrated siblings.
func newTestDBFile(t *testing.T) *storage.DB {
	t.Helper()
	dir := t.TempDir()
	db, err := storage.Open(dir + "/test.db")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func seedUser(t *testing.T, db *storage.DB, username string, role user.Role) *user.User {
	t.Helper()
	u := &user.User{
		ID:           "user-" + username,
		Username:     username,
		PasswordHash: "fake-hash",
		Role:         role,
		CreatedAt:    time.Now().UTC(),
	}
	if err := db.Users().Create(context.Background(), u); err != nil {
		t.Fatalf("Users().Create(%q): %v", username, err)
	}
	return u
}

func TestUserRepo_CreateAndGet(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	seedUser(t, db, "alice", user.RoleAdmin)

	got, err := db.Users().GetByUsername(ctx, "alice")
	if err != nil {
		t.Fatalf("GetByUsername: %v", err)
	}
	if got.Username != "alice" || got.Role != user.RoleAdmin || got.PasswordHash != "fake-hash" {
		t.Fatalf("got %+v", got)
	}

	got2, err := db.Users().GetByID(ctx, got.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got2.ID != got.ID {
		t.Fatalf("GetByID returned wrong user: %+v", got2)
	}
}

func TestUserRepo_DuplicateUsername(t *testing.T) {
	db := newTestDB(t)
	seedUser(t, db, "alice", user.RoleAdmin)

	err := db.Users().Create(context.Background(), &user.User{
		ID:           "other",
		Username:     "alice",
		PasswordHash: "x",
		Role:         user.RoleOperator,
		CreatedAt:    time.Now().UTC(),
	})
	if err == nil {
		t.Fatal("expected UNIQUE violation on duplicate username")
	}
}

func TestUserRepo_ListOrdersByUsername(t *testing.T) {
	db := newTestDB(t)
	seedUser(t, db, "charlie", user.RoleViewer)
	seedUser(t, db, "alice", user.RoleAdmin)
	seedUser(t, db, "bob", user.RoleOperator)

	users, err := db.Users().List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(users) != 3 {
		t.Fatalf("List length = %d; want 3", len(users))
	}
	want := []string{"alice", "bob", "charlie"}
	for i, u := range users {
		if u.Username != want[i] {
			t.Errorf("users[%d].Username = %q; want %q", i, u.Username, want[i])
		}
	}
}

func TestUserRepo_Count(t *testing.T) {
	db := newTestDB(t)

	n, err := db.Users().Count(context.Background())
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if n != 0 {
		t.Fatalf("empty DB Count = %d; want 0", n)
	}

	seedUser(t, db, "alice", user.RoleAdmin)
	seedUser(t, db, "bob", user.RoleOperator)

	n, err = db.Users().Count(context.Background())
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if n != 2 {
		t.Fatalf("Count = %d; want 2", n)
	}
}

func TestUserRepo_UpdatePasswordHash(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	u := seedUser(t, db, "alice", user.RoleAdmin)

	if err := db.Users().UpdatePasswordHash(ctx, u.ID, "new-hash"); err != nil {
		t.Fatalf("UpdatePasswordHash: %v", err)
	}

	got, _ := db.Users().GetByID(ctx, u.ID)
	if got.PasswordHash != "new-hash" {
		t.Fatalf("PasswordHash = %q; want new-hash", got.PasswordHash)
	}
}

func TestUserRepo_TouchLastLogin(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	u := seedUser(t, db, "alice", user.RoleAdmin)

	before := time.Now().Add(-time.Second).UTC()
	if err := db.Users().TouchLastLogin(ctx, u.ID); err != nil {
		t.Fatalf("TouchLastLogin: %v", err)
	}

	got, _ := db.Users().GetByID(ctx, u.ID)
	if got.LastLoginAt == nil {
		t.Fatal("LastLoginAt stayed nil after TouchLastLogin")
	}
	if got.LastLoginAt.Before(before) {
		t.Fatalf("LastLoginAt = %v predates %v", got.LastLoginAt, before)
	}
}

func TestUserRepo_Delete(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	u := seedUser(t, db, "alice", user.RoleAdmin)

	if err := db.Users().Delete(ctx, u.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if _, err := db.Users().GetByID(ctx, u.ID); err == nil {
		t.Fatal("GetByID after Delete should error")
	}
}

// NotFound is surfaced as storage.ErrNotFound so handlers can map it to 404.
func TestUserRepo_GetByUsername_NotFound(t *testing.T) {
	db := newTestDB(t)

	_, err := db.Users().GetByUsername(context.Background(), "nobody")
	if err == nil {
		t.Fatal("GetByUsername for missing user should error")
	}
	if err != storage.ErrNotFound {
		t.Fatalf("err = %v; want storage.ErrNotFound", err)
	}
}
