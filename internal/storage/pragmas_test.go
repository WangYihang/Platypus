package storage

import "testing"

// TestOpenEngagesWAL pins the PRAGMA invariants buildDSN() bakes into
// the connection URI. If a future change silently drops one of these
// (journal_mode=WAL, foreign_keys=ON, synchronous=NORMAL,
// busy_timeout=5000), this test goes red — catching regressions that
// would otherwise manifest as subtle concurrency or durability bugs.
func TestOpenEngagesWAL(t *testing.T) {
	db, err := Open(t.TempDir() + "/probe.db")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var mode string
	if err := db.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatalf("journal_mode: %v", err)
	}
	if mode != "wal" {
		t.Fatalf("journal_mode = %q, want wal", mode)
	}

	var fk int
	if err := db.QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil {
		t.Fatalf("foreign_keys: %v", err)
	}
	if fk != 1 {
		t.Fatalf("foreign_keys = %d, want 1", fk)
	}

	var sync int
	if err := db.QueryRow("PRAGMA synchronous").Scan(&sync); err != nil {
		t.Fatalf("synchronous: %v", err)
	}
	if sync != 1 { // 1 = NORMAL; SQLite default is 2 (FULL)
		t.Fatalf("synchronous = %d, want 1 (NORMAL)", sync)
	}

	var bt int
	if err := db.QueryRow("PRAGMA busy_timeout").Scan(&bt); err != nil {
		t.Fatalf("busy_timeout: %v", err)
	}
	if bt != 5000 {
		t.Fatalf("busy_timeout = %d, want 5000", bt)
	}
}
