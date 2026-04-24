package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Session mirrors a row in the sessions table. Every agent connection
// creates one row with disconnected_at=NULL; the disconnect handler
// stamps the timestamp. The row stays forever so the UI can show the
// full connection history per host.
type Session struct {
	ID             string
	ProjectID      string
	IngressAddr    string
	HostID         string
	Alias          string
	User           string
	RemoteAddr     string
	Version        string
	Python2        string
	Python3        string
	InterfacesJSON string
	GroupDispatch  bool
	ConnectedAt    time.Time
	DisconnectedAt *time.Time
}

func (db *DB) Sessions() *SessionRepo { return &SessionRepo{db: db.DB} }

type SessionRepo struct{ db *sql.DB }

func (r *SessionRepo) Insert(ctx context.Context, s *Session) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO sessions
		  (id, project_id, ingress_addr, host_id, alias, user, remote_addr,
		   version, python2, python3, interfaces_json, group_dispatch,
		   connected_at, disconnected_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL)`,
		s.ID, s.ProjectID, s.IngressAddr, s.HostID,
		nullIfEmpty(s.Alias), nullIfEmpty(s.User), nullIfEmpty(s.RemoteAddr),
		nullIfEmpty(s.Version), nullIfEmpty(s.Python2), nullIfEmpty(s.Python3),
		nullIfEmpty(s.InterfacesJSON), s.GroupDispatch,
		s.ConnectedAt.UTC(),
	)
	return err
}

func (r *SessionRepo) Get(ctx context.Context, id string) (*Session, error) {
	return r.queryOne(ctx, `WHERE id = ?`, id)
}

func (r *SessionRepo) ListForHost(ctx context.Context, hostID string) ([]*Session, error) {
	return r.queryMany(ctx, `WHERE host_id = ? ORDER BY connected_at DESC`, hostID)
}

func (r *SessionRepo) ListLiveForProject(ctx context.Context, projectID string) ([]*Session, error) {
	return r.queryMany(ctx,
		`WHERE project_id = ? AND disconnected_at IS NULL ORDER BY connected_at DESC`,
		projectID)
}

// SessionListOpts narrows ListForProject. Zero-value (all fields nil/0)
// returns every session for the project, newest first.
type SessionListOpts struct {
	// Live, if non-nil, filters to live (true) or closed-only (false)
	// sessions. Skipped when nil.
	Live *bool
	// Since, if non-nil, restricts to sessions whose connected_at is at
	// or after the given timestamp. Used by the dashboard for "last 24h"
	// time-series.
	Since *time.Time
	// Limit caps the result count; 0 means unbounded.
	Limit int
}

// ListForProject returns sessions in the project, newest first, with
// optional live + since + limit filters. Powers the SessionsPage list
// view and the dashboard time-series chart.
func (r *SessionRepo) ListForProject(ctx context.Context, projectID string, opts SessionListOpts) ([]*Session, error) {
	where := `WHERE project_id = ?`
	args := []any{projectID}
	if opts.Live != nil {
		if *opts.Live {
			where += ` AND disconnected_at IS NULL`
		} else {
			where += ` AND disconnected_at IS NOT NULL`
		}
	}
	if opts.Since != nil {
		where += ` AND connected_at >= ?`
		args = append(args, opts.Since.UTC())
	}
	where += ` ORDER BY connected_at DESC`
	if opts.Limit > 0 {
		where += ` LIMIT ?`
		args = append(args, opts.Limit)
	}
	return r.queryMany(ctx, where, args...)
}

// MarkDisconnected stamps disconnected_at on the row if it's currently
// NULL. Idempotent — a second call is a no-op.
func (r *SessionRepo) MarkDisconnected(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE sessions SET disconnected_at = ?
		  WHERE id = ? AND disconnected_at IS NULL`,
		time.Now().UTC(), id)
	return err
}

// SetGroupDispatch flips the group_dispatch flag on a session row. The
// runtime object in core keeps its own mirror; this persists the choice
// so it survives restart.
func (r *SessionRepo) SetGroupDispatch(ctx context.Context, id string, on bool) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE sessions SET group_dispatch = ? WHERE id = ?`, on, id)
	if err != nil {
		return err
	}
	return expectOneRow(res)
}

const sessionSelect = `
	SELECT id, project_id, ingress_addr, host_id, alias, user, remote_addr,
	       version, python2, python3, interfaces_json, group_dispatch,
	       connected_at, disconnected_at
	  FROM sessions`

func (r *SessionRepo) queryOne(ctx context.Context, where string, args ...any) (*Session, error) {
	row := r.db.QueryRowContext(ctx, sessionSelect+" "+where, args...)
	s, err := scanSession(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return s, nil
}

func (r *SessionRepo) queryMany(ctx context.Context, where string, args ...any) ([]*Session, error) {
	rows, err := r.db.QueryContext(ctx, sessionSelect+" "+where, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := []*Session{}
	for rows.Next() {
		s, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func scanSession(s rowScanner) (*Session, error) {
	var (
		row                                              Session
		alias, userS, remote, version, py2, py3, ifacesJ sql.NullString
		disc                                             sql.NullTime
	)
	err := s.Scan(&row.ID, &row.ProjectID, &row.IngressAddr, &row.HostID,
		&alias, &userS, &remote, &version, &py2, &py3, &ifacesJ,
		&row.GroupDispatch, &row.ConnectedAt, &disc)
	if err != nil {
		return nil, err
	}
	row.Alias = alias.String
	row.User = userS.String
	row.RemoteAddr = remote.String
	row.Version = version.String
	row.Python2 = py2.String
	row.Python3 = py3.String
	row.InterfacesJSON = ifacesJ.String
	if disc.Valid {
		t := disc.Time
		row.DisconnectedAt = &t
	}
	return &row, nil
}
