package storage

import "testing"

// TestMigration_DropAAT_IndexGone locks in migration 000016: the
// project-scoped active-AAT partial index (`idx_auth_tokens_aat_active`,
// added in migration 13) must be gone after Open() runs all pending
// migrations.
//
// AAT was an admin-only experimental surface and is being deleted in
// favour of a user-self PAT (introduced in migration 17). The CHECK
// constraint on auth_tokens.kind still permits 'aat' until migration 17
// rebuilds the table — this test only asserts the cleanup that
// migration 16 performs by itself.
func TestMigration_DropAAT_IndexGone(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var name string
	err = db.QueryRow(
		`SELECT name FROM sqlite_master WHERE type='index' AND name='idx_auth_tokens_aat_active'`,
	).Scan(&name)
	if err == nil {
		t.Fatal("idx_auth_tokens_aat_active still present after migration 16; expected drop")
	}
}
