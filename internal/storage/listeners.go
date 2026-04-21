package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Listener mirrors a row in the listeners table. It is the persistence
// half of the in-memory core.TCPServer — the server opens the port,
// this row survives restarts so the UI can re-materialise the ingress
// after the binary is bounced.
type Listener struct {
	ID             string
	ProjectID      string
	Host           string
	Port           uint16
	PublicIP       string
	ShellPath      string
	DisableHistory bool
	GroupDispatch  bool
	CreatedAt      time.Time
}

func (db *DB) Listeners() *ListenerRepo { return &ListenerRepo{db: db.DB} }

type ListenerRepo struct{ db *sql.DB }

func (r *ListenerRepo) Create(ctx context.Context, l *Listener) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO listeners
		  (id, project_id, host, port, public_ip, shell_path,
		   disable_history, group_dispatch, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		l.ID, l.ProjectID, l.Host, l.Port, l.PublicIP, l.ShellPath,
		l.DisableHistory, l.GroupDispatch, l.CreatedAt.UTC())
	return err
}

func (r *ListenerRepo) GetByID(ctx context.Context, id string) (*Listener, error) {
	l, err := scanListener(r.db.QueryRowContext(ctx, `
		SELECT id, project_id, host, port, public_ip, shell_path,
		       disable_history, group_dispatch, created_at
		  FROM listeners WHERE id = ?`, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return l, nil
}

func (r *ListenerRepo) ListByProject(ctx context.Context, projectID string) ([]*Listener, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, project_id, host, port, public_ip, shell_path,
		       disable_history, group_dispatch, created_at
		  FROM listeners WHERE project_id = ?
		 ORDER BY port ASC`, projectID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := []*Listener{}
	for rows.Next() {
		l, err := scanListener(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (r *ListenerRepo) List(ctx context.Context) ([]*Listener, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, project_id, host, port, public_ip, shell_path,
		       disable_history, group_dispatch, created_at
		  FROM listeners ORDER BY project_id, port ASC`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := []*Listener{}
	for rows.Next() {
		l, err := scanListener(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (r *ListenerRepo) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM listeners WHERE id = ?`, id)
	if err != nil {
		return err
	}
	return expectOneRow(res)
}

func scanListener(s rowScanner) (*Listener, error) {
	var (
		l        Listener
		publicIP sql.NullString
		shell    sql.NullString
	)
	err := s.Scan(
		&l.ID, &l.ProjectID, &l.Host, &l.Port, &publicIP, &shell,
		&l.DisableHistory, &l.GroupDispatch, &l.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	if publicIP.Valid {
		l.PublicIP = publicIP.String
	}
	if shell.Valid {
		l.ShellPath = shell.String
	}
	return &l, nil
}
