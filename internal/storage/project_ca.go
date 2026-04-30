package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// ProjectCA is the per-project root certificate authority. Exactly
// one row per project; the ECDSA P-256 private key is stored
// AES-GCM-encrypted against an operator-supplied KEK so a raw DB leak
// can't forge certs.
type ProjectCA struct {
	ProjectID     string
	CertPEM       string // PEM-encoded self-signed ECDSA P-256 root
	PrivKeyNonce  []byte // 12-byte AES-GCM nonce
	PrivKeyCT     []byte // AES-GCM-sealed PKCS#8 bytes (ECDSA P-256)
	SerialCounter int64
	CreatedAt     time.Time
	CreatedByUser string
	RevokedAt     *time.Time
	RevokedByUser string
	RevokedReason string
}

func (db *DB) ProjectCA() *ProjectCARepo { return &ProjectCARepo{db: db.DB} }

type ProjectCARepo struct {
	db *sql.DB
}

// Create inserts a fresh CA row for a project. Returns an error if one
// already exists — the server's CA lazy-init logic uses ErrNotFound to
// decide whether to mint and Create atomically. Caller is responsible
// for the encryption.
func (r *ProjectCARepo) Create(ctx context.Context, ca *ProjectCA) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO project_ca (
			project_id, cert_pem, privkey_nonce, privkey_ct,
			serial_counter, created_at, created_by_user,
			revoked_at, revoked_by_user, revoked_reason
		) VALUES (?, ?, ?, ?, 0, ?, ?, NULL, NULL, NULL)`,
		ca.ProjectID, ca.CertPEM, ca.PrivKeyNonce, ca.PrivKeyCT,
		ca.CreatedAt.UTC(), ca.CreatedByUser,
	)
	return err
}

// Get returns the CA row for a project, or ErrNotFound.
func (r *ProjectCARepo) Get(ctx context.Context, projectID string) (*ProjectCA, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT project_id, cert_pem, privkey_nonce, privkey_ct, serial_counter,
		       created_at, created_by_user, revoked_at, revoked_by_user, revoked_reason
		  FROM project_ca WHERE project_id = ?`, projectID)
	var (
		ca     ProjectCA
		revAt  sql.NullTime
		revBy  sql.NullString
		revRes sql.NullString
	)
	err := row.Scan(&ca.ProjectID, &ca.CertPEM, &ca.PrivKeyNonce, &ca.PrivKeyCT,
		&ca.SerialCounter, &ca.CreatedAt, &ca.CreatedByUser,
		&revAt, &revBy, &revRes)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if revAt.Valid {
		t := revAt.Time
		ca.RevokedAt = &t
	}
	ca.RevokedByUser = revBy.String
	ca.RevokedReason = revRes.String
	return &ca, nil
}

// AllocateSerial atomically increments and returns the next serial for
// a project's CA. Must be called inside a transaction that also
// inserts the matching issued_certs row — see
// enrollment.IssueAgentCert for the canonical usage.
func (r *ProjectCARepo) AllocateSerial(ctx context.Context, tx *sql.Tx, projectID string) (int64, error) {
	res, err := tx.ExecContext(ctx, `
		UPDATE project_ca
		   SET serial_counter = serial_counter + 1
		 WHERE project_id = ?`, projectID)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	if n == 0 {
		return 0, ErrNotFound
	}
	var serial int64
	if err := tx.QueryRowContext(ctx,
		`SELECT serial_counter FROM project_ca WHERE project_id = ?`, projectID).
		Scan(&serial); err != nil {
		return 0, err
	}
	return serial, nil
}

// BeginTx is a tiny convenience so the enrollment service doesn't reach
// through *DB.DB. Matches the idiom AgentSessions uses for RotateTo.
func (r *ProjectCARepo) BeginTx(ctx context.Context) (*sql.Tx, error) {
	return r.db.BeginTx(ctx, nil)
}
