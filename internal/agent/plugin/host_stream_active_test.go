package plugin

import (
	"testing"
	"time"
)

// TDD for the simplified "active stream" API: instead of the wasm
// side passing a stream id to host_stream_*, the dispatcher sets
// the per-plugin activeStream pointer before calling the wasm
// method, and host_stream_* dereferences it. extism plugin
// instances aren't goroutine-safe (we already serialise on
// loaded.mu) so a single active stream per plugin instance is
// safe.
//
// These tests exercise the activeStream lifecycle directly — set,
// observe, clear — without going through the wasm runtime. The
// host fn integration is covered by the dispatchWasmStream tests
// (next TDD step).

func TestActiveStream_SetAndClear(t *testing.T) {
	pctx := &pluginCtx{}
	if pctx.activeStream() != nil {
		t.Fatalf("default activeStream != nil")
	}

	s := &streamCtx{
		id:       1,
		inbound:  make(chan []byte, 1),
		outbound: make(chan []byte, 1),
	}
	pctx.setActiveStream(s)
	if got := pctx.activeStream(); got != s {
		t.Errorf("activeStream() != set value: %v vs %v", got, s)
	}

	pctx.clearActiveStream()
	if pctx.activeStream() != nil {
		t.Errorf("activeStream still set after clear")
	}
}

func TestActiveStream_Concurrent_LastWriteWins(t *testing.T) {
	// activeStream is set under loaded.mu by the dispatcher in
	// production. The atomic load/store guarantees a reader sees a
	// consistent pointer. Test that race with -race finds nothing.
	pctx := &pluginCtx{}
	done := make(chan struct{})
	a := &streamCtx{id: 1}
	b := &streamCtx{id: 2}

	go func() {
		for i := 0; i < 100; i++ {
			if i%2 == 0 {
				pctx.setActiveStream(a)
			} else {
				pctx.setActiveStream(b)
			}
		}
		close(done)
	}()
	for i := 0; i < 100; i++ {
		_ = pctx.activeStream() // value irrelevant; race detector checks
		time.Sleep(time.Microsecond)
	}
	<-done
	pctx.clearActiveStream()
}
