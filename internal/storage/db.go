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
	"strings"
	"sync/atomic"

	"github.com/golang-migrate/migrate/v4"
	migratesqlite "github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"

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
// ":memory:" DBs get a process-unique shared-cache name so multiple
// connections in the pool actually point at the same in-memory DB —
// default ":memory:" gives every new connection its own empty DB, which
// breaks once MaxOpenConns isn't pinned to 1. Tests opening independent
// in-memory instances still get isolation via the per-Open counter.
func buildDSN(path string) string {
	const pragmas = "_pragma=journal_mode(WAL)" +
		"&_pragma=foreign_keys(ON)" +
		"&_pragma=busy_timeout(5000)" +
		"&_pragma=synchronous(NORMAL)"
	if path == ":memory:" {
		n := memDBCounter.Add(1)
		return fmt.Sprintf("file:platypus-mem-%d?mode=memory&cache=shared&%s", n, pragmas)
	}
	// file:... accepts a bare path; strings.HasPrefix guard keeps callers
	// that already built their own URI from being double-prefixed.
	if strings.HasPrefix(path, "file:") {
		sep := "?"
		if strings.Contains(path, "?") {
			sep = "&"
		}
		return path + sep + pragmas
	}
	return "file:" + path + "?" + pragmas
}

// closeWith closes db, joining any close error with the original so we don't
// silently drop either failure. Keeps the setup paths in Open() straight-line.
func closeWith(db *sql.DB, orig error) error {
	if cerr := db.Close(); cerr != nil {
		return errors.Join(orig, fmt.Errorf("close after error: %w", cerr))
	}
	return orig
}

func runMigrations(db *sql.DB) error {
	src, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("load embedded migrations: %w", err)
	}
	drv, err := migratesqlite.WithInstance(db, &migratesqlite.Config{})
	if err != nil {
		return fmt.Errorf("migrate sqlite driver: %w", err)
	}
	m, err := migrate.NewWithInstance("iofs", src, "sqlite", drv)
	if err != nil {
		return fmt.Errorf("migrate instance: %w", err)
	}
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrate up: %w", err)
	}
	return nil
}
