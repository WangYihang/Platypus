package plugin

import (
	"context"
	"encoding/base64"
	"sync"
	"sync/atomic"

	extism "github.com/extism/go-sdk"
)

// host_stream_read / host_stream_write / host_stream_close are the
// primitives a wasm-streaming plugin uses to do IO mid-stream. Each
// operates on a "stream id" — a small integer the host hands the
// plugin via the inbound metadata payload (see PluginStreamRequest).
// The stream itself is a pair of byte channels owned by the agent;
// the wasm side never sees raw OS file descriptors.
//
// Lifecycle (Phase-2 design — no plugin uses these primitives yet,
// but the surface is wired so the next commit can migrate file_read
// or process_open without touching this file):
//
//   1. Agent's STREAM_TYPE_PLUGIN_STREAM dispatcher allocates a
//      *streamCtx via streams.open() and stashes the id in the
//      inbound PluginStreamRequest payload (a small int the wasm
//      method reads from extism input).
//   2. The dispatcher invokes the wasm export named in the
//      manifest's streams[].host_handler="wasm:<method>" marker.
//   3. The wasm export calls host_stream_read(stream_id) to consume
//      incoming bytes (one PluginStreamFrame at a time). Returns
//      empty data when the inbound side is at EOF.
//   4. host_stream_write(stream_id, bytes) sends a PluginStreamFrame
//      outbound. Backpressure = blocking on the channel.
//   5. host_stream_close(stream_id) signals "done writing"; the
//      agent emits KIND_EOF on the wire.
//   6. The wasm export returns; the agent collects pending output,
//      emits a terminal frame, closes the wire stream.
//
// Today's state: the primitives are registered (every plugin sees
// them in its host imports, capability-gated). DispatchStream still
// goes through the legacy host-provider path for every stream type
// because no plugin manifest references "wasm:" as host_handler yet.
// The migration of one stream type (file_read is the simplest
// candidate — unidirectional bytes, no PTY interleaving) is a
// follow-up commit.

// streamCtx is per-active-stream state. One streamCtx per
// in-flight wasm-streaming call.
type streamCtx struct {
	id          uint32
	inbound     chan []byte // wire → plugin
	outbound    chan []byte // plugin → wire
	writeClosed atomic.Bool
	inboundEOF  atomic.Bool
}

// streamRegistry is the per-plugin-instance map of active streams.
// Stored on pluginCtx so each plugin's wasm can only address its own
// streams (cross-plugin id reuse is impossible — different
// pluginCtx, different registry).
type streamRegistry struct {
	mu      sync.Mutex
	streams map[uint32]*streamCtx
	nextID  uint32
}

func newStreamRegistry() *streamRegistry {
	return &streamRegistry{streams: make(map[uint32]*streamCtx)}
}

// open allocates a new streamCtx with channels of capacity 1. The
// returned id is what the wasm side passes to host_stream_*.
func (r *streamRegistry) open() *streamCtx {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextID++
	id := r.nextID
	s := &streamCtx{
		id:       id,
		inbound:  make(chan []byte, 1),
		outbound: make(chan []byte, 1),
	}
	r.streams[id] = s
	return s
}

func (r *streamRegistry) get(id uint32) *streamCtx {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.streams[id]
}

// close pulls the stream out of the registry. Safe to call multiple
// times — second call sees no entry and returns silently.
func (r *streamRegistry) closeID(id uint32) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.streams, id)
}

// hostStreamRead pulls one frame off the inbound channel. Returns
// empty data on EOF. Wire shape: envelope.Data is a base64-of-bytes
// JSON string; the plugin's PDK decodes it.
func (pctx *pluginCtx) hostStreamRead(ctx context.Context, p *extism.CurrentPlugin, stack []uint64) {
	id := uint32(stack[0])
	s := pctx.streams.get(id)
	if s == nil {
		returnEnvelope(p, stack, failed("stream_not_found"))
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
// The wasm side passes the bytes already base64-decoded; this host
// fn receives raw bytes via extism's PDK.
func (pctx *pluginCtx) hostStreamWrite(ctx context.Context, p *extism.CurrentPlugin, stack []uint64) {
	id := uint32(stack[0])
	s := pctx.streams.get(id)
	if s == nil {
		returnEnvelope(p, stack, failed("stream_not_found"))
		return
	}
	if s.writeClosed.Load() {
		returnEnvelope(p, stack, failed("stream_write_closed"))
		return
	}
	raw, err := p.ReadBytes(stack[1])
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
	id := uint32(stack[0])
	s := pctx.streams.get(id)
	if s == nil {
		returnEnvelope(p, stack, failed("stream_not_found"))
		return
	}
	if !s.writeClosed.Swap(true) {
		close(s.outbound)
	}
	returnEnvelope(p, stack, envelope{Ok: true})
}
