package storage

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// Terminal recording status values mirror the CHECK constraint in
// migrations/000021_terminal_recordings.up.sql.
const (
	RecordingStatusRecording = "recording"
	RecordingStatusCompleted = "completed"
	RecordingStatusFailed    = "failed"
)

// TerminalRecording is one row in terminal_recordings. The actual
// .cast bytes live on disk; FilePath is relative to the configured
// recordings dir.
type TerminalRecording struct {
	ID           string
	ProjectID    string
	HostID       string
	AgentID      string
	UserID       string
	Cols         int
	Rows         int
	Shell        string
	Title        string
	FilePath     string
	SizeBytes    int64
	DurationMs   int64
	FrameCount   int64
	Status       string
	ErrorMessage string
	StartedAt    time.Time
	EndedAt      *time.Time
	// Summary is the LLM-generated one-line description of the
	// session (see internal/llm). NULL until the summariser runs;
	// stays NULL when the project hasn't opted in or the LLM call
	// fails. Clients MUST tolerate empty.
	Summary string
}

// RecordingFilter narrows a List query. Zero-value fields are ignored.
// Cursor is the started_at timestamp of the last item from the
// previous page (RFC3339 nano); rows strictly older than the cursor
// are returned, newest first.
type RecordingFilter struct {
	ProjectID string
	HostID    string
	UserID    string
	AgentID   string
	Status    string
	Q         string // substring match on title / shell / host_id / agent_id
	Cursor    *time.Time
	Limit     int
}

func (db *DB) TerminalRecordings() *RecordingsRepo {
	return &RecordingsRepo{db: db.DB}
}

type RecordingsRepo struct{ db *sql.DB }

const recordingColumns = "id, project_id, host_id, agent_id, user_id, cols, rows, shell, title, file_path, size_bytes, duration_ms, frame_count, status, error_message, started_at, ended_at, summary"

func scanRecording(scanner interface{ Scan(...any) error }) (*TerminalRecording, error) {
	var (
		r       TerminalRecording
		endedAt sql.NullTime
		summary sql.NullString
	)
	if err := scanner.Scan(
		&r.ID, &r.ProjectID, &r.HostID, &r.AgentID, &r.UserID,
		&r.Cols, &r.Rows, &r.Shell, &r.Title, &r.FilePath,
		&r.SizeBytes, &r.DurationMs, &r.FrameCount,
		&r.Status, &r.ErrorMessage, &r.StartedAt, &endedAt, &summary,
	); err != nil {
		return nil, err
	}
	if endedAt.Valid {
		t := endedAt.Time
		r.EndedAt = &t
	}
	if summary.Valid {
		r.Summary = summary.String
	}
	return &r, nil
}

// Create inserts a new recording row in `recording` state. The caller
// holds the file handle and updates the row via UpdateProgress / Finish.
func (r *RecordingsRepo) Create(ctx context.Context, rec *TerminalRecording) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO terminal_recordings (
			id, project_id, host_id, agent_id, user_id,
			cols, rows, shell, title, file_path,
			size_bytes, duration_ms, frame_count, status, error_message,
			started_at, ended_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL)
	`,
		rec.ID, rec.ProjectID, rec.HostID, rec.AgentID, rec.UserID,
		rec.Cols, rec.Rows, rec.Shell, rec.Title, rec.FilePath,
		rec.SizeBytes, rec.DurationMs, rec.FrameCount, rec.Status, rec.ErrorMessage,
		rec.StartedAt.UTC(),
	)
	return err
}

// Get returns the row for id, or ErrNotFound when absent.
func (r *RecordingsRepo) Get(ctx context.Context, id string) (*TerminalRecording, error) {
	row := r.db.QueryRowContext(ctx,
		"SELECT "+recordingColumns+" FROM terminal_recordings WHERE id = ?", id)
	rec, err := scanRecording(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return rec, nil
}

// Finish stamps a terminal status (`completed` or `failed`) and the
// final size / duration / frame_count counters. errMsg only set on
// failure.
func (r *RecordingsRepo) Finish(ctx context.Context, id, status string, sizeBytes, durationMs, frameCount int64, errMsg string, at time.Time) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE terminal_recordings
		   SET status = ?, size_bytes = ?, duration_ms = ?, frame_count = ?,
		       error_message = ?, ended_at = ?
		 WHERE id = ?
	`, status, sizeBytes, durationMs, frameCount, errMsg, at.UTC(), id)
	return err
}

// SetTitle updates the operator-editable label on a row. Idempotent;
// no-op when the row is missing.
func (r *RecordingsRepo) SetTitle(ctx context.Context, id, title string) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE terminal_recordings SET title = ? WHERE id = ?`, title, id)
	if err != nil {
		return err
	}
	return expectOneRow(res)
}

// SetSummary stores the LLM-generated one-line description. The
// summariser runs out-of-band after Finish; row may have been
// deleted in the meantime, so this is a soft update — no error
// when the row is missing (we don't want a deleted recording to
// surface as a goroutine log line).
func (r *RecordingsRepo) SetSummary(ctx context.Context, id, summary string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE terminal_recordings SET summary = ? WHERE id = ?`, summary, id)
	return err
}

// Delete removes the row. The caller is responsible for unlinking the
// file on disk (the storage layer doesn't know the recordings dir).
func (r *RecordingsRepo) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM terminal_recordings WHERE id = ?`, id)
	return err
}

// List returns rows newest-first for the given filter. When Limit is 0
// it defaults to 50; pass a negative number to disable.
func (r *RecordingsRepo) List(ctx context.Context, f RecordingFilter) ([]*TerminalRecording, error) {
	q := "SELECT " + recordingColumns + " FROM terminal_recordings"
	conds, args := buildRecordingWhere(f)
	if f.Cursor != nil {
		conds = append(conds, "started_at < ?")
		args = append(args, f.Cursor.UTC())
	}
	if len(conds) > 0 {
		q += " WHERE " + strings.Join(conds, " AND ")
	}
	q += " ORDER BY started_at DESC"

	limit := f.Limit
	if limit == 0 {
		limit = 50
	}
	if limit > 0 {
		q += " LIMIT ?"
		args = append(args, limit)
	}

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := []*TerminalRecording{}
	for rows.Next() {
		rec, err := scanRecording(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

// Count returns the total number of rows matching the filter (ignoring
// cursor / limit). Used by the list endpoint to populate the page
// counter.
func (r *RecordingsRepo) Count(ctx context.Context, f RecordingFilter) (int, error) {
	q := "SELECT COUNT(*) FROM terminal_recordings"
	conds, args := buildRecordingWhere(f)
	if len(conds) > 0 {
		q += " WHERE " + strings.Join(conds, " AND ")
	}
	var n int
	if err := r.db.QueryRowContext(ctx, q, args...).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// MarkAbandonedRecording transitions every row still in `recording`
// state to `failed` with a generic error_message. Run at server boot
// to close audit windows the previous instance left open.
func (r *RecordingsRepo) MarkAbandoned(ctx context.Context, msg string, at time.Time) (int64, error) {
	res, err := r.db.ExecContext(ctx, `
		UPDATE terminal_recordings
		   SET status = 'failed', error_message = ?, ended_at = ?
		 WHERE status = 'recording'
	`, msg, at.UTC())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func buildRecordingWhere(f RecordingFilter) ([]string, []any) {
	var conds []string
	var args []any
	if f.ProjectID != "" {
		conds = append(conds, "project_id = ?")
		args = append(args, f.ProjectID)
	}
	if f.HostID != "" {
		conds = append(conds, "host_id = ?")
		args = append(args, f.HostID)
	}
	if f.UserID != "" {
		conds = append(conds, "user_id = ?")
		args = append(args, f.UserID)
	}
	if f.AgentID != "" {
		conds = append(conds, "agent_id = ?")
		args = append(args, f.AgentID)
	}
	if f.Status != "" {
		conds = append(conds, "status = ?")
		args = append(args, f.Status)
	}
	if q := strings.TrimSpace(f.Q); q != "" {
		needle := "%" + q + "%"
		conds = append(conds, "(title LIKE ? OR shell LIKE ? OR host_id LIKE ? OR agent_id LIKE ?)")
		args = append(args, needle, needle, needle, needle)
	}
	return conds, args
}
