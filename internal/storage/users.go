package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/WangYihang/Platypus/internal/user"
)

// Users returns a UserRepo bound to this DB. The repo is a thin method
// receiver — no state of its own — so callers can instantiate it inline.
func (db *DB) Users() *UserRepo {
	return &UserRepo{db: db.DB}
}

// UserRepo encapsulates row-level access to the users table.
type UserRepo struct {
	db *sql.DB
}

func (r *UserRepo) Create(ctx context.Context, u *user.User) error {
	var lastLogin any
	if u.LastLoginAt != nil {
		lastLogin = u.LastLoginAt.UTC()
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO users (id, username, password_hash, role, created_at, last_login_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		u.ID, u.Username, u.PasswordHash, string(u.Role), u.CreatedAt.UTC(), lastLogin)
	return err
}

func (r *UserRepo) GetByID(ctx context.Context, id string) (*user.User, error) {
	return r.queryOne(ctx,
		`SELECT id, username, password_hash, role, created_at, last_login_at
		   FROM users WHERE id = ?`, id)
}

func (r *UserRepo) GetByUsername(ctx context.Context, username string) (*user.User, error) {
	return r.queryOne(ctx,
		`SELECT id, username, password_hash, role, created_at, last_login_at
		   FROM users WHERE username = ?`, username)
}

// List returns human users — the "system" pseudo-user seeded at
// startup for FK targets is always filtered out so UI listings and
// bootstrap-gate checks don't have to know about the implementation
// detail. If a caller genuinely needs the row, use GetByID(SystemUserID).
func (r *UserRepo) List(ctx context.Context) ([]*user.User, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, username, password_hash, role, created_at, last_login_at
		   FROM users WHERE id != ? ORDER BY username ASC`, SystemUserID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := []*user.User{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// Count returns the number of human users, matching List's filter.
// The bootstrap endpoint uses this to decide whether the admin account
// has been created yet — without the filter the seeded system user
// would make it look already-bootstrapped on a fresh install.
func (r *UserRepo) Count(ctx context.Context) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM users WHERE id != ?`, SystemUserID).Scan(&n)
	return n, err
}

func (r *UserRepo) UpdatePasswordHash(ctx context.Context, id, hash string) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE users SET password_hash = ? WHERE id = ?`, hash, id)
	if err != nil {
		return err
	}
	return expectOneRow(res)
}

func (r *UserRepo) UpdateRole(ctx context.Context, id string, role user.Role) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE users SET role = ? WHERE id = ?`, string(role), id)
	if err != nil {
		return err
	}
	return expectOneRow(res)
}

func (r *UserRepo) TouchLastLogin(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE users SET last_login_at = ? WHERE id = ?`, time.Now().UTC(), id)
	if err != nil {
		return err
	}
	return expectOneRow(res)
}

func (r *UserRepo) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id)
	if err != nil {
		return err
	}
	return expectOneRow(res)
}

// rowScanner is satisfied by both *sql.Row and *sql.Rows so scanUser can be
// reused across singleton and list queries.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanUser(s rowScanner) (*user.User, error) {
	var (
		u         user.User
		role      string
		lastLogin sql.NullTime
	)
	err := s.Scan(&u.ID, &u.Username, &u.PasswordHash, &role, &u.CreatedAt, &lastLogin)
	if err != nil {
		return nil, err
	}
	u.Role = user.Role(role)
	if lastLogin.Valid {
		t := lastLogin.Time
		u.LastLoginAt = &t
	}
	return &u, nil
}

func (r *UserRepo) queryOne(ctx context.Context, q string, args ...any) (*user.User, error) {
	u, err := scanUser(r.db.QueryRowContext(ctx, q, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}

func expectOneRow(res sql.Result) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
