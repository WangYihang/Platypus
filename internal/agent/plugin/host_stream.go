package plugin

import (
	"context"
	"encoding/base64"
	"sync/atomic"

	extism "github.com/extism/go-sdk"
)

// host_stream_read / host_stream_write / host_stream_close are the
// primitives a wasm-streaming plugin uses to do IO mid-stream.
//
// The wasm side does NOT pass a stream id — the agent's dispatcher
// sets pctx.activeStream just before invoking the wasm method, and
// each host fn dereferences it. extism.Plugin is not goroutine-safe
// (we already serialise on loaded.mu) so at most one stream is in
// flight per plugin instance + a single active-stream slot is
// sufficient.
//
// Lifecycle (Phase-2; the dispatchWasmStream landing this surface
// is the next TDD step — today's host fns return stream_not_found
// because nothing sets activeStream):
//
//   1. Agent's stream dispatcher receives FILE_READ (or any other
//      type the manifest's claim resolved to "wasm:method").
//   2. Allocates a *streamCtx, calls pctx.setActiveStream(s).
//   3. Spawns two pumps: wire→s.inbound, s.outbound→wire.
//   4. Calls extism.Plugin.Call(method, marshalled metadata).
//   5. Wasm reads via host_stream_read, writes via
//      host_stream_write, signals end-of-output via
//      host_stream_close.
//   6. Wasm returns; dispatcher closes s.inbound, joins pumps,
//      writes terminal frame, calls pctx.clearActiveStream().

// streamCtx is per-active-stream state. Two channels (cap=1 for
// natural backpressure) + atomics for the closed/EOF flags the
// host fns consult.
type streamCtx struct {
	id          uint32
	inbound     chan []byte // wire → wasm reader
	outbound    chan []byte // wasm writer → wire
	writeClosed atomic.Bool // host_stream_close → outbound closed; subsequent writes fail
	inboundEOF  atomic.Bool // wire EOF observed; subsequent reads return empty
}

// activeStream returns the current per-plugin active stream, or nil
// when no stream is in flight (the universal state today, and the
// resting state between calls).
func (pctx *pluginCtx) activeStream() *streamCtx {
	if pctx == nil {
		return nil
	}
	return pctx.activeStreamPtr.Load()
}

// setActiveStream stashes the per-stream state for the upcoming
// wasm method call. Called by the dispatcher under loaded.mu (the
// extism.Plugin's own serialisation lock) so concurrent setters are
// not possible in production.
func (pctx *pluginCtx) setActiveStream(s *streamCtx) {
	pctx.activeStreamPtr.Store(s)
}

// clearActiveStream releases the slot when the wasm method returns.
// Idempotent — a double-clear is a no-op.
func (pctx *pluginCtx) clearActiveStream() {
	pctx.activeStreamPtr.Store(nil)
}

// hostStreamRead pulls one frame off the inbound channel. Returns
// empty data on EOF. Wire shape: envelope.Data is a base64-encoded
// JSON string; the plugin's PDK decodes it on the wasm side.
func (pctx *pluginCtx) hostStreamRead(ctx context.Context, p *extism.CurrentPlugin, stack []uint64) {
	s := pctx.activeStream()
	if s == nil {
		returnEnvelope(p, stack, failed("stream_not_active"))
		return
	}
	if s.inboundEOF.Load() {
		returnEnvelope(p, stack, envelope{Ok: true, Data: rawJSONString("")})
		return
	}
	select {
	case b, ok := <-s.inbound:
		if !ok {
			s.inboundEOF.Store(true)
			returnEnvelope(p, stack, envelope{Ok: true, Data: rawJSONString("")})
			return
		}
		returnEnvelope(p, stack, envelope{Ok: true, Data: rawJSONString(base64.StdEncoding.EncodeToString(b))})
	case <-ctx.Done():
		returnEnvelope(p, stack, failed("ctx_cancelled"))
	}
}

// hostStreamWrite enqueues one frame onto the outbound channel.
// Blocking on a full channel is the natural backpressure mechanism.
func (pctx *pluginCtx) hostStreamWrite(ctx context.Context, p *extism.CurrentPlugin, stack []uint64) {
	s := pctx.activeStream()
	if s == nil {
		returnEnvelope(p, stack, failed("stream_not_active"))
		return
	}
	if s.writeClosed.Load() {
		returnEnvelope(p, stack, failed("stream_write_closed"))
		return
	}
	raw, err := p.ReadBytes(stack[0])
	if err != nil {
		returnEnvelope(p, stack, failed("read_bytes: "+err.Error()))
		return
	}
	select {
	case s.outbound <- raw:
		returnEnvelope(p, stack, envelope{Ok: true})
	case <-ctx.Done():
		returnEnvelope(p, stack, failed("ctx_cancelled"))
	}
}

// hostStreamClose marks the outbound side EOF. Subsequent writes
// return stream_write_closed. The agent's bridge converts the
// channel close into a terminal KIND_EOF frame on the wire.
func (pctx *pluginCtx) hostStreamClose(_ context.Context, p *extism.CurrentPlugin, stack []uint64) {
	s := pctx.activeStream()
	if s == nil {
		returnEnvelope(p, stack, failed("stream_not_active"))
		return
	}
	if !s.writeClosed.Swap(true) {
		close(s.outbound)
	}
	returnEnvelope(p, stack, envelope{Ok: true})
}
