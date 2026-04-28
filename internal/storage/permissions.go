package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Permission is one row in the permissions table — the catalogue of
// fine-grained capabilities that can be attached to a role.
type Permission struct {
	Slug        string
	Resource    string // UI grouping ("hosts" / "files" / "admin"); not part of any access decision
	Description string
	CreatedAt   time.Time
}

func (db *DB) Permissions() *PermissionRepo { return &PermissionRepo{db: db.DB} }

type PermissionRepo struct {
	db *sql.DB
}

// List returns the catalogue ordered by (resource, slug) so the admin
// UI can render grouped sections without an in-app sort.
func (r *PermissionRepo) List(ctx context.Context) ([]*Permission, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT slug, resource, description, created_at
		  FROM permissions
		 ORDER BY resource, slug`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []*Permission
	for rows.Next() {
		p := &Permission{}
		if err := rows.Scan(&p.Slug, &p.Resource, &p.Description, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Get fetches a single permission by slug. ErrNotFound for missing.
func (r *PermissionRepo) Get(ctx context.Context, slug string) (*Permission, error) {
	p := &Permission{}
	err := r.db.QueryRowContext(ctx, `
		SELECT slug, resource, description, created_at
		  FROM permissions WHERE slug = ?`, slug,
	).Scan(&p.Slug, &p.Resource, &p.Description, &p.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return p, nil
}
