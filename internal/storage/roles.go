package storage

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// Role names a permission set. The slug is the wire identifier (kept
// in users.role / project_members.role); name + description are
// human-facing. Permissions is populated by Get and Update; List
// leaves it nil for cheapness.
type Role struct {
	Slug        string
	Name        string
	Description string
	IsBuiltin   bool
	IsGlobal    bool
	IsProject   bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Permissions []string
}

// RoleFilter narrows List by slot affinity. Both fields nil = no
// filter (all roles). Pass &true for IsGlobal to ask "give me roles
// that can be assigned to users.role".
type RoleFilter struct {
	IsGlobal  *bool
	IsProject *bool
}

func (db *DB) Roles() *RoleRepo { return &RoleRepo{db: db.DB} }

type RoleRepo struct {
	db *sql.DB
}

// List returns roles matching the filter, ordered by slug. Permissions
// are NOT populated — callers that need them call Get on the rows
// they care about. Avoids the N+1 flatten-then-group cost when an
// admin UI just wants the role list.
func (r *RoleRepo) List(ctx context.Context, f RoleFilter) ([]*Role, error) {
	q := `SELECT slug, name, description, is_builtin, is_global, is_project, created_at, updated_at
	        FROM roles`
	var args []any
	var clauses []string
	if f.IsGlobal != nil {
		clauses = append(clauses, "is_global = ?")
		args = append(args, boolToInt(*f.IsGlobal))
	}
	if f.IsProject != nil {
		clauses = append(clauses, "is_project = ?")
		args = append(args, boolToInt(*f.IsProject))
	}
	if len(clauses) > 0 {
		q += " WHERE " + strings.Join(clauses, " AND ")
	}
	q += " ORDER BY slug"
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []*Role
	for rows.Next() {
		role, err := scanRole(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, role)
	}
	return out, rows.Err()
}

// Get returns a single role with its permissions populated. ErrNotFound
// for missing.
func (r *RoleRepo) Get(ctx context.Context, slug string) (*Role, error) {
	role, err := scanRole(r.db.QueryRowContext(ctx, `
		SELECT slug, name, description, is_builtin, is_global, is_project, created_at, updated_at
		  FROM roles WHERE slug = ?`, slug))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	perms, err := r.listPermissions(ctx, slug)
	if err != nil {
		return nil, err
	}
	role.Permissions = perms
	return role, nil
}

// HasPermission is the hot-path check the RBAC middleware will use.
// Implemented as a single composite-key lookup against the role's
// primary index.
func (r *RoleRepo) HasPermission(ctx context.Context, roleSlug, permSlug string) (bool, error) {
	var one int
	err := r.db.QueryRowContext(ctx, `
		SELECT 1 FROM role_permissions
		 WHERE role_slug = ? AND perm_slug = ? LIMIT 1`,
		roleSlug, permSlug,
	).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// Create inserts a custom role and the role_permissions rows for its
// initial permission set. Atomic: a partial seed (role row inserted
// but permissions failed to attach) is impossible.
func (r *RoleRepo) Create(ctx context.Context, role *Role, perms []string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	rollback := true
	defer func() {
		if rollback {
			_ = tx.Rollback()
		}
	}()

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO roles (slug, name, description, is_builtin, is_global, is_project, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		role.Slug, role.Name, nullableString(role.Description),
		boolToInt(role.IsBuiltin), boolToInt(role.IsGlobal), boolToInt(role.IsProject),
		role.CreatedAt.UTC(), role.UpdatedAt.UTC(),
	); err != nil {
		return err
	}

	for _, p := range perms {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO role_permissions (role_slug, perm_slug) VALUES (?, ?)`,
			role.Slug, p,
		); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	rollback = false
	return nil
}

// Update replaces a role's mutable fields and its full permission set.
// Atomic: the row's name/description and its permissions move
// together. The DB-side trigger that protects the admin role from
// losing admin:* permissions fires inside this transaction; if it
// does, the entire Update aborts with no partial change applied.
func (r *RoleRepo) Update(ctx context.Context, role *Role, perms []string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	rollback := true
	defer func() {
		if rollback {
			_ = tx.Rollback()
		}
	}()

	if _, err := tx.ExecContext(ctx, `
		UPDATE roles
		   SET name = ?, description = ?, is_global = ?, is_project = ?, updated_at = ?
		 WHERE slug = ?`,
		role.Name, nullableString(role.Description),
		boolToInt(role.IsGlobal), boolToInt(role.IsProject),
		time.Now().UTC(), role.Slug,
	); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM role_permissions WHERE role_slug = ?`, role.Slug,
	); err != nil {
		return err
	}
	for _, p := range perms {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO role_permissions (role_slug, perm_slug) VALUES (?, ?)`,
			role.Slug, p,
		); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	rollback = false
	return nil
}

// Delete removes a role. Refuses on builtin (ErrRoleBuiltin) or when
// any user / project_member still references it (ErrRoleInUse). The
// in-use check is read-then-delete in a single tx so a concurrent
// user.role assignment can't slip in between.
func (r *RoleRepo) Delete(ctx context.Context, slug string) error {
	role, err := r.Get(ctx, slug)
	if err != nil {
		return err
	}
	if role.IsBuiltin {
		return ErrRoleBuiltin
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	rollback := true
	defer func() {
		if rollback {
			_ = tx.Rollback()
		}
	}()

	var inUse int
	if err := tx.QueryRowContext(ctx, `
		SELECT
		  (SELECT COUNT(*) FROM users WHERE role = ?) +
		  (SELECT COUNT(*) FROM project_members WHERE role = ?)`,
		slug, slug,
	).Scan(&inUse); err != nil {
		return err
	}
	if inUse > 0 {
		return ErrRoleInUse
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM roles WHERE slug = ?`, slug); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	rollback = false
	return nil
}

func (r *RoleRepo) listPermissions(ctx context.Context, roleSlug string) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT perm_slug FROM role_permissions
		 WHERE role_slug = ? ORDER BY perm_slug`, roleSlug)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func scanRole(s rowScanner) (*Role, error) {
	var (
		role                                Role
		desc                                sql.NullString
		isBuiltin, isGlobal, isProject      int
	)
	err := s.Scan(
		&role.Slug, &role.Name, &desc,
		&isBuiltin, &isGlobal, &isProject,
		&role.CreatedAt, &role.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	role.Description = desc.String
	role.IsBuiltin = isBuiltin == 1
	role.IsGlobal = isGlobal == 1
	role.IsProject = isProject == 1
	return &role, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
