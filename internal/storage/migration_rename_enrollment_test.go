package storage

import (
	"context"
	"crypto/sha256"
	"testing"
	"time"
)

// TestMigration_EnrollmentTokensRenamed locks in migration 000015:
// after Open() runs all pending migrations, the legacy `pat_tokens` and
// `pat_redemption_events` tables are gone and their renamed successors
// `enrollment_tokens` and `enrollment_redemption_events` are present.
//
// The rename frees the "PAT" name for a future user-issued personal-
// access-token surface; the "PAT" tokens this table used to hold are
// actually one-shot agent-enrollment credentials, so the storage name
// is being aligned with what the rows really are.
func TestMigration_EnrollmentTokensRenamed(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	for _, want := range []string{"enrollment_tokens", "enrollment_redemption_events"} {
		var name string
		err := db.QueryRow(
			`SELECT name FROM sqlite_master WHERE type='table' AND name=?`,
			want,
		).Scan(&name)
		if err != nil {
			t.Errorf("expected table %q after migration 15: %v", want, err)
		}
	}
	for _, gone := range []string{"pat_tokens", "pat_redemption_events"} {
		var name string
		err := db.QueryRow(
			`SELECT name FROM sqlite_master WHERE type='table' AND name=?`,
			gone,
		).Scan(&name)
		if err == nil {
			t.Errorf("legacy table %q still present after migration 15", gone)
		}
	}
}

// TestMigration_EnrollmentTokensFKIntact asserts that the
// install_download_tokens.consumed_pat_id foreign key (added in
// migration 4 pointing at pat_tokens.token_id) was rewritten by SQLite's
// RENAME TO to point at the new enrollment_tokens table — and still
// enforces referential integrity. We seed an enrollment_tokens row and
// confirm an install_download_tokens row referencing it survives
// PRAGMA foreign_key_check.
func TestMigration_EnrollmentTokensFKIntact(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()

	// Seed user + project (FK targets for both tables we touch).
	if _, err := db.ExecContext(ctx, `
		INSERT INTO users (id, username, password_hash, role, created_at)
		VALUES ('u-fk', 'u-fk', 'x', 'admin', ?)`,
		time.Now().UTC()); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO projects (id, name, slug, created_at, created_by)
		VALUES ('p-fk', 'P', 'p-fk', ?, 'u-fk')`,
		time.Now().UTC()); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	// Insert into the renamed table directly via SQL — we want to verify
	// the schema, not the Go repo's behaviour.
	hash := sha256.Sum256([]byte("s"))
	_, err = db.ExecContext(ctx, `
		INSERT INTO enrollment_tokens (
			token_id, secret_hash, project_id, issued_by_user,
			issued_at, expires_at, max_uses, uses, revoked
		) VALUES ('plt_fk', ?, 'p-fk', 'u-fk', ?, ?, 1, 0, 0)`,
		hash[:], time.Now().UTC(), time.Now().Add(time.Hour).UTC())
	if err != nil {
		t.Fatalf("insert enrollment_tokens: %v", err)
	}

	// install_download_tokens.consumed_pat_id references the renamed
	// table. SQLite's RENAME TO rewrites the FK target automatically;
	// this insert proves the rewrite happened.
	_, err = db.ExecContext(ctx, `
		INSERT INTO install_download_tokens (
			download_id, secret_hash, project_id, issued_by_user,
			issued_at, expires_at, server_endpoint,
			consumed_pat_id
		) VALUES ('dl_fk', ?, 'p-fk', 'u-fk', ?, ?, 'host:1', 'plt_fk')`,
		hash[:], time.Now().UTC(), time.Now().Add(time.Hour).UTC())
	if err != nil {
		t.Fatalf("insert install_download_tokens referencing renamed table: %v", err)
	}

	rows, err := db.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatalf("foreign_key_check: %v", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var table, parent string
		var rowid int64
		var fkid int
		if err := rows.Scan(&table, &rowid, &parent, &fkid); err != nil {
			t.Fatalf("scan: %v", err)
		}
		t.Errorf("foreign_key_check violation: table=%s rowid=%d parent=%s fkid=%d",
			table, rowid, parent, fkid)
	}
}
