package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/WangYihang/Platypus/internal/optoken"
	"github.com/WangYihang/Platypus/internal/user"
)

// UserSession is the typed view of an auth_tokens row with kind=
// 'user_session'. Roles / scopes are intentionally absent — sessions
// inherit them from the live users.role at verify time so a demoted
// admin sees the change on the next request rather than at the next
// logout.
type UserSession struct {
	TokenID       string
	SecretHash    []byte
	UserID        string
	UserAgent     string
	CreatedAt     time.Time
	ExpiresAt     time.Time
	IdleExpiresAt time.Time
	LastUsedAt    *time.Time
	LastUsedIP    string
	Revoked       bool
	RevokedAt     *time.Time
	RevokedByUser string
	RevokedReason string
}

// AuthTokens returns the repo accessor. Mirrors the (db *DB).EnrollmentTokens()
// pattern so callers don't learn about a different access shape per kind.
func (db *DB) AuthTokens() *AuthTokenRepo { return &AuthTokenRepo{db: db.DB} }

type AuthTokenRepo struct {
	db *sql.DB
}

// CreateSession inserts an auth_tokens row with kind='user_session'.
// Validates the input shape against what the table CHECK requires —
// the DB will reject any inconsistency, but a Go-side check gives the
// caller a precise error message instead of a generic CHECK failure.
func (r *AuthTokenRepo) CreateSession(ctx context.Context, s *UserSession) error {
	if err := validateSession(s); err != nil {
		return err
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO auth_tokens (
			token_id, kind, secret_hash, user_id,
			user_agent,
			created_at, expires_at, idle_expires_at
		) VALUES (?, 'user_session', ?, ?, ?, ?, ?, ?)`,
		s.TokenID,
		s.SecretHash,
		s.UserID,
		nullableString(s.UserAgent),
		s.CreatedAt.UTC(),
		s.ExpiresAt.UTC(),
		s.IdleExpiresAt.UTC(),
	)
	return err
}

func validateSession(s *UserSession) error {
	switch {
	case s == nil:
		return errors.New("storage: UserSession is nil")
	case s.TokenID == "":
		return errors.New("storage: UserSession.TokenID empty")
	case s.UserID == "":
		return errors.New("storage: UserSession.UserID empty")
	case len(s.SecretHash) == 0:
		return errors.New("storage: UserSession.SecretHash empty")
	case s.CreatedAt.IsZero():
		return errors.New("storage: UserSession.CreatedAt unset")
	case s.ExpiresAt.IsZero():
		return errors.New("storage: UserSession.ExpiresAt unset")
	case s.IdleExpiresAt.IsZero():
		return errors.New("storage: UserSession.IdleExpiresAt unset")
	}
	return nil
}

// GetSession fetches a single user_session by id. ErrNotFound for
// missing rows; the kind filter rejects rows of any other kind so the
// typed accessor never silently leaks a non-session row.
func (r *AuthTokenRepo) GetSession(ctx context.Context, tokenID string) (*UserSession, error) {
	row := r.db.QueryRowContext(ctx, baseSelect+` WHERE token_id = ? AND kind = 'user_session'`, tokenID)
	rec, err := scanAuthTokenRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return rowToSession(rec), nil
}

// ListSessionsForUser returns active (non-revoked) sessions a user
// holds, newest first. Drives the "Active sessions" UI in account
// settings — revoked rows would only confuse a user looking at their
// live device list.
func (r *AuthTokenRepo) ListSessionsForUser(ctx context.Context, userID string) ([]*UserSession, error) {
	rows, err := r.db.QueryContext(ctx, baseSelect+`
		WHERE kind = 'user_session' AND user_id = ? AND revoked_at IS NULL
		ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []*UserSession
	for rows.Next() {
		rec, err := scanAuthTokenRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rowToSession(rec))
	}
	return out, rows.Err()
}

// RevokeAllSessionsForUser kills every active session for a user.
// Returns the number of rows touched. Called on password change,
// account suspension, or admin-initiated forced logout. Other token
// kinds owned by the same user are deliberately left alone — they
// have their own rotation lifecycle and the issuer may want them to
// outlive the session reset.
func (r *AuthTokenRepo) RevokeAllSessionsForUser(ctx context.Context, userID, byUser, reason string, at time.Time) (int, error) {
	res, err := r.db.ExecContext(ctx, `
		UPDATE auth_tokens
		   SET revoked_at = ?, revoked_by_user = ?, revoked_reason = ?
		 WHERE kind = 'user_session'
		   AND user_id = ?
		   AND revoked_at IS NULL`,
		at.UTC(), byUser, nullableString(reason), userID)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

// Verify is the hot-path lookup the API verifier calls on every
// authenticated request. The (token_id, kind) WHERE pair is what
// guarantees a kind-mismatch in the verifier dispatch can never
// resolve to a row of the wrong kind — the SQL filter itself is the
// safety property, not Go-side branching.
//
// Reason classification (caller logs verbatim):
//
//	"success"         — token valid, *Verified populated
//	"unknown"         — no row with that (id, kind)
//	"invalid_secret"  — hash mismatch
//	"expired"         — expires_at <= now
//	"idle_expired"    — kind=user_session AND idle_expires_at <= now
//	"revoked"         — revoked_at IS NOT NULL
func (r *AuthTokenRepo) Verify(ctx context.Context, tokenID string, secret []byte, expectedKind optoken.Kind, now time.Time) (*optoken.Verified, string, error) {
	row := r.db.QueryRowContext(ctx, baseSelect+` WHERE token_id = ? AND kind = ?`, tokenID, string(expectedKind))
	rec, err := scanAuthTokenRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "unknown", nil
	}
	if err != nil {
		return nil, "", err
	}
	// Order: secret check first to keep timing equivalent across
	// success and the "row exists but wrong secret" path. Other checks
	// (revoked, expired) are fine to run after — they don't reveal
	// per-token timing.
	hashOfPresented := optoken.Hash(secret)
	if !optoken.Equal(hashOfPresented, rec.secretHash) {
		return nil, "invalid_secret", nil
	}
	if rec.revokedAt.Valid {
		return nil, "revoked", nil
	}
	if !rec.expiresAt.After(now) {
		return nil, "expired", nil
	}
	if expectedKind == optoken.KindUserSession {
		if !rec.idleExpiresAt.Valid || !rec.idleExpiresAt.Time.After(now) {
			return nil, "idle_expired", nil
		}
	}
	return rowToVerified(rec), "success", nil
}

// Revoke marks a token revoked. Idempotent: re-revoking a revoked
// token is a no-op (no error, original revoked metadata preserved).
// Returns ErrNotFound if no row matches the id at all.
func (r *AuthTokenRepo) Revoke(ctx context.Context, tokenID, byUser, reason string, at time.Time) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE auth_tokens
		   SET revoked_at = ?, revoked_by_user = ?, revoked_reason = ?
		 WHERE token_id = ? AND revoked_at IS NULL`,
		at.UTC(), byUser, nullableString(reason), tokenID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		// Either the row doesn't exist or it was already revoked. The
		// existence check distinguishes 404 from "already revoked".
		var exists int
		err := r.db.QueryRowContext(ctx,
			`SELECT 1 FROM auth_tokens WHERE token_id = ? LIMIT 1`, tokenID).Scan(&exists)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		// Row exists and was already revoked — caller treats as success.
	}
	return nil
}

// TouchLastUsed records the latest authenticated use of a token. Best
// effort — callers run this asynchronously off the request hot path.
// idleExpiresAt is honored only for user_session rows (the CHECK
// constraint already enforces it must stay NULL for other kinds);
// pass nil for non-session touches.
func (r *AuthTokenRepo) TouchLastUsed(ctx context.Context, tokenID, ip, ua string, idleExpiresAt *time.Time, at time.Time) error {
	if idleExpiresAt != nil {
		_, err := r.db.ExecContext(ctx, `
			UPDATE auth_tokens
			   SET last_used_at = ?, last_used_ip = ?, user_agent = ?, idle_expires_at = ?
			 WHERE token_id = ?`,
			at.UTC(), nullableString(ip), nullableString(ua), idleExpiresAt.UTC(), tokenID)
		return err
	}
	_, err := r.db.ExecContext(ctx, `
		UPDATE auth_tokens
		   SET last_used_at = ?, last_used_ip = ?, user_agent = ?
		 WHERE token_id = ?`,
		at.UTC(), nullableString(ip), nullableString(ua), tokenID)
	return err
}

// baseSelect is shared by Get / Verify / List paths so column order
// stays in lockstep across queries — scanAuthTokenRow assumes this
// exact ordering.
const baseSelect = `
	SELECT token_id, kind, secret_hash, user_id,
	       name, description,
	       created_at, expires_at,
	       last_used_at, last_used_ip, user_agent,
	       revoked_at, revoked_by_user, revoked_reason,
	       project_id, role, scopes,
	       idle_expires_at
	  FROM auth_tokens`

// authTokenRow is the unified internal row. Typed views (UserSession
// today, future scoped tokens tomorrow) are built from it. Private —
// the layer above this (api) deals only with typed views.
type authTokenRow struct {
	tokenID                       string
	kind                          string
	secretHash                    []byte
	userID                        string
	name                          sql.NullString
	description                   sql.NullString
	createdAt                     time.Time
	expiresAt                     time.Time
	lastUsedAt                    sql.NullTime
	lastUsedIP                    sql.NullString
	userAgent                     sql.NullString
	revokedAt                     sql.NullTime
	revokedByUser                 sql.NullString
	revokedReason                 sql.NullString
	projectID, roleStr, scopesStr sql.NullString
	idleExpiresAt                 sql.NullTime
}

func scanAuthTokenRow(s rowScanner) (*authTokenRow, error) {
	var r authTokenRow
	err := s.Scan(
		&r.tokenID, &r.kind, &r.secretHash, &r.userID,
		&r.name, &r.description,
		&r.createdAt, &r.expiresAt,
		&r.lastUsedAt, &r.lastUsedIP, &r.userAgent,
		&r.revokedAt, &r.revokedByUser, &r.revokedReason,
		&r.projectID, &r.roleStr, &r.scopesStr,
		&r.idleExpiresAt,
	)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func rowToSession(r *authTokenRow) *UserSession {
	s := &UserSession{
		TokenID:       r.tokenID,
		SecretHash:    r.secretHash,
		UserID:        r.userID,
		UserAgent:     r.userAgent.String,
		CreatedAt:     r.createdAt,
		ExpiresAt:     r.expiresAt,
		LastUsedIP:    r.lastUsedIP.String,
		Revoked:       r.revokedAt.Valid,
		RevokedByUser: r.revokedByUser.String,
		RevokedReason: r.revokedReason.String,
	}
	if r.idleExpiresAt.Valid {
		s.IdleExpiresAt = r.idleExpiresAt.Time
	}
	if r.lastUsedAt.Valid {
		t := r.lastUsedAt.Time
		s.LastUsedAt = &t
	}
	if r.revokedAt.Valid {
		t := r.revokedAt.Time
		s.RevokedAt = &t
	}
	return s
}

// rowToVerified projects an authTokenRow into the cache-ready
// optoken.Verified shape. The verifier in api uses the result both as
// a Principal source and as a cache value.
func rowToVerified(r *authTokenRow) *optoken.Verified {
	v := &optoken.Verified{
		TokenID:   r.tokenID,
		Kind:      optoken.Kind(r.kind),
		Hash:      r.secretHash,
		UserID:    r.userID,
		Role:      user.Role(r.roleStr.String),
		Scopes:    optoken.ParseList(r.scopesStr.String),
		ProjectID: r.projectID.String,
		ExpiresAt: r.expiresAt,
	}
	if r.idleExpiresAt.Valid {
		v.IdleExpiresAt = r.idleExpiresAt.Time
	}
	return v
}
