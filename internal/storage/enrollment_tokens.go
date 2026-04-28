package storage

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"errors"
	"time"
)

// EnrollmentToken mirrors one row in enrollment_tokens. The plaintext
// secret exists only in memory during the minting flow; the stored
// value is SHA-256(secret).
//
// The on-the-wire shape is the historical "PAT" — the row id is
// `plt_<...>` and the admin REST surface still says /pat-tokens — but
// these tokens are one-shot agent-enrollment credentials, not
// long-lived user PATs. The struct name matches the lifecycle.
type EnrollmentToken struct {
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

// EnrollmentStatus is a view-only value derived from EnrollmentToken at
// read time. We deliberately don't materialise this column in the
// database — keeping state in (revoked, expires_at, uses, max_uses)
// avoids any chance of drift between the derived value and the
// underlying facts.
type EnrollmentStatus string

const (
	EnrollmentStatusPending  EnrollmentStatus = "pending"
	EnrollmentStatusConsumed EnrollmentStatus = "consumed"
	EnrollmentStatusExpired  EnrollmentStatus = "expired"
	EnrollmentStatusRevoked  EnrollmentStatus = "revoked"
)

// Status returns the derived status of the token as of `now`.
func (p *EnrollmentToken) Status(now time.Time) EnrollmentStatus {
	switch {
	case p.Revoked:
		return EnrollmentStatusRevoked
	case p.Uses >= p.MaxUses:
		return EnrollmentStatusConsumed
	case !p.ExpiresAt.After(now):
		return EnrollmentStatusExpired
	default:
		return EnrollmentStatusPending
	}
}

// ErrEnrollmentTokenAlreadyConsumed is returned by atomic redeem paths
// when a racing request successfully incremented uses first. It is NOT
// a CHECK failure on secret or binding — those have their own sentinel
// values.
var ErrEnrollmentTokenAlreadyConsumed = errors.New("storage: enrollment token already consumed")

func (db *DB) EnrollmentTokens() *EnrollmentTokenRepo { return &EnrollmentTokenRepo{db: db.DB} }

type EnrollmentTokenRepo struct {
	db *sql.DB
}

// Create inserts a row representing a freshly-minted enrollment token.
// The plaintext secret is hashed by the caller — we never want it
// materialised here.
func (r *EnrollmentTokenRepo) Create(ctx context.Context, p *EnrollmentToken) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO enrollment_tokens (
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

// Get fetches an enrollment token by its token_id. Returns ErrNotFound if missing.
func (r *EnrollmentTokenRepo) Get(ctx context.Context, tokenID string) (*EnrollmentToken, error) {
	return r.scanOne(ctx, `
		SELECT token_id, secret_hash, project_id, issued_by_user, issued_at,
		       expires_at, max_uses, uses, binding_machine_id,
		       binding_host_alias, description, revoked, revoked_at,
		       revoked_by_user, revoked_reason
		  FROM enrollment_tokens WHERE token_id = ?`, tokenID)
}

// ListByProject returns all enrollment tokens in a project, newest
// first. Optional includeInactive controls whether revoked rows show
// up. This hits the idx_enrollment_unrevoked partial index only when
// the caller filters on unrevoked; for full history we do a full scan,
// which is fine for projects with thousands of historical tokens.
func (r *EnrollmentTokenRepo) ListByProject(ctx context.Context, projectID string, includeInactive bool) ([]*EnrollmentToken, error) {
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
			  FROM enrollment_tokens WHERE project_id = ?
			  ORDER BY issued_at DESC`, projectID)
	} else {
		// Still returns consumed/expired rows alongside pending ones; callers
		// filter precisely by status in-app. We rely on idx_enrollment_unrevoked
		// for the revoked=0 path here.
		rows, err = r.db.QueryContext(ctx, `
			SELECT token_id, secret_hash, project_id, issued_by_user, issued_at,
			       expires_at, max_uses, uses, binding_machine_id,
			       binding_host_alias, description, revoked, revoked_at,
			       revoked_by_user, revoked_reason
			  FROM enrollment_tokens
			 WHERE project_id = ? AND revoked = 0
			 ORDER BY issued_at DESC`, projectID)
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []*EnrollmentToken
	for rows.Next() {
		p, err := scanEnrollmentToken(rows)
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
// On "success" the returned *EnrollmentToken reflects post-increment state.
func (r *EnrollmentTokenRepo) TryConsume(ctx context.Context, tokenID string, secret []byte, machineID string, now time.Time) (*EnrollmentToken, string, error) {
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

	p, err := scanEnrollmentTokenSingle(tx.QueryRowContext(ctx, `
		SELECT token_id, secret_hash, project_id, issued_by_user, issued_at,
		       expires_at, max_uses, uses, binding_machine_id,
		       binding_host_alias, description, revoked, revoked_at,
		       revoked_by_user, revoked_reason
		  FROM enrollment_tokens WHERE token_id = ?`, tokenID))
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
		UPDATE enrollment_tokens
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
func (r *EnrollmentTokenRepo) Revoke(ctx context.Context, tokenID, byUser, reason string, at time.Time) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE enrollment_tokens
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
func (r *EnrollmentTokenRepo) scanOne(ctx context.Context, q string, args ...interface{}) (*EnrollmentToken, error) {
	return scanEnrollmentTokenSingle(r.db.QueryRowContext(ctx, q, args...))
}

func scanEnrollmentTokenSingle(row rowScanner) (*EnrollmentToken, error) {
	p, err := scanEnrollmentToken(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return p, err
}

func scanEnrollmentToken(row rowScanner) (*EnrollmentToken, error) {
	var (
		p        EnrollmentToken
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
