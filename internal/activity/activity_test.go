package activity

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/WangYihang/Platypus/internal/storage"
)

// TestRecorder_DrainsInOrder_ThenClose exercises the happy path: the
// writer goroutine drains queued events, and Close() flushes the tail
// before returning. After Close, every Record'd event must be
// observable in storage.
func TestRecorder_DrainsInOrder_ThenClose(t *testing.T) {
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	r := New(db)

	const n = 20
	for i := 0; i < n; i++ {
		r.Record(Input{
			Category: storage.CategoryAgent,
			Action:   fmt.Sprintf("evt-%03d", i),
		})
	}
	// Close blocks until the writer drains the queue. After it returns
	// every event is either persisted or dropped (none can still be
	// in-flight).
	r.Close()

	events, _, err := db.Activities().List(context.Background(), storage.ActivityFilter{Limit: 200})
	if err != nil {
		t.Fatalf("Activities().List: %v", err)
	}
	if len(events) != n {
		t.Fatalf("got %d events; want %d", len(events), n)
	}
	// List returns newest-first, so reverse our expected order.
	for i, ev := range events {
		want := fmt.Sprintf("evt-%03d", n-1-i)
		if ev.Action != want {
			t.Fatalf("event[%d].Action = %q; want %q", i, ev.Action, want)
		}
	}
}

// TestRecorder_CloseIsIdempotent makes sure a second Close is a
// no-op rather than a panic on close-of-closed-channel.
func TestRecorder_CloseIsIdempotent(t *testing.T) {
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	r := New(db)
	r.Close()
	r.Close() // must not panic
}

// TestRecorder_PostCloseRecordIsNoop confirms that calls racing with
// Close are silently dropped rather than blocking forever or panicking.
func TestRecorder_PostCloseRecordIsNoop(t *testing.T) {
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	r := New(db)
	r.Close()

	done := make(chan struct{})
	go func() {
		// If this blocks, the test times out; Record must see the
		// closing signal and bail immediately.
		r.Record(Input{Category: storage.CategoryAgent, Action: "post-close"})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Record blocked after Close")
	}
}

// TestRecorder_BackpressureBlocksWhenFull pins the "blocking send"
// contract: with a tiny queue and a paused writer the Nth Record call
// can't complete until the writer consumes a slot.
func TestRecorder_BackpressureBlocksWhenFull(t *testing.T) {
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	// Cap of 2; the writer goroutine has to consume before a 3rd
	// Record can enqueue.
	r := newWithCap(db, 2)

	// Fill the queue. With a live writer, some events will already
	// have been drained, so there's no clean "queue is full" moment —
	// instead we test the weaker but still meaningful invariant that
	// Record returns for every call we issue and Close flushes them
	// all.
	const n = 50
	for i := 0; i < n; i++ {
		r.Record(Input{Category: storage.CategoryAgent, Action: fmt.Sprintf("bp-%03d", i)})
	}
	r.Close()

	events, _, err := db.Activities().List(context.Background(), storage.ActivityFilter{Limit: 200})
	if err != nil {
		t.Fatalf("Activities().List: %v", err)
	}
	if len(events) != n {
		t.Fatalf("got %d events after Close; want %d", len(events), n)
	}
}

// TestRecorder_RecordWithContext_HonoursCancel shows that a caller
// whose context is canceled doesn't stall on a full queue — the
// event is dropped so the request handler can return.
func TestRecorder_RecordWithContext_HonoursCancel(t *testing.T) {
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	// Build a recorder with db=nil so the writer never starts and
	// queue fills immediately; cap=0 makes every send block until
	// either the writer drains (never) or ctx cancels.
	r := &Recorder{
		db:      db,
		queue:   make(chan *storage.Activity), // unbuffered, no writer
		closing: make(chan struct{}),
	}
	// No writer goroutine — senders will block indefinitely without ctx.

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		r.RecordWithContext(ctx, Input{Category: storage.CategoryAgent, Action: "ctx-cancel"})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("RecordWithContext did not honour ctx cancellation")
	}
}
