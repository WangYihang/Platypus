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
	// SQLite defaults foreign_keys to OFF; the ON DELETE CASCADE edges in the
	// schema are only meaningful with it on.
	if _, err := raw.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return nil, closeWith(raw, fmt.Errorf("enable foreign_keys: %w", err))
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
