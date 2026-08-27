package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/WangYihang/Platypus/internal/user"
)

// Project mirrors a row in the projects table. Slug is the url-safe,
// unique identifier surfaced in REST routes (/api/v1/projects/:slug),
// Name is the human-readable label shown in the UI.
type Project struct {
	ID        string
	Name      string
	Slug      string
	CreatedAt time.Time
	CreatedBy string

	// AISummariesEnabled gates the LLM round-trip that summarises a
	// finished terminal recording (internal/recording.Session.Finish).
	// Off by default and only flipped by an explicit call to
	// SetAISummariesEnabled: cast files can contain pasted secrets, so
	// they never leave the deployment without per-project consent.
	AISummariesEnabled bool
}

func (db *DB) Projects() *ProjectRepo {
	return &ProjectRepo{db: db.DB}
}

type ProjectRepo struct {
	db *sql.DB
}

func (r *ProjectRepo) Create(ctx context.Context, p *Project) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO projects (id, name, slug, created_at, created_by)
		VALUES (?, ?, ?, ?, ?)`,
		p.ID, p.Name, p.Slug, p.CreatedAt.UTC(), p.CreatedBy)
	return err
}

func (r *ProjectRepo) GetByID(ctx context.Context, id string) (*Project, error) {
	return r.queryOne(ctx, `
		SELECT id, name, slug, created_at, created_by, ai_summaries_enabled
		  FROM projects WHERE id = ?`, id)
}

func (r *ProjectRepo) GetBySlug(ctx context.Context, slug string) (*Project, error) {
	return r.queryOne(ctx, `
		SELECT id, name, slug, created_at, created_by, ai_summaries_enabled
		  FROM projects WHERE slug = ?`, slug)
}

func (r *ProjectRepo) List(ctx context.Context) ([]*Project, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, slug, created_at, created_by, ai_summaries_enabled
		  FROM projects ORDER BY slug ASC`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := []*Project{}
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ListForUser returns the projects the given user may view. A global
// admin sees every project; anyone else sees only the projects they hold
// a project_members row in.
func (r *ProjectRepo) ListForUser(ctx context.Context, userID string, globalRole user.Role) ([]*Project, error) {
	if globalRole == user.RoleAdmin {
		return r.List(ctx)
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT p.id, p.name, p.slug, p.created_at, p.created_by, p.ai_summaries_enabled
		  FROM projects p
		  JOIN project_members m ON m.project_id = p.id
		 WHERE m.user_id = ?
		 ORDER BY p.slug ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := []*Project{}
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *ProjectRepo) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM projects WHERE id = ?`, id)
	if err != nil {
		return err
	}
	return expectOneRow(res)
}

// AddMember upserts a project_members row. Re-calling it with a new role
// overwrites the previous role — keeps the HTTP layer idempotent.
func (r *ProjectRepo) AddMember(ctx context.Context, projectID, userID string, role user.Role) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO project_members (project_id, user_id, role)
		VALUES (?, ?, ?)
		ON CONFLICT (project_id, user_id) DO UPDATE SET role = excluded.role`,
		projectID, userID, string(role))
	return err
}

func (r *ProjectRepo) RemoveMember(ctx context.Context, projectID, userID string) error {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM project_members WHERE project_id = ? AND user_id = ?`,
		projectID, userID)
	if err != nil {
		return err
	}
	return expectOneRow(res)
}

func (r *ProjectRepo) MemberRole(ctx context.Context, projectID, userID string) (user.Role, error) {
	var role string
	err := r.db.QueryRowContext(ctx,
		`SELECT role FROM project_members WHERE project_id = ? AND user_id = ?`,
		projectID, userID).Scan(&role)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return user.Role(role), nil
}

// ProjectMember is the shape returned by ListMembers.
type ProjectMember struct {
	UserID   string
	Username string
	Role     user.Role
}

// ListMembers returns every member of a project joined with the user row
// so the UI can render "alice (operator)" without a second lookup.
func (r *ProjectRepo) ListMembers(ctx context.Context, projectID string) ([]*ProjectMember, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT m.user_id, u.username, m.role
		  FROM project_members m
		  JOIN users u ON u.id = m.user_id
		 WHERE m.project_id = ?
		 ORDER BY u.username ASC`, projectID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := []*ProjectMember{}
	for rows.Next() {
		var (
			m    ProjectMember
			role string
		)
		if err := rows.Scan(&m.UserID, &m.Username, &role); err != nil {
			return nil, err
		}
		m.Role = user.Role(role)
		out = append(out, &m)
	}
	return out, rows.Err()
}

// SetAISummariesEnabled flips the per-project opt-in for LLM session
// summaries. Row missing → ErrNotFound, so a caller acting on a stale
// project id gets a 404 rather than silently succeeding.
func (r *ProjectRepo) SetAISummariesEnabled(ctx context.Context, projectID string, enabled bool) error {
	v := 0
	if enabled {
		v = 1
	}
	res, err := r.db.ExecContext(ctx,
		"UPDATE projects SET ai_summaries_enabled = ? WHERE id = ?", v, projectID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func scanProject(s rowScanner) (*Project, error) {
	// ai_summaries_enabled is stored as INTEGER 0/1 (the migration
	// constrains it to those two values), so scan through an int
	// rather than relying on the driver's bool coercion.
	var (
		p         Project
		aiEnabled int
	)
	err := s.Scan(&p.ID, &p.Name, &p.Slug, &p.CreatedAt, &p.CreatedBy, &aiEnabled)
	if err != nil {
		return nil, err
	}
	p.AISummariesEnabled = aiEnabled != 0
	return &p, nil
}

func (r *ProjectRepo) queryOne(ctx context.Context, q string, args ...any) (*Project, error) {
	p, err := scanProject(r.db.QueryRowContext(ctx, q, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return p, nil
}
