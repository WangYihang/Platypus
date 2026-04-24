package storage

import (
	"context"
	"database/sql"
	"time"
)

// AdminSetting is one row in admin_settings — a JSON-encoded override
// for a runtime-tunable knob. Absence in this table means "fall back
// to YAML / hardcoded default"; presence means "admin explicitly set
// this via the Web UI".
type AdminSetting struct {
	Key       string
	Value     string // JSON-encoded; the settings registry parses it
	UpdatedAt time.Time
	UpdatedBy string // user ID
}

func (db *DB) AdminSettings() *AdminSettingsRepo {
	return &AdminSettingsRepo{db: db.DB}
}

type AdminSettingsRepo struct {
	db *sql.DB
}

// Get returns the row for key, or ErrNotFound when absent.
func (r *AdminSettingsRepo) Get(ctx context.Context, key string) (*AdminSetting, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT key, value, updated_at, updated_by FROM admin_settings WHERE key = ?`, key)
	var s AdminSetting
	if err := row.Scan(&s.Key, &s.Value, &s.UpdatedAt, &s.UpdatedBy); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &s, nil
}

// Upsert inserts or overwrites a setting. Callers have already
// JSON-encoded the value and validated it against the expected type.
func (r *AdminSettingsRepo) Upsert(ctx context.Context, key, value, updatedBy string, at time.Time) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO admin_settings (key, value, updated_at, updated_by)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET
			value = excluded.value,
			updated_at = excluded.updated_at,
			updated_by = excluded.updated_by
	`, key, value, at, updatedBy)
	return err
}

// Delete removes the override for key. Subsequent reads fall back to
// YAML / hardcoded default. No-op when the key is absent.
func (r *AdminSettingsRepo) Delete(ctx context.Context, key string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM admin_settings WHERE key = ?`, key)
	return err
}

// All lists every override row, ordered by key for deterministic output.
func (r *AdminSettingsRepo) All(ctx context.Context) ([]*AdminSetting, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT key, value, updated_at, updated_by FROM admin_settings ORDER BY key ASC`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []*AdminSetting
	for rows.Next() {
		var s AdminSetting
		if err := rows.Scan(&s.Key, &s.Value, &s.UpdatedAt, &s.UpdatedBy); err != nil {
			return nil, err
		}
		out = append(out, &s)
	}
	return out, rows.Err()
}
