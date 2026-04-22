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

// Open opens a SQLite database at path (":memory:" is fine for tests),
// enables foreign-key enforcement, and applies every embedded migration. If
// anything fails mid-setup the partially-opened connection is closed and an
// error is returned — callers never see half-migrated state.
func Open(path string) (*DB, error) {
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %q: %w", path, err)
	}
	if err := raw.Ping(); err != nil {
		return nil, closeWith(raw, fmt.Errorf("ping sqlite %q: %w", path, err))
	}
	// Pragmas are per-connection in SQLite — Go's sql.DB may create
	// several underlying connections under concurrent load, and each
	// spawn resets to defaults. Pinning MaxOpenConns to 1 guarantees
	// every query hits the single connection we configure below, which
	// is the standard idiom for SQLite (concurrent writers serialise on
	// the write lock anyway; one connection is strictly simpler).
	raw.SetMaxOpenConns(1)
	// SQLite defaults foreign_keys to OFF; the ON DELETE CASCADE edges in the
	// schema are only meaningful with it on.
	if _, err := raw.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return nil, closeWith(raw, fmt.Errorf("enable foreign_keys: %w", err))
	}
	// Block instead of failing fast when two callers both want a
	// RESERVED write lock at the same time. With MaxOpenConns=1 the
	// pool serialises for us, but the timeout is still belt-and-braces
	// for tests that sometimes hit the driver-level lock before the
	// pool's queue.
	if _, err := raw.Exec("PRAGMA busy_timeout = 5000"); err != nil {
		return nil, closeWith(raw, fmt.Errorf("set busy_timeout: %w", err))
	}
	if err := runMigrations(raw); err != nil {
		return nil, closeWith(raw, err)
	}
	return &DB{DB: raw}, nil
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
