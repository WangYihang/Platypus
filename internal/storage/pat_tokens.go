package storage

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"errors"
	"time"
)

// PATToken mirrors one row in pat_tokens. The plaintext secret exists only
// in memory during the minting flow; the stored value is SHA-256(secret).
type PATToken struct {
	TokenID          string
	SecretHash       []byte
	ProjectID        string
	IssuedByUser     string
	IssuedAt         time.Time
	ExpiresAt        time.Time
	MaxUses          int
	Uses             int
	BindingMachineID string
	BindingHostAlias string
	Description      string
	Revoked          bool
	RevokedAt        *time.Time
	RevokedByUser    string
	RevokedReason    string
}

// PATStatus is a view-only value derived from PATToken at read time. We
// deliberately don't materialise this column in the database — keeping
// state in (revoked, expires_at, uses, max_uses) avoids any chance of
// drift between the derived value and the underlying facts.
type PATStatus string

const (
	PATStatusPending  PATStatus = "pending"
	PATStatusConsumed PATStatus = "consumed"
	PATStatusExpired  PATStatus = "expired"
	PATStatusRevoked  PATStatus = "revoked"
)

// Status returns the derived status of the token as of `now`.
func (p *PATToken) Status(now time.Time) PATStatus {
	switch {
	case p.Revoked:
		return PATStatusRevoked
	case p.Uses >= p.MaxUses:
		return PATStatusConsumed
	case !p.ExpiresAt.After(now):
		return PATStatusExpired
	default:
		return PATStatusPending
	}
}

// ErrPATAlreadyConsumed is returned by atomic redeem paths when a racing
// request successfully incremented uses first. It is NOT a CHECK failure
// on secret or binding — those have their own sentinel values.
var ErrPATAlreadyConsumed = errors.New("storage: PAT already consumed")

func (db *DB) PATTokens() *PATTokenRepo { return &PATTokenRepo{db: db.DB} }

type PATTokenRepo struct {
	db *sql.DB
}

// Create inserts a row representing a freshly-minted PAT. The plaintext
// secret is hashed by the caller — we never want it materialised here.
func (r *PATTokenRepo) Create(ctx context.Context, p *PATToken) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO pat_tokens (
			token_id, secret_hash, project_id, issued_by_user,
			issued_at, expires_at, max_uses, uses,
			binding_machine_id, binding_host_alias, description,
			revoked, revoked_at, revoked_by_user, revoked_reason
		) VALUES (?, ?, ?, ?, ?, ?, ?, 0, ?, ?, ?, 0, NULL, NULL, NULL)`,
		p.TokenID, p.SecretHash, p.ProjectID, p.IssuedByUser,
		p.IssuedAt.UTC(), p.ExpiresAt.UTC(), p.MaxUses,
		nullableString(p.BindingMachineID),
		nullableString(p.BindingHostAlias),
		nullableString(p.Description),
	)
	return err
}

// Get fetches a PAT by its token_id. Returns ErrNotFound if missing.
func (r *PATTokenRepo) Get(ctx context.Context, tokenID string) (*PATToken, error) {
	return r.scanOne(ctx, `
		SELECT token_id, secret_hash, project_id, issued_by_user, issued_at,
		       expires_at, max_uses, uses, binding_machine_id,
		       binding_host_alias, description, revoked, revoked_at,
		       revoked_by_user, revoked_reason
		  FROM pat_tokens WHERE token_id = ?`, tokenID)
}

// ListByProject returns all PATs in a project, newest first. Optional
// statusFilter limits the result at the app layer (filter empty means
// all). This hits the idx_pat_unrevoked partial index only when the
// caller filters on unrevoked; for full history we do a full scan,
// which is fine for projects with thousands of historical tokens.
func (r *PATTokenRepo) ListByProject(ctx context.Context, projectID string, includeInactive bool) ([]*PATToken, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if includeInactive {
		rows, err = r.db.QueryContext(ctx, `
			SELECT token_id, secret_hash, project_id, issued_by_user, issued_at,
			       expires_at, max_uses, uses, binding_machine_id,
			       binding_host_alias, description, revoked, revoked_at,
			       revoked_by_user, revoked_reason
			  FROM pat_tokens WHERE project_id = ?
			  ORDER BY issued_at DESC`, projectID)
	} else {
		// Still returns consumed/expired rows alongside pending ones; callers
		// filter precisely by status in-app. We rely on idx_pat_unrevoked
		// for the revoked=0 path here.
		rows, err = r.db.QueryContext(ctx, `
			SELECT token_id, secret_hash, project_id, issued_by_user, issued_at,
			       expires_at, max_uses, uses, binding_machine_id,
			       binding_host_alias, description, revoked, revoked_at,
			       revoked_by_user, revoked_reason
			  FROM pat_tokens
			 WHERE project_id = ? AND revoked = 0
			 ORDER BY issued_at DESC`, projectID)
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []*PATToken
	for rows.Next() {
		p, err := scanPATToken(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// TryConsume atomically verifies a token for use and increments its
// uses counter if (and only if) the token is still redeemable. The
// check+increment happens in a single SQL statement so two concurrent
// redemption attempts can never both succeed.
//
// Semantics (returning a classification string that callers log verbatim):
//
//	"success"                    — token valid and consumed this call
//	"unknown_token"              — no row with that token_id
//	"invalid_secret"             — hash mismatch
//	"expired"                    — expires_at <= now
//	"revoked"                    — admin killed it
//	"max_uses_reached"           — uses >= max_uses
//	"binding_machine_mismatch"   — binding_machine_id set and ≠ provided
//
// On "success" the returned *PATToken reflects post-increment state.
func (r *PATTokenRepo) TryConsume(ctx context.Context, tokenID string, secret []byte, machineID string, now time.Time) (*PATToken, string, error) {
	// BEGIN IMMEDIATE grabs a RESERVED lock up-front in SQLite, forcing
	// concurrent writers to queue. This is how we make the
	// compare-then-update sequence atomic without FOR UPDATE.
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, "", err
	}
	rollback := true
	defer func() {
		if rollback {
			_ = tx.Rollback()
		}
	}()

	p, err := scanPATTokenSingle(tx.QueryRowContext(ctx, `
		SELECT token_id, secret_hash, project_id, issued_by_user, issued_at,
		       expires_at, max_uses, uses, binding_machine_id,
		       binding_host_alias, description, revoked, revoked_at,
		       revoked_by_user, revoked_reason
		  FROM pat_tokens WHERE token_id = ?`, tokenID))
	if errors.Is(err, ErrNotFound) {
		return nil, "unknown_token", nil
	}
	if err != nil {
		return nil, "", err
	}

	// Classify without revealing which specific check failed to anyone but
	// the audit log — callers should surface a single generic rejection to
	// the agent. The ordering here matters only for audit clarity, not
	// security (we continue to use constant-time compare below).
	secretSum := sha256.Sum256(secret)
	if subtle.ConstantTimeCompare(secretSum[:], p.SecretHash) != 1 {
		return p, "invalid_secret", nil
	}
	if p.Revoked {
		return p, "revoked", nil
	}
	if !p.ExpiresAt.After(now) {
		return p, "expired", nil
	}
	if p.Uses >= p.MaxUses {
		return p, "max_uses_reached", nil
	}
	if p.BindingMachineID != "" && p.BindingMachineID != machineID {
		return p, "binding_machine_mismatch", nil
	}

	// Conditional UPDATE — double-guard against any lost race. If the row
	// state changed between our read and this write (impossible under
	// IMMEDIATE but cheap belt-and-braces), RowsAffected will be 0.
	res, err := tx.ExecContext(ctx, `
		UPDATE pat_tokens
		   SET uses = uses + 1
		 WHERE token_id = ?
		   AND revoked = 0
		   AND expires_at > ?
		   AND uses < max_uses`,
		tokenID, now.UTC())
	if err != nil {
		return nil, "", err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, "", err
	}
	if n == 0 {
		return p, "max_uses_reached", nil
	}
	p.Uses++

	if err := tx.Commit(); err != nil {
		return nil, "", err
	}
	rollback = false
	return p, "success", nil
}

// Revoke marks a token revoked. Idempotent — revoking twice is a no-op.
func (r *PATTokenRepo) Revoke(ctx context.Context, tokenID, byUser, reason string, at time.Time) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE pat_tokens
		   SET revoked = 1, revoked_at = ?, revoked_by_user = ?, revoked_reason = ?
		 WHERE token_id = ? AND revoked = 0`,
		at.UTC(), byUser, nullableString(reason), tokenID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		// Either the row doesn't exist or it was already revoked. Use a
		// follow-up lookup to distinguish so the handler can return 404
		// vs 200 correctly.
		if _, err := r.Get(ctx, tokenID); errors.Is(err, ErrNotFound) {
			return ErrNotFound
		}
	}
	return nil
}

// scanOne runs q, scans a single row, and returns ErrNotFound on empty.
func (r *PATTokenRepo) scanOne(ctx context.Context, q string, args ...interface{}) (*PATToken, error) {
	return scanPATTokenSingle(r.db.QueryRowContext(ctx, q, args...))
}

func scanPATTokenSingle(row rowScanner) (*PATToken, error) {
	p, err := scanPATToken(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return p, err
}

func scanPATToken(row rowScanner) (*PATToken, error) {
	var (
		p        PATToken
		bindMid  sql.NullString
		bindAlia sql.NullString
		desc     sql.NullString
		revokAt  sql.NullTime
		revokBy  sql.NullString
		revokRes sql.NullString
		revoked  int
	)
	err := row.Scan(
		&p.TokenID, &p.SecretHash, &p.ProjectID, &p.IssuedByUser,
		&p.IssuedAt, &p.ExpiresAt, &p.MaxUses, &p.Uses,
		&bindMid, &bindAlia, &desc,
		&revoked, &revokAt, &revokBy, &revokRes,
	)
	if err != nil {
		return nil, err
	}
	p.BindingMachineID = bindMid.String
	p.BindingHostAlias = bindAlia.String
	p.Description = desc.String
	p.Revoked = revoked == 1
	if revokAt.Valid {
		t := revokAt.Time
		p.RevokedAt = &t
	}
	p.RevokedByUser = revokBy.String
	p.RevokedReason = revokRes.String
	return &p, nil
}

// nullableString maps "" → nil for columns we explicitly want NULL-free
// in the "unset" case. Keeps semantics crisp (binding_machine_id IS NULL
// vs binding_machine_id = ”).
func nullableString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
