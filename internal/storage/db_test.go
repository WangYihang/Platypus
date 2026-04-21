package storage_test

import (
	"testing"

	"github.com/WangYihang/Platypus/internal/storage"
)

// Opening an in-memory DB must apply every migration so the schema is ready
// for use. Any future migration that fails to register, or any driver change
// that breaks migration application, will surface here first.
func TestOpen_AppliesInitialSchema(t *testing.T) {
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	wantTables := []string{
		"users",
		"refresh_tokens",
		"projects",
		"project_members",
		"listeners",
		"hosts",
		"sessions",
	}
	for _, tbl := range wantTables {
		var name string
		err := db.QueryRow(
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?",
			tbl,
		).Scan(&name)
		if err != nil {
			t.Errorf("table %q missing after migrations: %v", tbl, err)
		}
	}
}

// CHECK constraints on role enums are a first-line defence against bad data
// reaching the DB via an application-layer bug. Verify the user role check
// actually rejects unknown values.
func TestOpen_RejectsUnknownUserRole(t *testing.T) {
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(
		`INSERT INTO users (id, username, password_hash, role, created_at)
		 VALUES (?, ?, ?, ?, datetime('now'))`,
		"u1", "alice", "hash", "superuser",
	)
	if err == nil {
		t.Fatal("expected CHECK constraint to reject role='superuser'; got nil")
	}
}

// Foreign key enforcement is disabled by default in SQLite, so the wrapper
// must turn it on. Without this, ON DELETE CASCADE is silently ignored.
func TestOpen_EnforcesForeignKeys(t *testing.T) {
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	var on int
	if err := db.QueryRow("PRAGMA foreign_keys").Scan(&on); err != nil {
		t.Fatalf("query pragma: %v", err)
	}
	if on != 1 {
		t.Fatalf("foreign_keys pragma = %d; want 1", on)
	}
}
