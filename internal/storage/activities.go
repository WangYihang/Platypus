package storage

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Activity is one row in the unified activities log. Every meaningful
// action in the system — user, admin, agent, server — writes exactly one
// of these. Common fields are promoted to columns so they can participate
// in indexes and filters; anything that varies per action lives in the
// Meta JSON blob.
//
// ProjectID is a pointer because many events (user.login,
// project.create, server.start) happen outside any project scope. A nil
// pointer means "global event"; the repo writes SQL NULL for it.
type Activity struct {
	ID           int64
	At           time.Time
	ProjectID    *string
	ActorType    string // "user" | "system" | "agent" | "api_token" | "anonymous"
	ActorUser    string
	ActorIP      string
	ActorUA      string
	ActorTokenID string
	Category     string
	Action       string
	TargetType   string
	TargetID     string
	TargetLabel  string
	Outcome      string // "success" | "denied" | "error"
	Error        string
	DurationMs   *int64
	RequestID    string
	SessionID    string
	Meta         string // JSON-encoded payload; "" when none
}

// actor type constants.
const (
	ActorTypeUser      = "user"
	ActorTypeSystem    = "system"
	ActorTypeAgent     = "agent"
	ActorTypeAPIToken  = "api_token"
	ActorTypeAnonymous = "anonymous"
)

// outcome constants.
const (
	OutcomeSuccess = "success"
	OutcomeDenied  = "denied"
	OutcomeError   = "error"
)

// ActivityCategory enumerates the common categories; free-form strings
// are accepted at write time, but clients should prefer these values.
const (
	CategoryAuth     = "auth"
	CategorySession  = "session"
	CategoryCommand  = "command"
	CategoryFile     = "file"
	CategoryTunnel   = "tunnel"
	CategoryListener = "listener"
	CategoryAgent    = "agent"
	CategoryAdmin    = "admin"
	CategoryProject  = "project"
	CategoryServer   = "server"
	CategoryUser     = "user"
	CategoryHTTP     = "http"
)

// Activities returns the repo handle. Kept short to match the rest of
// the storage API (db.Users(), db.Projects(), …).
func (db *DB) Activities() *ActivityRepo {
	return &ActivityRepo{db: db.DB}
}

type ActivityRepo struct {
	db *sql.DB
}

// Record inserts one activity row. The caller is expected to set At
// explicitly — we don't default here because some recorders want to
// preserve a start time measured earlier.
func (r *ActivityRepo) Record(ctx context.Context, e *Activity) error {
	if e == nil {
		return errors.New("activities: nil event")
	}
	if e.At.IsZero() {
		e.At = time.Now().UTC()
	}
	if e.ActorType == "" {
		e.ActorType = ActorTypeSystem
	}
	if e.Outcome == "" {
		e.Outcome = OutcomeSuccess
	}
	if e.Category == "" {
		return errors.New("activities: category required")
	}
	if e.Action == "" {
		return errors.New("activities: action required")
	}

	var projectID any
	if e.ProjectID != nil && *e.ProjectID != "" {
		projectID = *e.ProjectID
	}
	var duration any
	if e.DurationMs != nil {
		duration = *e.DurationMs
	}

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO activities (
			at, project_id, actor_type, actor_user, actor_ip, actor_ua,
			actor_token_id, category, action, target_type, target_id,
			target_label, outcome, error, duration_ms, request_id,
			session_id, meta
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.At.UTC(),
		projectID,
		e.ActorType,
		nullableString(e.ActorUser),
		nullableString(e.ActorIP),
		nullableString(e.ActorUA),
		nullableString(e.ActorTokenID),
		e.Category,
		e.Action,
		nullableString(e.TargetType),
		nullableString(e.TargetID),
		nullableString(e.TargetLabel),
		e.Outcome,
		nullableString(e.Error),
		duration,
		nullableString(e.RequestID),
		nullableString(e.SessionID),
		nullableString(e.Meta),
	)
	return err
}

// ActivityFilter bounds a List query. Zero values mean "no filter on
// that dimension". ProjectID has three states: nil (no project filter),
// pointer to "" (only global events, project_id IS NULL), pointer to a
// specific id (that project; optionally merge global via IncludeGlobal).
type ActivityFilter struct {
	ProjectID     *string
	IncludeGlobal bool // when ProjectID is set, also include project_id IS NULL rows
	Categories    []string
	Actions       []string
	ActorUser     string
	Outcome       string
	SessionID     string
	TargetType    string
	TargetID      string
	Search        string // free-text LIKE against action / target_label / meta
	From          time.Time
	To            time.Time
	Limit         int
	Cursor        string // opaque keyset cursor from a prior page
}

// DefaultActivityListLimit is applied when Filter.Limit <= 0.
const DefaultActivityListLimit = 50

// MaxActivityListLimit caps runaway requests at the handler boundary.
const MaxActivityListLimit = 200

// List returns a page of activities matching the filter, newest first,
// plus an opaque cursor that the caller can pass back to fetch the next
// page. When there are no more rows the returned cursor is "".
func (r *ActivityRepo) List(ctx context.Context, f ActivityFilter) ([]*Activity, string, error) {
	limit := f.Limit
	if limit <= 0 {
		limit = DefaultActivityListLimit
	}
	if limit > MaxActivityListLimit {
		limit = MaxActivityListLimit
	}

	q := `
		SELECT id, at, project_id, actor_type, actor_user, actor_ip,
		       actor_ua, actor_token_id, category, action, target_type,
		       target_id, target_label, outcome, error, duration_ms,
		       request_id, session_id, meta
		  FROM activities`
	var args []any
	var where []string

	if f.ProjectID != nil {
		if *f.ProjectID == "" {
			where = append(where, `project_id IS NULL`)
		} else if f.IncludeGlobal {
			where = append(where, `(project_id = ? OR project_id IS NULL)`)
			args = append(args, *f.ProjectID)
		} else {
			where = append(where, `project_id = ?`)
			args = append(args, *f.ProjectID)
		}
	}
	if len(f.Categories) > 0 {
		placeholders := strings.TrimRight(strings.Repeat("?,", len(f.Categories)), ",")
		where = append(where, `category IN (`+placeholders+`)`)
		for _, c := range f.Categories {
			args = append(args, c)
		}
	}
	if len(f.Actions) > 0 {
		placeholders := strings.TrimRight(strings.Repeat("?,", len(f.Actions)), ",")
		where = append(where, `action IN (`+placeholders+`)`)
		for _, a := range f.Actions {
			args = append(args, a)
		}
	}
	if f.ActorUser != "" {
		where = append(where, `actor_user = ?`)
		args = append(args, f.ActorUser)
	}
	if f.Outcome != "" {
		where = append(where, `outcome = ?`)
		args = append(args, f.Outcome)
	}
	if f.SessionID != "" {
		where = append(where, `session_id = ?`)
		args = append(args, f.SessionID)
	}
	if f.TargetType != "" {
		where = append(where, `target_type = ?`)
		args = append(args, f.TargetType)
	}
	if f.TargetID != "" {
		where = append(where, `target_id = ?`)
		args = append(args, f.TargetID)
	}
	if s := strings.TrimSpace(f.Search); s != "" {
		like := "%" + s + "%"
		where = append(where, `(action LIKE ? OR target_label LIKE ? OR meta LIKE ?)`)
		args = append(args, like, like, like)
	}
	if !f.From.IsZero() {
		where = append(where, `at >= ?`)
		args = append(args, f.From.UTC())
	}
	if !f.To.IsZero() {
		where = append(where, `at <= ?`)
		args = append(args, f.To.UTC())
	}

	// Keyset cursor: (at DESC, id DESC). The cursor encodes the last row
	// returned; WHERE (at, id) < (cursor.at, cursor.id) in the reverse
	// ordering is "strictly older than cursor".
	if f.Cursor != "" {
		cursorAt, cursorID, err := decodeActivityCursor(f.Cursor)
		if err != nil {
			return nil, "", fmt.Errorf("invalid cursor: %w", err)
		}
		where = append(where, `(at < ? OR (at = ? AND id < ?))`)
		args = append(args, cursorAt, cursorAt, cursorID)
	}

	q = appendWhere(q, where)
	q += ` ORDER BY at DESC, id DESC LIMIT ?`
	args = append(args, limit+1) // over-fetch one to detect "more"

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = rows.Close() }()

	out := make([]*Activity, 0, limit)
	for rows.Next() {
		a, err := scanActivity(rows)
		if err != nil {
			return nil, "", err
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}

	var cursor string
	if len(out) > limit {
		last := out[limit-1]
		cursor = encodeActivityCursor(last.At, last.ID)
		out = out[:limit]
	}
	return out, cursor, nil
}

// Count returns the total matching rows, ignoring Limit/Cursor. Useful
// for the header count; callers may skip it on hot paths.
// DeleteOlderThan removes every activity row with at < cutoff and
// returns the number of rows deleted. Used by the retention reaper
// to keep the audit log bounded when admins configure a non-zero
// retention window.
func (r *ActivityRepo) DeleteOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM activities WHERE at < ?`, cutoff.UTC())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (r *ActivityRepo) Count(ctx context.Context, f ActivityFilter) (int64, error) {
	// Strip pagination from the filter and reuse the WHERE builder by
	// doing a micro duplication (the subset below is intentionally
	// small; it's not worth a shared helper yet).
	f.Cursor = ""
	f.Limit = 0

	q := `SELECT COUNT(1) FROM activities`
	var args []any
	var where []string
	if f.ProjectID != nil {
		if *f.ProjectID == "" {
			where = append(where, `project_id IS NULL`)
		} else if f.IncludeGlobal {
			where = append(where, `(project_id = ? OR project_id IS NULL)`)
			args = append(args, *f.ProjectID)
		} else {
			where = append(where, `project_id = ?`)
			args = append(args, *f.ProjectID)
		}
	}
	if len(f.Categories) > 0 {
		placeholders := strings.TrimRight(strings.Repeat("?,", len(f.Categories)), ",")
		where = append(where, `category IN (`+placeholders+`)`)
		for _, c := range f.Categories {
			args = append(args, c)
		}
	}
	if len(f.Actions) > 0 {
		placeholders := strings.TrimRight(strings.Repeat("?,", len(f.Actions)), ",")
		where = append(where, `action IN (`+placeholders+`)`)
		for _, a := range f.Actions {
			args = append(args, a)
		}
	}
	if f.ActorUser != "" {
		where = append(where, `actor_user = ?`)
		args = append(args, f.ActorUser)
	}
	if f.Outcome != "" {
		where = append(where, `outcome = ?`)
		args = append(args, f.Outcome)
	}
	if f.SessionID != "" {
		where = append(where, `session_id = ?`)
		args = append(args, f.SessionID)
	}
	if f.TargetType != "" {
		where = append(where, `target_type = ?`)
		args = append(args, f.TargetType)
	}
	if f.TargetID != "" {
		where = append(where, `target_id = ?`)
		args = append(args, f.TargetID)
	}
	if s := strings.TrimSpace(f.Search); s != "" {
		like := "%" + s + "%"
		where = append(where, `(action LIKE ? OR target_label LIKE ? OR meta LIKE ?)`)
		args = append(args, like, like, like)
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

	var n int64
	if err := r.db.QueryRowContext(ctx, q, args...).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

func scanActivity(row rowScanner) (*Activity, error) {
	var (
		a            Activity
		projectID    sql.NullString
		actorUser    sql.NullString
		actorIP      sql.NullString
		actorUA      sql.NullString
		actorTokenID sql.NullString
		targetType   sql.NullString
		targetID     sql.NullString
		targetLabel  sql.NullString
		errStr       sql.NullString
		duration     sql.NullInt64
		requestID    sql.NullString
		sessionID    sql.NullString
		meta         sql.NullString
	)
	if err := row.Scan(
		&a.ID, &a.At, &projectID, &a.ActorType, &actorUser, &actorIP,
		&actorUA, &actorTokenID, &a.Category, &a.Action, &targetType,
		&targetID, &targetLabel, &a.Outcome, &errStr, &duration,
		&requestID, &sessionID, &meta,
	); err != nil {
		return nil, err
	}
	if projectID.Valid {
		v := projectID.String
		a.ProjectID = &v
	}
	a.ActorUser = actorUser.String
	a.ActorIP = actorIP.String
	a.ActorUA = actorUA.String
	a.ActorTokenID = actorTokenID.String
	a.TargetType = targetType.String
	a.TargetID = targetID.String
	a.TargetLabel = targetLabel.String
	a.Error = errStr.String
	if duration.Valid {
		v := duration.Int64
		a.DurationMs = &v
	}
	a.RequestID = requestID.String
	a.SessionID = sessionID.String
	a.Meta = meta.String
	return &a, nil
}

// encodeActivityCursor packs (at, id) into a URL-safe opaque string so
// clients never need to parse the format. Downstream code treats the
// cursor as opaque.
func encodeActivityCursor(at time.Time, id int64) string {
	raw := fmt.Sprintf("%d|%d", at.UTC().UnixNano(), id)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// appendWhere glues optional predicates onto a base query. Kept tiny
// rather than pulling in a query-builder dependency — the set of queries
// that need it is small and well-known.
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

func decodeActivityCursor(s string) (time.Time, int64, error) {
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return time.Time{}, 0, err
	}
	parts := strings.SplitN(string(raw), "|", 2)
	if len(parts) != 2 {
		return time.Time{}, 0, errors.New("cursor format")
	}
	ns, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return time.Time{}, 0, err
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return time.Time{}, 0, err
	}
	return time.Unix(0, ns).UTC(), id, nil
}
