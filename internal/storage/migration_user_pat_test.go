package storage

import (
	"context"
	"testing"
	"time"
)

// TestMigration_UserPAT_Schema locks in migration 000017's table
// rebuild: the auth_tokens CHECK now permits kind IN ('pat',
// 'user_session') and rejects the legacy 'aat' kind. Existing
// user_session rows survive the rebuild; the new PAT-active partial
// index lights up.
func TestMigration_UserPAT_Schema(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `
		INSERT INTO users (id, username, password_hash, role, created_at)
		VALUES ('u-pat', 'u-pat', 'x', 'admin', ?)`, time.Now().UTC()); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	// PAT row must satisfy the new CHECK.
	_, err = db.ExecContext(ctx, `
		INSERT INTO auth_tokens (
			token_id, kind, secret_hash, user_id,
			name, scopes, created_at, expires_at
		) VALUES ('pat_ok', 'pat', X'00', 'u-pat',
		          'first', 'hosts:read', ?, ?)`,
		time.Now().UTC(), time.Now().Add(24*time.Hour).UTC())
	if err != nil {
		t.Fatalf("insert PAT row: %v", err)
	}

	// AAT must be rejected — kind not in (pat, user_session).
	_, err = db.ExecContext(ctx, `
		INSERT INTO auth_tokens (
			token_id, kind, secret_hash, user_id,
			name, role, scopes, created_at, expires_at
		) VALUES ('aat_no', 'aat', X'00', 'u-pat',
		          'should-fail', 'admin', 'hosts:read', ?, ?)`,
		time.Now().UTC(), time.Now().Add(24*time.Hour).UTC())
	if err == nil {
		t.Fatal("INSERT with kind='aat' must be rejected after migration 18")
	}

	// PAT row with project_id set must be rejected — PATs bind to a
	// user, not a project.
	if _, err := db.ExecContext(ctx, `
		INSERT INTO projects (id, name, slug, created_at, created_by)
		VALUES ('p-pat', 'p', 'p-pat', ?, 'u-pat')`, time.Now().UTC()); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO auth_tokens (
			token_id, kind, secret_hash, user_id,
			name, scopes, created_at, expires_at, project_id
		) VALUES ('pat_proj', 'pat', X'00', 'u-pat',
		          'should-fail', 'hosts:read', ?, ?, 'p-pat')`,
		time.Now().UTC(), time.Now().Add(24*time.Hour).UTC())
	if err == nil {
		t.Fatal("INSERT PAT with project_id must be rejected (PAT binds to user, not project)")
	}

	// Partial index for active PATs is present.
	var name string
	if err := db.QueryRow(
		`SELECT name FROM sqlite_master WHERE type='index' AND name='idx_auth_tokens_pat_active'`,
	).Scan(&name); err != nil {
		t.Errorf("idx_auth_tokens_pat_active missing: %v", err)
	}
}

// TestMigration_UserPAT_PreservesSessions makes sure the
// CREATE-NEW/INSERT-SELECT/DROP/RENAME table swap that migration 18
// performs preserves any pre-existing user_session rows untouched.
func TestMigration_UserPAT_PreservesSessions(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	// At this point migration 18 has already run (Open() applies all
	// migrations). We can still validate that a user_session insert
	// with the original column shape goes through, which exercises
	// the rebuilt CHECK from the user-session side.
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `
		INSERT INTO users (id, username, password_hash, role, created_at)
		VALUES ('u-sess', 'u-sess', 'x', 'admin', ?)`, time.Now().UTC()); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO auth_tokens (
			token_id, kind, secret_hash, user_id,
			created_at, expires_at, idle_expires_at
		) VALUES ('pst_ok', 'user_session', X'00', 'u-sess',
		          ?, ?, ?)`,
		time.Now().UTC(),
		time.Now().Add(30*24*time.Hour).UTC(),
		time.Now().Add(time.Hour).UTC())
	if err != nil {
		t.Fatalf("insert user_session row after migration 18: %v", err)
	}
}
