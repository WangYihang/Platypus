package storage

import (
	"context"
	"database/sql"
	"time"
)

// AuditExportFilter bounds an export query. Zero time values on either
// side mean "unbounded on that side".
type AuditExportFilter struct {
	From      time.Time
	To        time.Time
	ProjectID string // empty = all projects (global admin scope only)
}

// ListPATRedemptionEventsInRange returns events whose At falls within
// [From, To]. When ProjectID is set, joins pat_tokens to scope by
// project. Returned rows are newest first.
//
// Unlike ListByToken, this one is designed for bulk export — callers
// stream into an ndjson / csv response body, so we return []* with no
// hard limit. Add pagination if a real deployment grows to billions.
func (r *PATRedemptionEventRepo) ListInRange(ctx context.Context, f AuditExportFilter) ([]*PATRedemptionEvent, error) {
	q := `
		SELECT e.id, e.at, e.token_id, e.client_ip, e.machine_id, e.hostname,
		       e.agent_id, e.outcome, e.error_detail
		  FROM pat_redemption_events e`
	var args []any
	var where []string
	if f.ProjectID != "" {
		// LEFT JOIN lets us still include events whose token_id doesn't
		// exist in pat_tokens (scanning attacks); for those rows the
		// project_id filter rejects them, which is the right call — an
		// untargeted scan isn't a project-scoped fact.
		q += ` JOIN pat_tokens t ON t.token_id = e.token_id`
		where = append(where, `t.project_id = ?`)
		args = append(args, f.ProjectID)
	}
	if !f.From.IsZero() {
		where = append(where, `e.at >= ?`)
		args = append(args, f.From.UTC())
	}
	if !f.To.IsZero() {
		where = append(where, `e.at <= ?`)
		args = append(args, f.To.UTC())
	}
	q = appendWhere(q, where)
	q += ` ORDER BY e.at DESC`

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
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

// ListInRange returns connection events whose At falls within [From, To].
// ProjectID scoping joins through agent_sessions to filter — an agent
// isn't inherently tied to a project, but its active session is.
func (r *AgentConnectionEventRepo) ListInRange(ctx context.Context, f AuditExportFilter) ([]*AgentConnectionEvent, error) {
	q := `
		SELECT e.id, e.at, e.agent_id, e.session_id, e.client_ip,
		       e.event_type, e.reason, e.transport
		  FROM agent_connection_events e`
	var args []any
	var where []string
	if f.ProjectID != "" {
		// Same project-scoping caveat as above: events without an
		// agent_id (pre-enroll failures) drop out of the result. They
		// stay in admin_audit_log / pat_redemption_events; export
		// that table too to see the full security picture.
		q += ` LEFT JOIN agent_sessions s ON s.agent_id = e.agent_id AND s.project_id = ?`
		args = append(args, f.ProjectID)
		where = append(where, `s.session_id IS NOT NULL`)
	}
	if !f.From.IsZero() {
		where = append(where, `e.at >= ?`)
		args = append(args, f.From.UTC())
	}
	if !f.To.IsZero() {
		where = append(where, `e.at <= ?`)
		args = append(args, f.To.UTC())
	}
	q = appendWhere(q, where)
	q += ` ORDER BY e.at DESC`

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
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

// ListInRange returns admin actions whose At falls within [From, To].
// ProjectID scoping filters directly on admin_audit_log.project_id.
func (r *AdminAuditLogRepo) ListInRange(ctx context.Context, f AuditExportFilter) ([]*AdminAuditEvent, error) {
	q := `
		SELECT id, at, actor_user, actor_ip, actor_ua, action,
		       target_type, target_id, project_id, details, outcome, error
		  FROM admin_audit_log`
	var args []any
	var where []string
	if f.ProjectID != "" {
		where = append(where, `project_id = ?`)
		args = append(args, f.ProjectID)
	}
	if !f.From.IsZero() {
		where = append(where, `at >= ?`)
		args = append(args, f.From.UTC())
	}
	if !f.To.IsZero() {
		where = append(where, `at <= ?`)
		args = append(args, f.To.UTC())
	}
	q = appendWhere(q, where)
	q += ` ORDER BY at DESC`

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
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

// appendWhere glues optional predicates onto a base query. Keeping it
// tiny here rather than reaching for squirrel / bob — the query set is
// small and well-known.
func appendWhere(q string, clauses []string) string {
	if len(clauses) == 0 {
		return q
	}
	out := q + ` WHERE `
	for i, c := range clauses {
		if i > 0 {
			out += ` AND `
		}
		out += c
	}
	return out
}

// Make sure this file pulls in database/sql even when all queries go
// via the embedded *sql.DB — keeps goimports from deleting the import.
var _ = (*sql.DB)(nil)
