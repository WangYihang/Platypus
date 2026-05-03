package plugin

import (
	"sync/atomic"
	"testing"
)

// Pure-Go unit tests for the streamCtx state machine. The wasm-side
// host_stream_* host functions go through extism's runtime, which
// we don't exercise here (mocking the extism.CurrentPlugin handle
// is impractical); instead we cover the channel + atomics shape
// that the host functions delegate to. End-to-end coverage with a
// real wasm-streaming plugin is the dispatchWasmStream tests.
//
// streamRegistry was removed in favour of pctx.activeStream — see
// host_stream_active_test.go for its lifecycle tests.

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
	// Second Swap returns true — the host_stream_close path uses
	// this to detect "already closed" without an extra mutex.
	if old := s.writeClosed.Swap(true); !old {
		t.Errorf("second writeClosed.Swap(true) = %v; want true", old)
	}
}

// Cross-goroutine smoke: producer + consumer over a channel + EOF
// signal. Mirrors what the dispatcher runs at production time —
// one goroutine drains the wire into ctx.inbound + closes when EOF;
// the wasm-driven consumer (via host_stream_read) reads from it.
func TestStreamCtx_ProducerConsumer(t *testing.T) {
	s := &streamCtx{
		id:       1,
		inbound:  make(chan []byte, 1),
		outbound: make(chan []byte, 1),
	}

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
