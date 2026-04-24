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

// activityQueueCap bounds the in-flight queue between Record callers
// and the writer goroutine. A full queue means Record blocks
// (backpressure) — preferable to unbounded goroutine spawn under a
// sudden burst, or to silent data loss. 4096 slots is ≈128 KiB of
// pointers, a trivial amount of memory for the guarantee it buys.
const activityQueueCap = 4096

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

// Recorder wraps the storage layer and emits rows asynchronously
// through a single writer goroutine fed by a bounded queue. Every
// Record call enqueues one row; the writer drains serially and
// handles DB errors via the log package. Callers never see a DB
// error from the audit path.
type Recorder struct {
	db        *storage.DB
	queue     chan *storage.Activity
	closing   chan struct{} // closed by Close() — signals senders + writer
	closeOnce sync.Once
	wg        sync.WaitGroup
}

// New constructs a recorder bound to the given DB and starts its
// background writer goroutine. Call Close() to drain the queue and
// stop the writer (typically not needed in a long-running server;
// tests use it to observe the post-drain state).
func New(db *storage.DB) *Recorder {
	return newWithCap(db, activityQueueCap)
}

// newWithCap is the in-package constructor that lets tests pick a
// smaller queue so they can exercise the full-queue path without
// allocating 4096 slots.
func newWithCap(db *storage.DB, cap int) *Recorder {
	r := &Recorder{
		db:      db,
		queue:   make(chan *storage.Activity, cap),
		closing: make(chan struct{}),
	}
	if db != nil {
		r.wg.Add(1)
		go r.writer()
	}
	return r
}

// Record enqueues one activity row. If the queue is full the call
// blocks until the writer drains a slot OR Close() is called (the
// event is then dropped rather than panicking on a closed send).
// Post-Close calls are silent no-ops.
func (r *Recorder) Record(in Input) {
	if r == nil || r.db == nil {
		return
	}
	ev := buildActivity(in)
	select {
	case r.queue <- ev:
	case <-r.closing:
		// recorder is shutting down; drop
	}
}

// RecordWithContext is the context-aware variant. Same backpressure
// semantics as Record, but the caller's context can abort a blocked
// enqueue — useful for request-scoped callers who don't want their
// handler to stall on a full audit queue.
func (r *Recorder) RecordWithContext(ctx context.Context, in Input) {
	if r == nil || r.db == nil {
		return
	}
	ev := buildActivity(in)
	select {
	case r.queue <- ev:
	case <-ctx.Done():
		log.Warn("activity: drop (ctx canceled): %s", ev.Action)
	case <-r.closing:
		// recorder is shutting down; drop
	}
}

// Time runs fn, measures duration, and records a row based on fn's
// return value. The caller's error is returned unchanged; audit-write
// errors are swallowed (via the recorder's log-on-fail policy).
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

// Close signals the writer to drain the remaining queued events and
// exit. Blocks until the writer has finished processing everything it
// had in hand when Close was called. Safe to call multiple times;
// only the first call has an effect.
func (r *Recorder) Close() {
	if r == nil {
		return
	}
	r.closeOnce.Do(func() {
		close(r.closing)
	})
	r.wg.Wait()
}

// writer is the single consumer of the queue. It drains each event
// through writeOnce and exits after Close() signals AND the queue is
// empty — guaranteeing that every Record that completed before Close
// lands in storage.
func (r *Recorder) writer() {
	defer r.wg.Done()
	for {
		select {
		case ev := <-r.queue:
			r.persist(ev)
		case <-r.closing:
			// Drain whatever senders got in before Close landed.
			for {
				select {
				case ev := <-r.queue:
					r.persist(ev)
				default:
					return
				}
			}
		}
	}
}

// persist writes a single event with a bounded-duration context so a
// wedged DB connection doesn't stall the whole writer.
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
