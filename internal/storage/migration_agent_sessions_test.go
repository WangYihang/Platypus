package storage

import "testing"

// TestMigration_AgentSessionsTableDropped locks in migration 000009:
// after Open() runs all pending migrations the agent_sessions table
// must be gone. If someone accidentally reverts the migration (or
// adds a later one that recreates the table), this test catches it.
func TestMigration_AgentSessionsTableDropped(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var name string
	err = db.QueryRow(
		`SELECT name FROM sqlite_master
		  WHERE type='table' AND name='agent_sessions'`,
	).Scan(&name)
	if err == nil {
		t.Fatalf("agent_sessions table still exists after migration")
	}
	// Expected: sql.ErrNoRows (table absent).
}
