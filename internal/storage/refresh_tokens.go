package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// RefreshToken mirrors a row in the refresh_tokens table. The JWT itself is
// not stored — only its jti (ID). On refresh the handler looks up the row by
// jti, checks RevokedAt is nil and now < ExpiresAt, and (if rotating) creates
// a replacement before revoking the old row.
type RefreshToken struct {
	ID        string
	UserID    string
	ExpiresAt time.Time
	RevokedAt *time.Time
}

func (db *DB) RefreshTokens() *RefreshTokenRepo {
	return &RefreshTokenRepo{db: db.DB}
}

type RefreshTokenRepo struct {
	db *sql.DB
}

func (r *RefreshTokenRepo) Create(ctx context.Context, rt *RefreshToken) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO refresh_tokens (id, user_id, expires_at, revoked_at)
		VALUES (?, ?, ?, NULL)`,
		rt.ID, rt.UserID, rt.ExpiresAt.UTC())
	return err
}

func (r *RefreshTokenRepo) Get(ctx context.Context, id string) (*RefreshToken, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, user_id, expires_at, revoked_at FROM refresh_tokens WHERE id = ?`, id)
	var (
		rt      RefreshToken
		revoked sql.NullTime
	)
	err := row.Scan(&rt.ID, &rt.UserID, &rt.ExpiresAt, &revoked)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if revoked.Valid {
		t := revoked.Time
		rt.RevokedAt = &t
	}
	return &rt, nil
}

// Revoke marks a single refresh token as revoked. Idempotent for callers —
// a missing row still returns ErrNotFound so the handler can 404, but
// revoking an already-revoked token does not error.
func (r *RefreshTokenRepo) Revoke(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE refresh_tokens SET revoked_at = ? WHERE id = ?`,
		time.Now().UTC(), id)
	if err != nil {
		return err
	}
	return expectOneRow(res)
}

// RevokeAllForUser flips revoked_at on every live refresh token for a user.
// Useful on password change so all active logins are invalidated.
func (r *RefreshTokenRepo) RevokeAllForUser(ctx context.Context, userID string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE refresh_tokens SET revoked_at = ?
		  WHERE user_id = ? AND revoked_at IS NULL`,
		time.Now().UTC(), userID)
	return err
}
