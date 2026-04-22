package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// --- PAT redemption events ---------------------------------------------------

// PATRedemptionEvent is one entry in pat_redemption_events. Both successful
// and failed attempts land here — the failed ones are crucial for detecting
// scanning / brute-force against token_ids.
type PATRedemptionEvent struct {
	ID          int64
	At          time.Time
	TokenID     string
	ClientIP    string
	MachineID   string
	Hostname    string
	AgentID     string
	Outcome     string // see CHECK constraint in 000003_pat_tokens.up.sql
	ErrorDetail string
}

func (db *DB) PATRedemptionEvents() *PATRedemptionEventRepo {
	return &PATRedemptionEventRepo{db: db.DB}
}

type PATRedemptionEventRepo struct {
	db *sql.DB
}

// Record appends one attempt. Never fails the outer flow on a log-write
// error — callers should log-and-drop; but for tests we still return it.
func (r *PATRedemptionEventRepo) Record(ctx context.Context, e *PATRedemptionEvent) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO pat_redemption_events (
			at, token_id, client_ip, machine_id, hostname,
			agent_id, outcome, error_detail
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		e.At.UTC(), e.TokenID,
		nullableString(e.ClientIP),
		nullableString(e.MachineID),
		nullableString(e.Hostname),
		nullableString(e.AgentID),
		e.Outcome,
		nullableString(e.ErrorDetail),
	)
	return err
}

// ListByToken returns every attempt against a given token_id, newest first.
func (r *PATRedemptionEventRepo) ListByToken(ctx context.Context, tokenID string, limit int) ([]*PATRedemptionEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, at, token_id, client_ip, machine_id, hostname,
		       agent_id, outcome, error_detail
		  FROM pat_redemption_events
		 WHERE token_id = ?
		 ORDER BY at DESC
		 LIMIT ?`, tokenID, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []*PATRedemptionEvent
	for rows.Next() {
		e, err := scanPATRedemptionEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func scanPATRedemptionEvent(row rowScanner) (*PATRedemptionEvent, error) {
	var (
		e         PATRedemptionEvent
		clientIP  sql.NullString
		machineID sql.NullString
		hostname  sql.NullString
		agentID   sql.NullString
		errDetail sql.NullString
	)
	err := row.Scan(
		&e.ID, &e.At, &e.TokenID, &clientIP, &machineID, &hostname,
		&agentID, &e.Outcome, &errDetail,
	)
	if err != nil {
		return nil, err
	}
	e.ClientIP = clientIP.String
	e.MachineID = machineID.String
	e.Hostname = hostname.String
	e.AgentID = agentID.String
	e.ErrorDetail = errDetail.String
	return &e, nil
}

// --- Agent connection events -------------------------------------------------

// AgentConnectionEvent records enroll / reconnect / disconnect activity.
// It is the authoritative audit timeline; agent_sessions.last_seen_* is a
// mutable convenience cache only.
type AgentConnectionEvent struct {
	ID        int64
	At        time.Time
	AgentID   string
	SessionID string
	ClientIP  string
	EventType string // enroll_success | enroll_reject | reconnect_success | reconnect_reject | disconnect
	Reason    string
	Transport string // "tls_direct" | "mesh" | ""
}

func (db *DB) AgentConnectionEvents() *AgentConnectionEventRepo {
	return &AgentConnectionEventRepo{db: db.DB}
}

type AgentConnectionEventRepo struct {
	db *sql.DB
}

func (r *AgentConnectionEventRepo) Record(ctx context.Context, e *AgentConnectionEvent) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO agent_connection_events (
			at, agent_id, session_id, client_ip, event_type, reason, transport
		) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		e.At.UTC(),
		nullableString(e.AgentID),
		nullableString(e.SessionID),
		nullableString(e.ClientIP),
		e.EventType,
		nullableString(e.Reason),
		nullableString(e.Transport),
	)
	return err
}

// ListByAgent returns the N most recent connection events for an agent.
func (r *AgentConnectionEventRepo) ListByAgent(ctx context.Context, agentID string, limit int) ([]*AgentConnectionEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, at, agent_id, session_id, client_ip, event_type, reason, transport
		  FROM agent_connection_events
		 WHERE agent_id = ?
		 ORDER BY at DESC
		 LIMIT ?`, agentID, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []*AgentConnectionEvent
	for rows.Next() {
		e, err := scanAgentConnectionEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func scanAgentConnectionEvent(row rowScanner) (*AgentConnectionEvent, error) {
	var (
		e         AgentConnectionEvent
		agentID   sql.NullString
		sessionID sql.NullString
		clientIP  sql.NullString
		reason    sql.NullString
		transport sql.NullString
	)
	err := row.Scan(
		&e.ID, &e.At, &agentID, &sessionID, &clientIP,
		&e.EventType, &reason, &transport,
	)
	if err != nil {
		return nil, err
	}
	e.AgentID = agentID.String
	e.SessionID = sessionID.String
	e.ClientIP = clientIP.String
	e.Reason = reason.String
	e.Transport = transport.String
	return &e, nil
}

// --- Administrator audit log -------------------------------------------------

// AdminAuditEvent captures one admin action. Stored append-only. details
// is free-form JSON (strings only — never secrets).
type AdminAuditEvent struct {
	ID         int64
	At         time.Time
	ActorUser  string
	ActorIP    string
	ActorUA    string
	Action     string // e.g. "pat.issue" | "pat.revoke" | "session.revoke"
	TargetType string
	TargetID   string
	ProjectID  string
	Details    string // JSON
	Outcome    string // "success" | "denied" | "error"
	Error      string
}

func (db *DB) AdminAuditLog() *AdminAuditLogRepo {
	return &AdminAuditLogRepo{db: db.DB}
}

type AdminAuditLogRepo struct {
	db *sql.DB
}

func (r *AdminAuditLogRepo) Record(ctx context.Context, e *AdminAuditEvent) error {
	if e.ActorUser == "" {
		return errors.New("admin_audit_log: actor_user required")
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO admin_audit_log (
			at, actor_user, actor_ip, actor_ua, action,
			target_type, target_id, project_id, details, outcome, error
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.At.UTC(), e.ActorUser,
		nullableString(e.ActorIP),
		nullableString(e.ActorUA),
		e.Action,
		nullableString(e.TargetType),
		nullableString(e.TargetID),
		nullableString(e.ProjectID),
		nullableString(e.Details),
		e.Outcome,
		nullableString(e.Error),
	)
	return err
}

// ListRecent returns the most recent admin events, newest first. Intended
// for the admin UI audit tab; operators can filter server-side via the
// export endpoint instead.
func (r *AdminAuditLogRepo) ListRecent(ctx context.Context, limit int) ([]*AdminAuditEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, at, actor_user, actor_ip, actor_ua, action,
		       target_type, target_id, project_id, details, outcome, error
		  FROM admin_audit_log
		 ORDER BY at DESC
		 LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []*AdminAuditEvent
	for rows.Next() {
		e, err := scanAdminAuditEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func scanAdminAuditEvent(row rowScanner) (*AdminAuditEvent, error) {
	var (
		e          AdminAuditEvent
		actorIP    sql.NullString
		actorUA    sql.NullString
		targetType sql.NullString
		targetID   sql.NullString
		projectID  sql.NullString
		details    sql.NullString
		errorStr   sql.NullString
	)
	err := row.Scan(
		&e.ID, &e.At, &e.ActorUser, &actorIP, &actorUA, &e.Action,
		&targetType, &targetID, &projectID, &details, &e.Outcome, &errorStr,
	)
	if err != nil {
		return nil, err
	}
	e.ActorIP = actorIP.String
	e.ActorUA = actorUA.String
	e.TargetType = targetType.String
	e.TargetID = targetID.String
	e.ProjectID = projectID.String
	e.Details = details.String
	e.Error = errorStr.String
	return &e, nil
}
