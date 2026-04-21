package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
)

// Host mirrors a row in the hosts table. FingerprintFallback is true when
// the agent didn't report a platform-level machine_id and the server is
// aggregating sessions purely on hostname + sorted MACs. The UI surfaces
// that flag so operators know the merge may be lossy.
type Host struct {
	ID                  string
	ProjectID           string
	MachineID           string // "" when FingerprintFallback=true
	Fingerprint         string
	FingerprintFallback bool
	Hostname            string
	PrimaryAlias        string
	OS                  string
	FirstSeenAt         time.Time
	LastSeenAt          time.Time
}

// HostIdentity carries the agent-reported identity we upsert into the
// hosts table. SeenAt is used for both first_seen_at (on insert) and
// last_seen_at (always).
type HostIdentity struct {
	ProjectID   string
	MachineID   string
	Fingerprint string
	Hostname    string
	OS          string
	SeenAt      time.Time
}

func (db *DB) Hosts() *HostRepo { return &HostRepo{db: db.DB} }

type HostRepo struct {
	db *sql.DB
}

// Upsert merges the given identity into the hosts table. Matching order:
//
//  1. If (project_id, machine_id) exists and machine_id != "", update it.
//  2. Else if (project_id, fingerprint) exists, update it — and if the new
//     identity has a non-empty machine_id, backfill it and clear
//     fingerprint_fallback.
//  3. Else insert a new row.
//
// Returns the resulting Host, always with FirstSeenAt preserved across
// updates and LastSeenAt set to SeenAt.
func (r *HostRepo) Upsert(ctx context.Context, ident *HostIdentity) (*Host, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	rollback := true
	defer func() {
		if rollback {
			_ = tx.Rollback()
		}
	}()

	// 1) Lookup by machine_id.
	var existing *Host
	if ident.MachineID != "" {
		existing, err = scanHostSingle(tx.QueryRowContext(ctx,
			`SELECT id, project_id, machine_id, fingerprint, fingerprint_fallback,
			        hostname, primary_alias, os, first_seen_at, last_seen_at
			   FROM hosts WHERE project_id = ? AND machine_id = ?`,
			ident.ProjectID, ident.MachineID))
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}
	// 2) Lookup by fingerprint as fallback / upgrade path.
	if existing == nil {
		existing, err = scanHostSingle(tx.QueryRowContext(ctx,
			`SELECT id, project_id, machine_id, fingerprint, fingerprint_fallback,
			        hostname, primary_alias, os, first_seen_at, last_seen_at
			   FROM hosts WHERE project_id = ? AND fingerprint = ?`,
			ident.ProjectID, ident.Fingerprint))
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}

	if existing != nil {
		// Update the row in place.
		newMachineID := existing.MachineID
		newFallback := existing.FingerprintFallback
		if ident.MachineID != "" {
			newMachineID = ident.MachineID
			newFallback = false
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE hosts
			   SET machine_id = ?,
			       fingerprint = ?,
			       fingerprint_fallback = ?,
			       hostname = ?,
			       os = ?,
			       last_seen_at = ?
			 WHERE id = ?`,
			nullIfEmpty(newMachineID),
			ident.Fingerprint,
			newFallback,
			ident.Hostname,
			ident.OS,
			ident.SeenAt.UTC(),
			existing.ID,
		); err != nil {
			return nil, err
		}
		existing.MachineID = newMachineID
		existing.Fingerprint = ident.Fingerprint
		existing.FingerprintFallback = newFallback
		existing.Hostname = ident.Hostname
		existing.OS = ident.OS
		existing.LastSeenAt = ident.SeenAt.UTC()

		rollback = false
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return existing, nil
	}

	// 3) Insert fresh.
	h := &Host{
		ID:                  uuid.NewString(),
		ProjectID:           ident.ProjectID,
		MachineID:           ident.MachineID,
		Fingerprint:         ident.Fingerprint,
		FingerprintFallback: ident.MachineID == "",
		Hostname:            ident.Hostname,
		OS:                  ident.OS,
		FirstSeenAt:         ident.SeenAt.UTC(),
		LastSeenAt:          ident.SeenAt.UTC(),
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO hosts
		  (id, project_id, machine_id, fingerprint, fingerprint_fallback,
		   hostname, primary_alias, os, first_seen_at, last_seen_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		h.ID, h.ProjectID, nullIfEmpty(h.MachineID), h.Fingerprint,
		h.FingerprintFallback, h.Hostname, h.PrimaryAlias, h.OS,
		h.FirstSeenAt, h.LastSeenAt,
	); err != nil {
		return nil, err
	}
	rollback = false
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return h, nil
}

func (r *HostRepo) ListByProject(ctx context.Context, projectID string) ([]*Host, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, project_id, machine_id, fingerprint, fingerprint_fallback,
		       hostname, primary_alias, os, first_seen_at, last_seen_at
		  FROM hosts WHERE project_id = ?
		 ORDER BY hostname ASC`, projectID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := []*Host{}
	for rows.Next() {
		h, err := scanHostRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

func (r *HostRepo) GetByID(ctx context.Context, id string) (*Host, error) {
	h, err := scanHostSingle(r.db.QueryRowContext(ctx, `
		SELECT id, project_id, machine_id, fingerprint, fingerprint_fallback,
		       hostname, primary_alias, os, first_seen_at, last_seen_at
		  FROM hosts WHERE id = ?`, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return h, nil
}

// scanHostRow scans a single row from *sql.Rows or *sql.Row via rowScanner.
func scanHostRow(s rowScanner) (*Host, error) {
	var (
		h         Host
		machineID sql.NullString
		primary   sql.NullString
	)
	err := s.Scan(
		&h.ID, &h.ProjectID, &machineID, &h.Fingerprint, &h.FingerprintFallback,
		&h.Hostname, &primary, &h.OS, &h.FirstSeenAt, &h.LastSeenAt,
	)
	if err != nil {
		return nil, err
	}
	if machineID.Valid {
		h.MachineID = machineID.String
	}
	if primary.Valid {
		h.PrimaryAlias = primary.String
	}
	return &h, nil
}

// scanHostSingle returns (nil, sql.ErrNoRows) when the row is empty, so
// Upsert can distinguish "no match" from "scan failure".
func scanHostSingle(row *sql.Row) (*Host, error) {
	h, err := scanHostRow(row)
	if err != nil {
		return nil, err
	}
	return h, nil
}

// nullIfEmpty maps "" to a NULL value so UNIQUE(project_id, machine_id)
// doesn't collide across multiple fingerprint-only rows (SQLite treats
// NULLs as distinct in UNIQUE constraints).
func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
