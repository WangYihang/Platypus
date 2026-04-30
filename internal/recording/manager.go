package recording

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/WangYihang/Platypus/internal/storage"
)

// Manager coordinates cast-file storage + the terminal_recordings DB
// rows. The terminal handler asks for a Session at shell-open and
// hands all stdout/stderr/resize events to it; on close, the manager
// finalises the file and stamps ended_at + size_bytes on the row.
//
// Disabled is the no-op flag: when recordings are turned off (no
// config or explicit Enabled=false) every Begin call returns a Session
// whose write methods are no-ops and whose Finish does nothing. Lets
// the terminal handler stay free of "is recording on?" branches.
type Manager struct {
	db       *storage.DB
	dir      string
	disabled bool
}

// New constructs a Manager. dir is the absolute path where .cast files
// are written; disabled=true bypasses both the file and DB writes.
func New(db *storage.DB, dir string, enabled bool) *Manager {
	return &Manager{db: db, dir: dir, disabled: !enabled || db == nil || dir == ""}
}

// Enabled reports whether the manager will actually persist. Useful
// for the handler to decide whether to log "recording started".
func (m *Manager) Enabled() bool { return !m.disabled }

// Dir returns the cast-file storage directory.
func (m *Manager) Dir() string { return m.dir }

// BeginInput captures the input parameters for a new recording.
// ProjectID, HostID, and StartedAt are required when recording is
// enabled. UserID, AgentID, Shell, Cols, Rows are best-effort.
type BeginInput struct {
	ProjectID string
	HostID    string
	AgentID   string
	UserID    string
	Shell     string
	Title     string
	Cols      uint32
	Rows      uint32
	StartedAt time.Time
}

// Begin opens a new recording session. When the manager is disabled
// it returns a no-op Session; otherwise it allocates an ID, opens
// the cast file, and inserts a row. Errors creating the file or row
// are returned verbatim — callers can choose to abort the shell or
// continue without persistence.
func (m *Manager) Begin(ctx context.Context, in BeginInput) (*Session, error) {
	if m.disabled {
		return &Session{disabled: true}, nil
	}

	id, err := newID()
	if err != nil {
		return nil, err
	}
	relPath := id + ".cast"
	absPath := filepath.Join(m.dir, relPath)

	startedAt := in.StartedAt
	if startedAt.IsZero() {
		startedAt = time.Now().UTC()
	}

	w, err := NewWriter(absPath, Header{
		Version:   2,
		Width:     int(safeDim(in.Cols, 80)),
		Height:    int(safeDim(in.Rows, 24)),
		Timestamp: startedAt.Unix(),
		Title:     in.Title,
		Command:   in.Shell,
	})
	if err != nil {
		return nil, err
	}

	rec := &storage.TerminalRecording{
		ID:        id,
		ProjectID: in.ProjectID,
		HostID:    in.HostID,
		AgentID:   in.AgentID,
		UserID:    in.UserID,
		Cols:      int(safeDim(in.Cols, 80)),
		Rows:      int(safeDim(in.Rows, 24)),
		Shell:     in.Shell,
		Title:     in.Title,
		FilePath:  relPath,
		Status:    storage.RecordingStatusRecording,
		StartedAt: startedAt,
	}
	if err := m.db.TerminalRecordings().Create(ctx, rec); err != nil {
		_ = w.Close()
		_ = os.Remove(absPath)
		return nil, err
	}

	return &Session{
		manager: m,
		writer:  w,
		id:      id,
		absPath: absPath,
	}, nil
}

// Session is the per-shell handle the terminal handler interacts
// with. Methods are safe to call from the WS goroutine and the
// stream-pump goroutine — internal locking lives in Writer.
//
// A Session is single-use: once Finish has been called, further
// writes and a second Finish are no-ops.
type Session struct {
	manager *Manager
	writer  *Writer
	id      string
	absPath string
	// disabled tags a no-op session (manager turned off). All
	// methods short-circuit.
	disabled bool

	mu       sync.Mutex
	finished bool
}

// ID returns the recording id (also the .cast filename stem). Empty
// for disabled sessions.
func (s *Session) ID() string {
	if s == nil || s.disabled {
		return ""
	}
	return s.id
}

// WriteOutput records bytes the agent emitted on stdout/stderr.
func (s *Session) WriteOutput(data []byte) {
	if s == nil || s.disabled || s.writer == nil {
		return
	}
	_ = s.writer.WriteOutput(data)
}

// WriteInput records bytes the operator typed.
func (s *Session) WriteInput(data []byte) {
	if s == nil || s.disabled || s.writer == nil {
		return
	}
	_ = s.writer.WriteInput(data)
}

// WriteResize records a pty resize event.
func (s *Session) WriteResize(cols, rows uint32) {
	if s == nil || s.disabled || s.writer == nil {
		return
	}
	_ = s.writer.WriteResize(cols, rows)
}

// Finish closes the cast file and stamps the DB row. errMsg is the
// terminal failure reason (empty string for clean exits). Idempotent.
func (s *Session) Finish(ctx context.Context, errMsg string) {
	if s == nil || s.disabled {
		return
	}
	s.mu.Lock()
	if s.finished {
		s.mu.Unlock()
		return
	}
	s.finished = true
	s.mu.Unlock()

	bytes, frames, dur := s.writer.Stats()
	_ = s.writer.Close()

	// Re-stat the file in case a partial write left bytes behind we
	// didn't account for (mismatched buffering, etc.).
	if fi, err := os.Stat(s.absPath); err == nil {
		bytes = fi.Size()
	}

	status := storage.RecordingStatusCompleted
	if errMsg != "" {
		status = storage.RecordingStatusFailed
	}
	endedAt := time.Now().UTC()
	_ = s.manager.db.TerminalRecordings().Finish(
		ctx, s.id, status, bytes, dur.Milliseconds(), frames, errMsg, endedAt,
	)
}

// AbsolutePath returns the on-disk file path. Empty for disabled
// sessions. Useful for the file-download handler.
func (s *Session) AbsolutePath() string {
	if s == nil || s.disabled {
		return ""
	}
	return s.absPath
}

// PathFor returns the on-disk path for a recording id. Used by the
// download handler to stream cast files identified by DB row.
func (m *Manager) PathFor(rec *storage.TerminalRecording) string {
	if rec == nil || rec.FilePath == "" {
		return ""
	}
	if filepath.IsAbs(rec.FilePath) {
		return rec.FilePath
	}
	return filepath.Join(m.dir, rec.FilePath)
}

// DeleteFile unlinks the on-disk cast file for a recording. Missing
// files are not an error — the row may be stamped without ever having
// been flushed (Begin failure path).
func (m *Manager) DeleteFile(rec *storage.TerminalRecording) error {
	p := m.PathFor(rec)
	if p == "" {
		return nil
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// newID returns a 16-byte hex string. URL- and filename-safe; matches
// the shape of other Platypus IDs.
func newID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func safeDim(v, fallback uint32) uint32 {
	if v == 0 {
		return fallback
	}
	return v
}
