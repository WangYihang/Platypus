// Package storage wraps the SQLite connection and schema migrations used by
// the rest of the server. A single Open() call is the only entry point; it
// runs every pending migration before returning, so callers never see a
// partially-migrated database.
package storage

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"

	_ "modernc.org/sqlite" // pure-Go driver; keeps the server CGO-free for cross-compiles
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// DB wraps *sql.DB so we can add lifecycle helpers later without changing
// call-sites. It exposes the embedded *sql.DB directly via struct promotion.
type DB struct {
	*sql.DB
}

// memDBCounter gives each ":memory:" Open() call a process-unique name so
// multiple sql.DB connections share one in-memory DB (required once
// MaxOpenConns isn't pinned to 1) while different DB instances stay
// isolated.
var memDBCounter atomic.Uint64

// Open opens a SQLite database at path (":memory:" is fine for tests),
// runs every pending migration, and returns a handle. Pragmas are encoded
// in the DSN via modernc.org/sqlite's _pragma query param so they're
// applied to every connection the pool opens — WAL requires concurrent
// readers, and foreign-keys/busy-timeout are per-connection in SQLite, so
// the single-conn pinning the previous version used is no longer needed.
// If anything fails mid-setup the partially-opened connection is closed
// and an error is returned — callers never see half-migrated state.
func Open(path string) (*DB, error) {
	raw, err := sql.Open("sqlite", buildDSN(path))
	if err != nil {
		return nil, fmt.Errorf("open sqlite %q: %w", path, err)
	}
	if err := raw.Ping(); err != nil {
		return nil, closeWith(raw, fmt.Errorf("ping sqlite %q: %w", path, err))
	}
	if err := runMigrations(raw); err != nil {
		return nil, closeWith(raw, err)
	}
	return &DB{DB: raw}, nil
}

// buildDSN turns a caller-facing path ("/var/lib/platypus.db" or
// ":memory:") into a modernc.org/sqlite URI with the PRAGMAs the server
// relies on baked in:
//
//   - journal_mode=WAL:   concurrent readers, single writer. Persistent
//     across restarts (written to the DB header); setting it every
//     connection is a no-op after the first.
//   - foreign_keys=ON:    ON DELETE CASCADE edges in the schema require
//     this; SQLite defaults it to off, per-connection.
//   - busy_timeout=5000:  block up to 5s on a RESERVED write lock before
//     returning SQLITE_BUSY.
//   - synchronous=NORMAL: safe with WAL (fsync on checkpoint, not on
//     every commit). The durability window is "power loss within the
//     last few seconds", which is acceptable for our workload.
//
// plus a _txlock=immediate driver option. Default BEGIN is "deferred":
// the transaction starts as a reader and upgrades to a writer on the
// first write. Two concurrent deferred transactions that both try to
// upgrade race against the write lock and one gets SQLITE_BUSY_SNAPSHOT
// (517) — even with busy_timeout. _txlock=immediate makes every BEGIN
// acquire the write lock up front; contenders wait on the busy_timeout
// properly and serialise cleanly.
//
// ":memory:" DBs get a process-unique shared-cache name so multiple
// connections in the pool actually point at the same in-memory DB —
// default ":memory:" gives every new connection its own empty DB, which
// breaks once MaxOpenConns isn't pinned to 1. Tests opening independent
// in-memory instances still get isolation via the per-Open counter.
func buildDSN(path string) string {
	const params = "_txlock=immediate" +
		"&_pragma=journal_mode(WAL)" +
		"&_pragma=foreign_keys(ON)" +
		"&_pragma=busy_timeout(5000)" +
		"&_pragma=synchronous(NORMAL)"
	if path == ":memory:" {
		n := memDBCounter.Add(1)
		return fmt.Sprintf("file:platypus-mem-%d?mode=memory&cache=shared&%s", n, params)
	}
	// file:... accepts a bare path; strings.HasPrefix guard keeps callers
	// that already built their own URI from being double-prefixed.
	if strings.HasPrefix(path, "file:") {
		sep := "?"
		if strings.Contains(path, "?") {
			sep = "&"
		}
		return path + sep + params
	}
	return "file:" + path + "?" + params
}

// closeWith closes db, joining any close error with the original so we don't
// silently drop either failure. Keeps the setup paths in Open() straight-line.
func closeWith(db *sql.DB, orig error) error {
	if cerr := db.Close(); cerr != nil {
		return errors.Join(orig, fmt.Errorf("close after error: %w", cerr))
	}
	return orig
}

// runMigrations replays every pending NNNNNN_*.sql under migrations/
// against db, in lexicographic order. The leading 6-digit prefix is the
// migration's version; schema_migrations records the highest version
// applied so reopening a migrated DB is a no-op.
//
// All pending migrations execute inside a single transaction. SQLite
// serialises writers anyway and our migrations are pure DDL — running
// 23 of them in 23 separate BEGIN/COMMIT pairs is pure overhead,
// especially under -race where each transaction commit pays the full
// fsync + WAL bookkeeping cost. Bundling them halves the wall-clock
// time of a fresh Open() and makes the test suite tractable under the
// race detector. Failure semantics improve too: a broken migration
// leaves the DB at the previous version cleanly instead of in a
// partially-applied state.
//
// We dropped golang-migrate/v4 here because its sqlite driver leaked
// one io.Pipe writer goroutine per Open() call (the prefetch ring is
// never drained on early Up() exit, and the driver's Close() helpfully
// closes the *sql.DB the caller still owns). Under -race those leaked
// goroutines accumulated until the suite deadlocked at 120s.
func runMigrations(db *sql.DB) error {
	if _, err := db.Exec(`
        CREATE TABLE IF NOT EXISTS schema_migrations (
            version    INTEGER PRIMARY KEY,
            applied_at INTEGER NOT NULL
        )`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	var current int64
	if err := db.QueryRow(
		`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`,
	).Scan(&current); err != nil {
		return fmt.Errorf("read current schema version: %w", err)
	}

	pending, err := loadMigrations(current)
	if err != nil {
		return err
	}
	if len(pending) == 0 {
		return nil
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin migration tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, mig := range pending {
		if _, err := tx.Exec(mig.body); err != nil {
			return fmt.Errorf("apply migration %06d_%s: %w", mig.version, mig.name, err)
		}
		if _, err := tx.Exec(
			`INSERT INTO schema_migrations(version, applied_at) VALUES (?, unixepoch())`,
			mig.version,
		); err != nil {
			return fmt.Errorf("record migration %06d_%s: %w", mig.version, mig.name, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migrations: %w", err)
	}
	return nil
}

type migration struct {
	version int64
	name    string
	body    string
}

// loadMigrations returns every migration with version > after, ordered by
// version ascending. Filenames must match `NNNNNN_name.sql`; anything
// else is rejected loudly so a typo doesn't silently get skipped.
func loadMigrations(after int64) ([]migration, error) {
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("read embedded migrations: %w", err)
	}
	out := make([]migration, 0, len(entries))
	for _, ent := range entries {
		if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".sql") {
			continue
		}
		name := strings.TrimSuffix(ent.Name(), ".sql")
		i := strings.IndexByte(name, '_')
		if i <= 0 {
			return nil, fmt.Errorf("malformed migration filename %q (want NNNNNN_name.sql)", ent.Name())
		}
		v, err := strconv.ParseInt(name[:i], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("malformed migration version in %q: %w", ent.Name(), err)
		}
		if v <= after {
			continue
		}
		body, err := fs.ReadFile(migrationsFS, path.Join("migrations", ent.Name()))
		if err != nil {
			return nil, fmt.Errorf("read migration %q: %w", ent.Name(), err)
		}
		out = append(out, migration{version: v, name: name[i+1:], body: string(body)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].version < out[j].version })
	return out, nil
}

