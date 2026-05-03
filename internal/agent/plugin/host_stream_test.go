package plugin

import (
	"sync/atomic"
	"testing"
)

// Pure-Go unit tests for the streamRegistry + streamCtx state
// machine. The wasm-side host_stream_* host functions go through
// extism's runtime, which we don't exercise here (mocking the
// extism.CurrentPlugin handle is impractical); instead we cover
// the registry mechanics + the EOF / closed-write atomics, which
// are the bits the host functions delegate to.
//
// End-to-end coverage with a real wasm-streaming plugin is the
// follow-up commit that migrates one stream type (file_read is
// the planned candidate).

func TestStreamRegistry_OpenAssignsUniqueIDs(t *testing.T) {
	r := newStreamRegistry()
	a := r.open()
	b := r.open()
	c := r.open()
	if a.id == b.id || b.id == c.id || a.id == c.id {
		t.Fatalf("expected unique ids, got %d %d %d", a.id, b.id, c.id)
	}
	if a.id == 0 {
		t.Errorf("ids should be 1-indexed (0 reserved as 'invalid')")
	}
}

func TestStreamRegistry_GetReturnsSameInstance(t *testing.T) {
	r := newStreamRegistry()
	s := r.open()
	got := r.get(s.id)
	if got != s {
		t.Errorf("get returned a different *streamCtx than open allocated")
	}
	if r.get(99999) != nil {
		t.Errorf("get of unknown id should return nil")
	}
}

func TestStreamRegistry_CloseRemovesEntry(t *testing.T) {
	r := newStreamRegistry()
	s := r.open()
	r.closeID(s.id)
	if r.get(s.id) != nil {
		t.Errorf("closed id still resolves")
	}
	// Idempotent.
	r.closeID(s.id)
}

func TestStreamCtx_AtomicsZeroValueBehaviour(t *testing.T) {
	var s streamCtx
	if s.writeClosed.Load() {
		t.Errorf("writeClosed default = true; want false")
	}
	if s.inboundEOF.Load() {
		t.Errorf("inboundEOF default = true; want false")
	}
	// First Swap returns the previous value (false).
	if old := s.writeClosed.Swap(true); old {
		t.Errorf("first writeClosed.Swap(true) = %v; want false", old)
	}
	// Second Swap returns the previous value (true) — the host fn
	// uses this to detect "already closed" without an extra mutex.
	if old := s.writeClosed.Swap(true); !old {
		t.Errorf("second writeClosed.Swap(true) = %v; want true", old)
	}
}

// Cross-goroutine smoke: producer + consumer over a channel + EOF
// signal. Mirrors what the agent's STREAM_TYPE_PLUGIN_STREAM
// dispatcher does (when it exists): one goroutine drains the wire
// into ctx.inbound, the wasm-driven consumer reads from it; one
// goroutine drains ctx.outbound to the wire, the wasm-driven
// producer writes to it.
func TestStreamCtx_ProducerConsumer(t *testing.T) {
	r := newStreamRegistry()
	s := r.open()

	var produced int32
	go func() {
		for i := 0; i < 4; i++ {
			s.inbound <- []byte{byte(i)}
			atomic.AddInt32(&produced, 1)
		}
		close(s.inbound)
	}()

	got := 0
	for b := range s.inbound {
		if int(b[0]) != got {
			t.Errorf("frame %d = %d, want %d", got, b[0], got)
		}
		got++
	}
	if got != 4 || atomic.LoadInt32(&produced) != 4 {
		t.Errorf("got=%d produced=%d, want 4 each", got, produced)
	}
}
