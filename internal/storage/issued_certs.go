package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// IssuedCert is one row in issued_certs — the authoritative record of a
// certificate the project CA has signed. Append-only: revocation marks
// revoked_at, never deletes. The (project_id, serial) pair uniquely
// identifies a cert and is what appears in the CRL.
type IssuedCert struct {
	Serial        int64
	ProjectID     string
	AgentID       string
	CertPEM       string
	PubKeyPEM     string
	IssuedAt      time.Time
	NotBefore     time.Time
	NotAfter      time.Time
	IssuedReason  string // "enroll" | "rotation" | "reissue" | "admin"
	IssuedByUser  string
	RevokedAt     *time.Time
	RevokedByUser string
	RevokedReason string
}

// IsActive reports whether the cert is currently usable: not revoked
// and not past its expiry. Used by tests and by the /certs admin view.
func (c *IssuedCert) IsActive(now time.Time) bool {
	if c.RevokedAt != nil {
		return false
	}
	return c.NotAfter.After(now) && !c.NotBefore.After(now)
}

func (db *DB) IssuedCerts() *IssuedCertRepo { return &IssuedCertRepo{db: db.DB} }

type IssuedCertRepo struct {
	db *sql.DB
}

// InsertTx inserts a freshly-signed cert inside an existing transaction.
// The caller (enrollment.Service.IssueAgentCert) first calls
// ProjectCA.AllocateSerial to get a fresh serial, signs the cert, then
// calls this — all in one transaction so a failure between issue and
// persist doesn't leak out-of-band certs.
func (r *IssuedCertRepo) InsertTx(ctx context.Context, tx *sql.Tx, c *IssuedCert) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO issued_certs (
			serial, project_id, agent_id, cert_pem, pubkey_pem,
			issued_at, not_before, not_after, issued_reason,
			issued_by_user, revoked_at, revoked_by_user, revoked_reason
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL, NULL)`,
		c.Serial, c.ProjectID, nullableString(c.AgentID), c.CertPEM, c.PubKeyPEM,
		c.IssuedAt.UTC(), c.NotBefore.UTC(), c.NotAfter.UTC(),
		c.IssuedReason, nullableString(c.IssuedByUser),
	)
	return err
}

// ListByProject returns all issued certs in a project, newest first.
// Set activeOnly=true to filter out revoked + expired rows (hot path
// for the admin "active certs" view).
func (r *IssuedCertRepo) ListByProject(ctx context.Context, projectID string, activeOnly bool, now time.Time) ([]*IssuedCert, error) {
	q := `
		SELECT serial, project_id, agent_id, cert_pem, pubkey_pem,
		       issued_at, not_before, not_after, issued_reason,
		       issued_by_user, revoked_at, revoked_by_user, revoked_reason
		  FROM issued_certs WHERE project_id = ?`
	args := []any{projectID}
	if activeOnly {
		q += ` AND revoked_at IS NULL AND not_after > ?`
		args = append(args, now.UTC())
	}
	q += ` ORDER BY serial DESC`

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*IssuedCert
	for rows.Next() {
		c, err := scanIssuedCert(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Get fetches a specific cert by (project, serial). Returns ErrNotFound
// if missing.
func (r *IssuedCertRepo) Get(ctx context.Context, projectID string, serial int64) (*IssuedCert, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT serial, project_id, agent_id, cert_pem, pubkey_pem,
		       issued_at, not_before, not_after, issued_reason,
		       issued_by_user, revoked_at, revoked_by_user, revoked_reason
		  FROM issued_certs WHERE project_id = ? AND serial = ?`, projectID, serial)
	c, err := scanIssuedCert(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return c, err
}

// Revoke marks a cert revoked. Idempotent — revoking an already-revoked
// cert is a no-op.
func (r *IssuedCertRepo) Revoke(ctx context.Context, projectID string, serial int64, byUser, reason string, at time.Time) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE issued_certs
		   SET revoked_at = ?, revoked_by_user = ?, revoked_reason = ?
		 WHERE project_id = ? AND serial = ? AND revoked_at IS NULL`,
		at.UTC(), byUser, nullableString(reason), projectID, serial)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		// Either missing or already revoked — disambiguate via a Get so
		// handlers can 404 correctly.
		if _, err := r.Get(ctx, projectID, serial); errors.Is(err, ErrNotFound) {
			return ErrNotFound
		}
	}
	return nil
}

// ListRevokedLive returns certs that are revoked AND still within their
// not_after window — i.e. the entries that actually belong in a CRL.
// Rows whose not_after has passed are redundant (natural expiry) and
// get dropped from the CRL automatically.
func (r *IssuedCertRepo) ListRevokedLive(ctx context.Context, projectID string, now time.Time) ([]*IssuedCert, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT serial, project_id, agent_id, cert_pem, pubkey_pem,
		       issued_at, not_before, not_after, issued_reason,
		       issued_by_user, revoked_at, revoked_by_user, revoked_reason
		  FROM issued_certs
		 WHERE project_id = ? AND revoked_at IS NOT NULL AND not_after > ?
		 ORDER BY serial DESC`, projectID, now.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*IssuedCert
	for rows.Next() {
		c, err := scanIssuedCert(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func scanIssuedCert(row rowScanner) (*IssuedCert, error) {
	var (
		c         IssuedCert
		agentID   sql.NullString
		issuedBy  sql.NullString
		revAt     sql.NullTime
		revBy     sql.NullString
		revReason sql.NullString
	)
	err := row.Scan(
		&c.Serial, &c.ProjectID, &agentID, &c.CertPEM, &c.PubKeyPEM,
		&c.IssuedAt, &c.NotBefore, &c.NotAfter, &c.IssuedReason,
		&issuedBy, &revAt, &revBy, &revReason,
	)
	if err != nil {
		return nil, err
	}
	c.AgentID = agentID.String
	c.IssuedByUser = issuedBy.String
	if revAt.Valid {
		t := revAt.Time
		c.RevokedAt = &t
	}
	c.RevokedByUser = revBy.String
	c.RevokedReason = revReason.String
	return &c, nil
}
