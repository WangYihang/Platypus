package plugin

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TDD for the per-stream orchestrator that the production agent
// stream dispatcher will sit on top of. runActiveStream's job is
// the choreography only:
//
//   1. allocate a fresh streamCtx + install it as pctx.activeStream
//   2. invoke the wasm method (via injectable invokerFn — production
//      wraps extism.Plugin.CallWithContext) on a goroutine
//   3. when invoker returns, clear pctx.activeStream + close the
//      outbound channel (idempotent — covers the case where the
//      plugin already called host_stream_close)
//   4. surface invoker's (output, err) via the returned channel
//
// The byte-pumping (wire ↔ s.inbound / s.outbound channels) is
// the dispatcher's responsibility, not runActiveStream's. Keeping
// these layers split makes runActiveStream pure-state-machine
// and unit-testable without a wire transport.

// invokerFn is the signature the runActiveStream caller injects.
// In production this is `extism.Plugin.CallWithContext` wrapped on
// *loaded; in tests it's a closure that fakes wasm by reading +
// writing through pctx.activeStream().
type invokerFn func(ctx context.Context, method string, input []byte) ([]byte, error)

type invokerResult struct {
	output []byte
	err    error
}

func TestRunActiveStream_InvokerCalledWithMethodAndPayload(t *testing.T) {
	pctx := &pluginCtx{}
	var gotMethod string
	var gotPayload []byte
	inv := func(_ context.Context, method string, input []byte) ([]byte, error) {
		gotMethod = method
		gotPayload = append([]byte(nil), input...)
		return []byte("response-bytes"), nil
	}

	_, done := runActiveStream(context.Background(), pctx, "file_read", []byte("the-request"), inv)
	res := <-done

	if gotMethod != "file_read" {
		t.Errorf("method = %q, want file_read", gotMethod)
	}
	if string(gotPayload) != "the-request" {
		t.Errorf("payload = %q, want the-request", gotPayload)
	}
	if res.err != nil {
		t.Errorf("invoker err = %v", res.err)
	}
	if string(res.output) != "response-bytes" {
		t.Errorf("output = %q, want response-bytes", res.output)
	}
}

func TestRunActiveStream_ActiveStreamSetDuringInvoke(t *testing.T) {
	pctx := &pluginCtx{}
	var sawActive *streamCtx
	inv := func(_ context.Context, _ string, _ []byte) ([]byte, error) {
		// Snapshot what the host_stream_* primitives would observe.
		sawActive = pctx.activeStream()
		return nil, nil
	}
	_, done := runActiveStream(context.Background(), pctx, "x", nil, inv)
	<-done
	if sawActive == nil {
		t.Errorf("activeStream() was nil during invoker — primitives would see stream_not_active")
	}
	if pctx.activeStream() != nil {
		t.Errorf("activeStream() not cleared after invoker returned")
	}
}

func TestRunActiveStream_FakeWasmReadEcho(t *testing.T) {
	// Simulates a wasm method that reads one inbound chunk + writes it
	// back outbound + closes. The dispatcher hasn't pumped bytes onto
	// s.inbound; the test goroutine does that role here.
	pctx := &pluginCtx{}
	inv := func(_ context.Context, _ string, _ []byte) ([]byte, error) {
		s := pctx.activeStream()
		if s == nil {
			return nil, errors.New("no active stream")
		}
		chunk := <-s.inbound
		s.outbound <- chunk
		// Mark write-closed so the dispatcher's outbound drain
		// goroutine (production) sees end-of-data.
		if !s.writeClosed.Swap(true) {
			close(s.outbound)
		}
		return nil, nil
	}

	s, done := runActiveStream(context.Background(), pctx, "echo", nil, inv)
	s.inbound <- []byte("hello")
	got := <-s.outbound
	if string(got) != "hello" {
		t.Errorf("echo = %q, want hello", got)
	}
	// outbound is closed.
	if _, ok := <-s.outbound; ok {
		t.Errorf("outbound should be closed after host_stream_close")
	}
	res := <-done
	if res.err != nil {
		t.Errorf("err: %v", res.err)
	}
}

func TestRunActiveStream_OutboundClosedEvenWithoutHostStreamClose(t *testing.T) {
	// A plugin that returns without calling host_stream_close (e.g.
	// after an early error) MUST NOT leave the outbound drain
	// goroutine stuck. runActiveStream's defer must close the
	// channel so the dispatcher unblocks.
	pctx := &pluginCtx{}
	inv := func(_ context.Context, _ string, _ []byte) ([]byte, error) {
		return nil, errors.New("plugin returned early")
	}
	s, done := runActiveStream(context.Background(), pctx, "x", nil, inv)

	select {
	case _, ok := <-s.outbound:
		if ok {
			t.Errorf("outbound delivered an unexpected chunk")
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for outbound to close")
	}
	res := <-done
	if res.err == nil {
		t.Errorf("expected invoker err to propagate")
	}
}
