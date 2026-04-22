package storage

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// AgentSession represents one generation of an agent's session_token.
// Each rotation creates a new row and sets rotated_at on the previous one;
// revocation sets revoked_at. A session is "active" when both are NULL
// and expires_at is in the future.
type AgentSession struct {
	SessionID        string
	AgentID          string
	ProjectID        string
	SessionTokenHash []byte
	IssuedAt         time.Time
	IssuedReason     string // "enroll" | "rotation" | "reset"
	RotatedFrom      string
	ExpiresAt        time.Time
	RotatedAt        *time.Time
	RevokedAt        *time.Time
	RevokedReason    string
	RevokedByUser    string
	LastSeenAt       *time.Time
	LastSeenIP       string
	MachineID        string
}

// IsActive reports whether this particular row is the currently-live
// session for its agent. Pass a wall clock so tests can control expiry.
func (s *AgentSession) IsActive(now time.Time) bool {
	return s.RotatedAt == nil && s.RevokedAt == nil && s.ExpiresAt.After(now)
}

// ErrSessionAlreadyActive is returned by InsertActive when the caller tries
// to insert a new active session while one already exists for the agent.
// Callers who want to rotate should use RotateTo instead; callers who
// want to reset should revoke the old session first.
var ErrSessionAlreadyActive = errors.New("storage: agent already has an active session")

func (db *DB) AgentSessions() *AgentSessionRepo {
	return &AgentSessionRepo{db: db.DB}
}

type AgentSessionRepo struct {
	db *sql.DB
}

// InsertActive inserts a brand-new active session. Fails with
// ErrSessionAlreadyActive if the partial UNIQUE index in the schema
// rejects it (i.e. there's already an active row for this agent).
func (r *AgentSessionRepo) InsertActive(ctx context.Context, s *AgentSession) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO agent_sessions (
			session_id, agent_id, project_id, session_token_hash,
			issued_at, issued_reason, rotated_from, expires_at,
			rotated_at, revoked_at, revoked_reason, revoked_by_user,
			last_seen_at, last_seen_ip, machine_id
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL, NULL, NULL, NULL, NULL, ?)`,
		s.SessionID, s.AgentID, s.ProjectID, s.SessionTokenHash,
		s.IssuedAt.UTC(), s.IssuedReason, nullableString(s.RotatedFrom),
		s.ExpiresAt.UTC(), nullableString(s.MachineID),
	)
	if isUniqueViolation(err) {
		return ErrSessionAlreadyActive
	}
	return err
}

// RotateTo atomically marks the caller-supplied oldSessionID as rotated
// and inserts a new active row. Runs in a transaction so no brief window
// exists in which two rows are active for the same agent.
func (r *AgentSessionRepo) RotateTo(ctx context.Context, oldSessionID string, next *AgentSession, at time.Time) error {
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

	// Mark old rotated. If it's already rotated/revoked the UPDATE will
	// be a no-op — we detect that via RowsAffected so we don't silently
	// create a fresh active row on top of a dead predecessor.
	res, err := tx.ExecContext(ctx, `
		UPDATE agent_sessions
		   SET rotated_at = ?
		 WHERE session_id = ? AND rotated_at IS NULL AND revoked_at IS NULL`,
		at.UTC(), oldSessionID)
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

	next.RotatedFrom = oldSessionID
	_, err = tx.ExecContext(ctx, `
		INSERT INTO agent_sessions (
			session_id, agent_id, project_id, session_token_hash,
			issued_at, issued_reason, rotated_from, expires_at,
			rotated_at, revoked_at, revoked_reason, revoked_by_user,
			last_seen_at, last_seen_ip, machine_id
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL, NULL, NULL, NULL, NULL, ?)`,
		next.SessionID, next.AgentID, next.ProjectID, next.SessionTokenHash,
		next.IssuedAt.UTC(), next.IssuedReason, next.RotatedFrom,
		next.ExpiresAt.UTC(), nullableString(next.MachineID),
	)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrSessionAlreadyActive
		}
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	rollback = false
	return nil
}

// GetActive returns the currently-active session for an agent, or
// ErrNotFound.
func (r *AgentSessionRepo) GetActive(ctx context.Context, agentID string) (*AgentSession, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT session_id, agent_id, project_id, session_token_hash,
		       issued_at, issued_reason, rotated_from, expires_at,
		       rotated_at, revoked_at, revoked_reason, revoked_by_user,
		       last_seen_at, last_seen_ip, machine_id
		  FROM agent_sessions
		 WHERE agent_id = ? AND rotated_at IS NULL AND revoked_at IS NULL`,
		agentID)
	return scanAgentSessionSingle(row)
}

// GetBySessionID looks up a specific session generation.
func (r *AgentSessionRepo) GetBySessionID(ctx context.Context, sessionID string) (*AgentSession, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT session_id, agent_id, project_id, session_token_hash,
		       issued_at, issued_reason, rotated_from, expires_at,
		       rotated_at, revoked_at, revoked_reason, revoked_by_user,
		       last_seen_at, last_seen_ip, machine_id
		  FROM agent_sessions WHERE session_id = ?`, sessionID)
	return scanAgentSessionSingle(row)
}

// RevokeActive kills whatever session is currently active for the given
// agent. Returns ErrNotFound if there isn't one.
func (r *AgentSessionRepo) RevokeActive(ctx context.Context, agentID, byUser, reason string, at time.Time) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE agent_sessions
		   SET revoked_at = ?, revoked_by_user = ?, revoked_reason = ?
		 WHERE agent_id = ? AND rotated_at IS NULL AND revoked_at IS NULL`,
		at.UTC(), byUser, nullableString(reason), agentID)
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

// TouchLastSeen updates the convenience columns on the currently-active
// session. last_seen_at is allowed to be overwritten — the append-only
// authoritative record lives in agent_connection_events.
func (r *AgentSessionRepo) TouchLastSeen(ctx context.Context, sessionID, clientIP string, at time.Time) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE agent_sessions
		   SET last_seen_at = ?, last_seen_ip = ?
		 WHERE session_id = ?`,
		at.UTC(), nullableString(clientIP), sessionID)
	return err
}

// History returns every session row for an agent, newest first. Used by
// the admin UI audit view and by the /audit/export endpoint.
func (r *AgentSessionRepo) History(ctx context.Context, agentID string) ([]*AgentSession, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT session_id, agent_id, project_id, session_token_hash,
		       issued_at, issued_reason, rotated_from, expires_at,
		       rotated_at, revoked_at, revoked_reason, revoked_by_user,
		       last_seen_at, last_seen_ip, machine_id
		  FROM agent_sessions WHERE agent_id = ?
		  ORDER BY issued_at DESC`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*AgentSession
	for rows.Next() {
		s, err := scanAgentSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func scanAgentSessionSingle(row rowScanner) (*AgentSession, error) {
	s, err := scanAgentSession(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return s, err
}

func scanAgentSession(row rowScanner) (*AgentSession, error) {
	var (
		s         AgentSession
		rotFrom   sql.NullString
		rotAt     sql.NullTime
		revAt     sql.NullTime
		revReason sql.NullString
		revBy     sql.NullString
		lastAt    sql.NullTime
		lastIP    sql.NullString
		machineID sql.NullString
	)
	err := row.Scan(
		&s.SessionID, &s.AgentID, &s.ProjectID, &s.SessionTokenHash,
		&s.IssuedAt, &s.IssuedReason, &rotFrom, &s.ExpiresAt,
		&rotAt, &revAt, &revReason, &revBy,
		&lastAt, &lastIP, &machineID,
	)
	if err != nil {
		return nil, err
	}
	s.RotatedFrom = rotFrom.String
	if rotAt.Valid {
		t := rotAt.Time
		s.RotatedAt = &t
	}
	if revAt.Valid {
		t := revAt.Time
		s.RevokedAt = &t
	}
	s.RevokedReason = revReason.String
	s.RevokedByUser = revBy.String
	if lastAt.Valid {
		t := lastAt.Time
		s.LastSeenAt = &t
	}
	s.LastSeenIP = lastIP.String
	s.MachineID = machineID.String
	return &s, nil
}

// isUniqueViolation recognises SQLite's "UNIQUE constraint failed" error
// text. We have a similar helper over in internal/api, but importing
// there would create a package cycle; keep a private copy.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
}
