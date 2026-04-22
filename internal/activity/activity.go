// Package activity is the process-wide entry point for writing
// structured activity rows to the storage.activities table. It
// intentionally has no dependency on gin, so both HTTP handlers and
// non-HTTP code paths (agent-side TCP handshake, server lifecycle,
// enrollment service, …) can call through it.
//
// HTTP-aware helpers — which fill in actor_ip, actor_ua, request_id, and
// the project id from the URL — live in internal/api.
package activity

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"

	"github.com/WangYihang/Platypus/internal/log"
	"github.com/WangYihang/Platypus/internal/storage"
)

// Input is the caller-facing payload. Every field is optional; Record
// fills in defaults (at, actor_type, outcome) when the caller leaves
// them empty. Fields left zero serialise as NULL / empty in storage.
type Input struct {
	ProjectID    *string // nil = global event
	ActorType    string
	ActorUser    string
	ActorIP      string
	ActorUA      string
	ActorTokenID string
	Category     string
	Action       string
	TargetType   string
	TargetID     string
	TargetLabel  string
	Outcome      string
	Error        string
	DurationMs   *int64
	RequestID    string
	SessionID    string
	Meta         any // marshalled to JSON; pass a string to skip re-marshalling

	At time.Time
}

// Recorder wraps the storage layer and emits rows asynchronously. Every
// call to Record is fire-and-forget: the goroutine it spawns is bounded
// by a 2-second context and all errors are logged rather than returned.
// This keeps the audit layer from ever blocking the request path.
type Recorder struct {
	db *storage.DB
}

// New constructs a recorder bound to the given DB.
func New(db *storage.DB) *Recorder {
	return &Recorder{db: db}
}

// Record emits one activity row asynchronously.
func (r *Recorder) Record(in Input) {
	if r == nil || r.db == nil {
		return
	}
	ev := buildActivity(in)
	go r.persist(ev)
}

// RecordWithContext is the context-aware variant; handy for tests that
// want a bounded write, or for agent-side code that has its own
// lifecycle context.
func (r *Recorder) RecordWithContext(ctx context.Context, in Input) {
	if r == nil || r.db == nil {
		return
	}
	ev := buildActivity(in)
	go func() {
		writeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		r.writeOnce(writeCtx, ev)
	}()
}

// Time runs fn, measures duration, and records a row based on fn's
// return value. The caller's error is returned unchanged; audit-write
// errors are swallowed.
func (r *Recorder) Time(in Input, fn func() error) error {
	start := time.Now().UTC()
	err := fn()
	dur := time.Since(start).Milliseconds()
	in.DurationMs = &dur
	if in.At.IsZero() {
		in.At = start
	}
	if err != nil {
		if in.Outcome == "" {
			in.Outcome = storage.OutcomeError
		}
		if in.Error == "" {
			in.Error = err.Error()
		}
	} else if in.Outcome == "" {
		in.Outcome = storage.OutcomeSuccess
	}
	r.Record(in)
	return err
}

func (r *Recorder) persist(ev *storage.Activity) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	r.writeOnce(ctx, ev)
}

func (r *Recorder) writeOnce(ctx context.Context, ev *storage.Activity) {
	defer func() {
		if p := recover(); p != nil {
			log.Error("activity: recorder panic: %v", p)
		}
	}()
	if err := r.db.Activities().Record(ctx, ev); err != nil {
		log.Error("activity: record %q: %v", ev.Action, err)
	}
}

// buildActivity converts Input into the storage row. Shared by all
// entry points so default handling stays in one place.
func buildActivity(in Input) *storage.Activity {
	return &storage.Activity{
		At:           firstNonZero(in.At, time.Now().UTC()),
		ProjectID:    in.ProjectID,
		ActorType:    defaultStr(in.ActorType, storage.ActorTypeSystem),
		ActorUser:    in.ActorUser,
		ActorIP:      in.ActorIP,
		ActorUA:      in.ActorUA,
		ActorTokenID: in.ActorTokenID,
		Category:     in.Category,
		Action:       in.Action,
		TargetType:   in.TargetType,
		TargetID:     in.TargetID,
		TargetLabel:  in.TargetLabel,
		Outcome:      defaultStr(in.Outcome, storage.OutcomeSuccess),
		Error:        in.Error,
		DurationMs:   in.DurationMs,
		RequestID:    in.RequestID,
		SessionID:    in.SessionID,
		Meta:         marshalMeta(in.Meta),
	}
}

// marshalMeta normalises Meta to the stored TEXT column. A string is
// trusted verbatim so callers can pass pre-encoded JSON; everything
// else is json-marshalled.
func marshalMeta(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	b, err := json.Marshal(v)
	if err != nil {
		log.Error("activity: marshal meta: %v", err)
		return ""
	}
	return string(b)
}

func firstNonZero(a, b time.Time) time.Time {
	if !a.IsZero() {
		return a
	}
	return b
}

func defaultStr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// --- Package-level singleton -------------------------------------------------

var (
	recorderPtr atomic.Pointer[Recorder]
	recorderMu  sync.Mutex
)

// SetRecorder installs the process-wide recorder. Safe to call multiple
// times (later calls win). In production main.go calls it once at
// startup; tests can install a recorder against an in-memory DB.
func SetRecorder(r *Recorder) {
	recorderMu.Lock()
	defer recorderMu.Unlock()
	recorderPtr.Store(r)
}

// GetRecorder returns the installed recorder or nil. Chiefly useful for
// tests to assert a write happened or swap the recorder out.
func GetRecorder() *Recorder {
	return recorderPtr.Load()
}

// Record writes via the singleton. If no recorder is installed (e.g. a
// unit test that skipped main.go wiring) the call is a no-op.
func Record(in Input) {
	if r := recorderPtr.Load(); r != nil {
		r.Record(in)
	}
}

// RecordWithContext is the context-aware variant of Record.
func RecordWithContext(ctx context.Context, in Input) {
	if r := recorderPtr.Load(); r != nil {
		r.RecordWithContext(ctx, in)
	}
}

// Time wraps fn with duration measurement and records via the singleton.
// Returns fn's error unchanged.
func Time(in Input, fn func() error) error {
	if r := recorderPtr.Load(); r != nil {
		return r.Time(in, fn)
	}
	return fn()
}
