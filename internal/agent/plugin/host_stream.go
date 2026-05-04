package plugin

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"io"
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
//
// The legacy-wasm-bridge path (DispatchLegacyWasmStream) leaves the
// channels nil and instead sets `wire` to the raw agent stream so
// host_link_write_frame can write length-prefixed proto frames
// directly to the wire — matching the byte-for-byte format the
// pre-migration Go handlers emitted (link.WriteFrame). Pump-mode
// (PLUGIN_STREAM) leaves `wire` nil and uses the channels.
//
// Exactly one of {channels, wire} is populated for any given
// streamCtx; host fns route based on which is non-nil.
type streamCtx struct {
	id          uint32
	inbound     chan []byte // pump-mode: wire → wasm reader
	outbound    chan []byte // pump-mode: wasm writer → wire
	writeClosed atomic.Bool // host_stream_close → outbound closed; subsequent writes fail
	inboundEOF  atomic.Bool // wire EOF observed; subsequent reads return empty

	// wire is the raw agent stream for the legacy-wasm-bridge path.
	// Non-nil → host_link_write_frame and host_link_read_frame
	// operate directly on this. Bidirectional (io.ReadWriter) so
	// plugins like sys-file-write can both ack with an outbound
	// frame and consume incoming chunked-input frames over the same
	// stream. Synchronised by extism's per-plugin call serialisation:
	// at most one wasm call is in flight per loaded plugin, so direct
	// reads/writes don't race with each other.
	wire io.ReadWriter
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

// hostStreamRead pulls one frame off the inbound stream. Wire shape:
// envelope.Data is a base64-encoded JSON string; the plugin's PDK
// decodes it on the wasm side. Returns empty data on EOF.
//
// Two backend modes:
//   - legacy-wasm-bridge (s.wire != nil): reads a length-prefixed
//     frame straight off the wire (same protocol as the soon-to-be-
//     retired host_link_read_frame). Matches the wire shape the
//     server-side legacy stream-type handlers (FILE_WRITE etc) emit.
//   - pump mode (s.wire == nil): pulls one already-decoded chunk off
//     the inbound channel that pumpInboundFrames feeds.
//
// On the pump-mode path we don't short-circuit on s.inboundEOF here
// even though the pump sets it on close: the pump may have pushed a
// final DATA chunk into the cap=1 buffer and *then* closed the
// channel + set the flag, so reading the flag without first draining
// would lose that last chunk. The receive's `!ok` detection on a
// closed channel is correct regardless of buffer state.
func (pctx *pluginCtx) hostStreamRead(ctx context.Context, p *extism.CurrentPlugin, stack []uint64) {
	s := pctx.activeStream()
	if s == nil {
		returnEnvelope(p, stack, failed("stream_not_active"))
		return
	}
	if s.wire != nil {
		var hdr [4]byte
		if _, err := io.ReadFull(s.wire, hdr[:]); err != nil {
			if err == io.EOF {
				returnEnvelope(p, stack, envelope{Ok: true, Data: rawJSONString("")})
				return
			}
			returnEnvelope(p, stack, failed("read_header: "+err.Error()))
			return
		}
		length := binary.BigEndian.Uint32(hdr[:])
		if length > linkFrameMaxBytes {
			returnEnvelope(p, stack, failed("frame_too_large"))
			return
		}
		body := make([]byte, length)
		if length > 0 {
			if _, err := io.ReadFull(s.wire, body); err != nil {
				returnEnvelope(p, stack, failed("read_body: "+err.Error()))
				return
			}
		}
		returnEnvelope(p, stack, envelope{Ok: true, Data: rawJSONString(base64.StdEncoding.EncodeToString(body))})
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

// hostStreamWrite emits one frame on the outbound side of the stream.
//
// Two backend modes:
//   - legacy-wasm-bridge (s.wire != nil): writes a length-prefixed
//     frame straight to the wire. Matches the wire format the
//     server-side legacy stream-type handlers (FILE_READ, PROCESS_OPEN,
//     etc) read with `link.ReadFrame(stream, &TypedResponse{})`.
//     Identical effect to the soon-to-be-retired host_link_write_frame.
//   - pump mode (s.wire == nil): enqueues onto the outbound channel
//     that pumpOutboundFrames wraps in PluginStreamFrame envelopes.
//     Blocking on a full channel is the natural backpressure mechanism.
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
	if s.wire != nil {
		if len(raw) > linkFrameMaxBytes {
			returnEnvelope(p, stack, failed("frame_too_large"))
			return
		}
		var hdr [4]byte
		binary.BigEndian.PutUint32(hdr[:], uint32(len(raw)))
		if _, err := s.wire.Write(hdr[:]); err != nil {
			returnEnvelope(p, stack, failed("write_header: "+err.Error()))
			return
		}
		if len(raw) > 0 {
			if _, err := s.wire.Write(raw); err != nil {
				returnEnvelope(p, stack, failed("write_body: "+err.Error()))
				return
			}
		}
		returnEnvelope(p, stack, envelope{Ok: true})
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
	// In legacy-wasm-bridge mode the wasm writes directly to the
	// wire and there's no channel to close — the underlying stream's
	// lifetime is owned by the dispatcher. The call still succeeds
	// so plugin code that calls host_stream_close defensively at end
	// of its method body keeps working in both modes.
	if s.wire != nil {
		s.writeClosed.Store(true)
		returnEnvelope(p, stack, envelope{Ok: true})
		return
	}
	if !s.writeClosed.Swap(true) {
		close(s.outbound)
	}
	returnEnvelope(p, stack, envelope{Ok: true})
}

// hostLinkWriteFrame writes a length-prefixed frame straight to the
// wire of a legacy-wasm-bridge stream. The wasm side is responsible
// for marshalling the inner proto (e.g. v2pb.FileReadResponse) and
// passing the marshalled bytes as the argument; this fn prepends
// the 4-byte big-endian length prefix that link.ReadFrame on the
// peer side expects, matching the bytes the legacy Go handlers used
// to emit via link.WriteFrame.
//
// In pump-mode (PLUGIN_STREAM) the host fn returns
// "stream_not_legacy_bridge" — those plugins should write via the
// pumped host_stream_write instead so the pumps can wrap each chunk
// in a PluginStreamFrame.
func (pctx *pluginCtx) hostLinkWriteFrame(_ context.Context, p *extism.CurrentPlugin, stack []uint64) {
	s := pctx.activeStream()
	if s == nil {
		returnEnvelope(p, stack, failed("stream_not_active"))
		return
	}
	if s.wire == nil {
		returnEnvelope(p, stack, failed("stream_not_legacy_bridge"))
		return
	}
	if s.writeClosed.Load() {
		returnEnvelope(p, stack, failed("stream_write_closed"))
		return
	}
	body, err := p.ReadBytes(stack[0])
	if err != nil {
		returnEnvelope(p, stack, failed("read_bytes: "+err.Error()))
		return
	}
	if len(body) > linkFrameMaxBytes {
		returnEnvelope(p, stack, failed("frame_too_large"))
		return
	}
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(len(body)))
	if _, err := s.wire.Write(hdr[:]); err != nil {
		returnEnvelope(p, stack, failed("write_header: "+err.Error()))
		return
	}
	if len(body) > 0 {
		if _, err := s.wire.Write(body); err != nil {
			returnEnvelope(p, stack, failed("write_body: "+err.Error()))
			return
		}
	}
	returnEnvelope(p, stack, envelope{Ok: true})
}

// linkFrameMaxBytes mirrors internal/link.FrameMaxBytes to keep the
// host fn's bound aligned with the wire-side reader's check. Hard-
// coded rather than imported to avoid pulling internal/link into
// host_stream.go's import set just for the constant.
const linkFrameMaxBytes = 1 << 20 // 1 MiB

// hostLinkReadFrame reads one length-prefixed frame from the wire
// of a legacy-wasm-bridge stream and returns the body bytes (raw,
// base64-encoded inside the JSON envelope so binary data round-
// trips cleanly). Mirrors internal/link.ReadFrame: 4-byte big-
// endian length header + body.
//
// Returns ok:true with empty data when the wire EOFs cleanly (caller
// loop should terminate). On a non-EOF read error returns ok:false
// with the error in the envelope. In pump-mode (PLUGIN_STREAM)
// returns "stream_not_legacy_bridge".
func (pctx *pluginCtx) hostLinkReadFrame(_ context.Context, p *extism.CurrentPlugin, stack []uint64) {
	s := pctx.activeStream()
	if s == nil {
		returnEnvelope(p, stack, failed("stream_not_active"))
		return
	}
	if s.wire == nil {
		returnEnvelope(p, stack, failed("stream_not_legacy_bridge"))
		return
	}
	var hdr [4]byte
	if _, err := io.ReadFull(s.wire, hdr[:]); err != nil {
		if err == io.EOF {
			returnEnvelope(p, stack, envelope{Ok: true, Data: rawJSONString("")})
			return
		}
		returnEnvelope(p, stack, failed("read_header: "+err.Error()))
		return
	}
	length := binary.BigEndian.Uint32(hdr[:])
	if length > linkFrameMaxBytes {
		returnEnvelope(p, stack, failed("frame_too_large"))
		return
	}
	body := make([]byte, length)
	if length > 0 {
		if _, err := io.ReadFull(s.wire, body); err != nil {
			returnEnvelope(p, stack, failed("read_body: "+err.Error()))
			return
		}
	}
	returnEnvelope(p, stack, envelope{Ok: true, Data: rawJSONString(base64.StdEncoding.EncodeToString(body))})
}
