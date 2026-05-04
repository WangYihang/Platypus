package plugin

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"

	extism "github.com/extism/go-sdk"
	"github.com/tetratelabs/wazero/api"
)

// hostNamespace is the wasm import namespace under which every host
// function this package registers lives. PDKs reference functions as
// `("platypus", "host_log")`, etc. Distinct from extism's own
// `extism:host/user` namespace so a plugin can't accidentally shadow
// our names by binding into the default namespace.
const hostNamespace = "platypus"

// pluginCtx carries the per-plugin state every host function needs to
// enforce its capability. One pluginCtx is built at plugin
// instantiation time and closed-over by all the host functions for
// that plugin; the same set of host functions is registered into the
// extism runtime per plugin (Extism's design).
type pluginCtx struct {
	id              string
	manifest        *Manifest
	granted         map[CapabilityID]bool
	stateDir        string
	logSink         *logBuffer
	now             func() time.Time
	correlationID   func() string // updated per-invocation; never nil
	maxFileReadSize int64
	maxKVValueSize  int64

	// activeStreamPtr holds the per-plugin active wasm-streaming
	// context. Set by the agent's dispatcher just before invoking
	// the wasm method, cleared after the method returns. The
	// host_stream_* primitives consult it via the activeStream() /
	// setActiveStream() / clearActiveStream() helpers.
	//
	// Only one stream is in flight per plugin instance because
	// extism.Plugin.Call is serialised on loaded.mu — so a single
	// pointer slot (no map / id scheme) is sufficient.
	activeStreamPtr atomic.Pointer[streamCtx]

	// processHandles is the per-plugin spawn table for the
	// host_process_* family. Cleared on plugin teardown via
	// reapProcessHandles so a crashed plugin doesn't leak a
	// background child. processHandleCounter is the monotonically-
	// increasing handle id; uint32 is plenty (the table is short-
	// lived per plugin instance and a busy plugin spawning thousands
	// of long-lived processes is out of scope).
	processMu             sync.Mutex
	processHandles        map[uint32]*processHandle
	processHandleCounter  uint32

	// netHandles is the per-plugin TCP-dial table for the host_net_*
	// family. Same lifecycle as processHandles: cleared on plugin
	// teardown via reapNetHandles so a crashed plugin doesn't leak a
	// live TCP conn.
	netMu             sync.Mutex
	netHandles        map[uint32]*netHandle
	netHandleCounter  uint32
}

// buildHostFunctions returns the slice handed to extism.NewPlugin for
// one plugin. Each function captures pctx; missing capabilities cause
// the host fn to return an error JSON to the plugin without doing the
// actual work.
//
// Wire conventions:
//   - Inputs that are bytes/strings come in as a single i64 offset into
//     the plugin's linear memory (allocated by the plugin's PDK, freed
//     by extism after the call returns).
//   - Outputs are JSON envelopes ({"ok":bool,"data":...,"error":"..."})
//     written back into plugin memory; the host returns the i64 offset.
//   - Numeric outputs (e.g. host_log returns nothing) are i32 status
//     codes: 0 = ok, non-zero = capability_denied or transport error.
//
// The JSON envelope is uniform on purpose — every PDK can decode the
// same shape, and we don't have to chase per-call protobuf descriptors
// at runtime.
//
// Per-family implementations live in host_log.go, host_kv.go,
// host_fs.go, host_exec.go, host_uname.go, host_http.go.
func (pctx *pluginCtx) buildHostFunctions() []extism.HostFunction {
	return []extism.HostFunction{
		newHostFunc("host_log", []api.ValueType{api.ValueTypeI32, api.ValueTypeI64},
			[]api.ValueType{api.ValueTypeI32}, pctx.hostLog),
		newHostFunc("host_kv_get", []api.ValueType{api.ValueTypeI64},
			[]api.ValueType{api.ValueTypeI64}, pctx.hostKVGet),
		newHostFunc("host_kv_put", []api.ValueType{api.ValueTypeI64, api.ValueTypeI64},
			[]api.ValueType{api.ValueTypeI64}, pctx.hostKVPut),
		newHostFunc("host_fs_read", []api.ValueType{api.ValueTypeI64},
			[]api.ValueType{api.ValueTypeI64}, pctx.hostFSRead),
		newHostFunc("host_fs_read_range", []api.ValueType{api.ValueTypeI64},
			[]api.ValueType{api.ValueTypeI64}, pctx.hostFSReadRange),
		newHostFunc("host_fs_listdir", []api.ValueType{api.ValueTypeI64},
			[]api.ValueType{api.ValueTypeI64}, pctx.hostFSListdir),
		newHostFunc("host_fs_stat", []api.ValueType{api.ValueTypeI64},
			[]api.ValueType{api.ValueTypeI64}, pctx.hostFSStat),
		newHostFunc("host_fs_write", []api.ValueType{api.ValueTypeI64},
			[]api.ValueType{api.ValueTypeI64}, pctx.hostFSWrite),
		newHostFunc("host_fs_mkdir", []api.ValueType{api.ValueTypeI64},
			[]api.ValueType{api.ValueTypeI64}, pctx.hostFSMkdir),
		newHostFunc("host_fs_chmod", []api.ValueType{api.ValueTypeI64},
			[]api.ValueType{api.ValueTypeI64}, pctx.hostFSChmod),
		newHostFunc("host_fs_delete", []api.ValueType{api.ValueTypeI64},
			[]api.ValueType{api.ValueTypeI64}, pctx.hostFSDelete),
		newHostFunc("host_fs_rename", []api.ValueType{api.ValueTypeI64},
			[]api.ValueType{api.ValueTypeI64}, pctx.hostFSRename),
		newHostFunc("host_exec", []api.ValueType{api.ValueTypeI64},
			[]api.ValueType{api.ValueTypeI64}, pctx.hostExec),
		newHostFunc("host_uname", []api.ValueType{},
			[]api.ValueType{api.ValueTypeI64}, pctx.hostUname),
		newHostFunc("host_http", []api.ValueType{api.ValueTypeI64},
			[]api.ValueType{api.ValueTypeI64}, pctx.hostHTTP),

		// Wasm-streaming primitives (groundwork for migrating
		// PROCESS_OPEN / FILE_* / TUNNEL_PULL off the legacy host-
		// provider claim path). Today no plugin manifest references
		// "wasm:" host_handler markers, so the dispatch path that
		// allocates per-stream channels + populates pctx.streams
		// hasn't been wired yet — the primitives return
		// "stream_not_found" until that lands.
		// Wasm-streaming primitives operate on the per-plugin
		// "active stream" slot the dispatcher sets before each
		// wasm method call. No stream id parameter — extism's
		// per-plugin call serialisation guarantees at most one
		// active stream per instance.
		newHostFunc("host_stream_read", []api.ValueType{},
			[]api.ValueType{api.ValueTypeI64}, pctx.hostStreamRead),
		newHostFunc("host_stream_write", []api.ValueType{api.ValueTypeI64},
			[]api.ValueType{api.ValueTypeI64}, pctx.hostStreamWrite),
		newHostFunc("host_stream_close", []api.ValueType{},
			[]api.ValueType{api.ValueTypeI64}, pctx.hostStreamClose),

		newHostFunc("host_fs_write_range", []api.ValueType{api.ValueTypeI64},
			[]api.ValueType{api.ValueTypeI64}, pctx.hostFSWriteRange),

		// Streaming process spawn — wasm migration target for the
		// legacy STREAM_TYPE_PROCESS_OPEN handler. Gated by CapProcess.
		newHostFunc("host_process_spawn", []api.ValueType{api.ValueTypeI64},
			[]api.ValueType{api.ValueTypeI64}, pctx.hostProcessSpawn),
		newHostFunc("host_process_relay", []api.ValueType{api.ValueTypeI64},
			[]api.ValueType{api.ValueTypeI64}, pctx.hostProcessRelay),
		newHostFunc("host_process_kill", []api.ValueType{api.ValueTypeI64},
			[]api.ValueType{api.ValueTypeI64}, pctx.hostProcessKill),

		// Outbound TCP dial — wasm migration target for the legacy
		// STREAM_TYPE_TUNNEL_PULL handler. Gated by CapNetDial.
		newHostFunc("host_net_dial", []api.ValueType{api.ValueTypeI64},
			[]api.ValueType{api.ValueTypeI64}, pctx.hostNetDial),
		newHostFunc("host_net_relay", []api.ValueType{api.ValueTypeI64},
			[]api.ValueType{api.ValueTypeI64}, pctx.hostNetRelay),
		newHostFunc("host_net_close", []api.ValueType{api.ValueTypeI64},
			[]api.ValueType{api.ValueTypeI64}, pctx.hostNetClose),
	}
}

func newHostFunc(name string, params, returns []api.ValueType,
	cb func(ctx context.Context, p *extism.CurrentPlugin, stack []uint64)) extism.HostFunction {
	hf := extism.NewHostFunctionWithStack(name, cb, params, returns)
	hf.SetNamespace(hostNamespace)
	return hf
}

// envelope is the JSON shape every host function returns. Ok=true with
// Data populated for the success case; Ok=false with Error populated
// for capability_denied / IO errors. Plugins MUST tolerate Data being
// any of {string, base64-bytes, object} — the per-call doc spells out
// which shape applies.
type envelope struct {
	Ok    bool            `json:"ok"`
	Data  json.RawMessage `json:"data,omitempty"`
	Error string          `json:"error,omitempty"`
}

// returnEnvelope writes the envelope into plugin memory and stuffs the
// resulting offset into stack[0]. On allocator failure we fall back to
// returning offset=0 — the plugin's PDK should treat that as a
// host-side error.
func returnEnvelope(p *extism.CurrentPlugin, stack []uint64, env envelope) {
	b, err := json.Marshal(env)
	if err != nil {
		stack[0] = 0
		return
	}
	off, err := p.WriteBytes(b)
	if err != nil {
		stack[0] = 0
		return
	}
	stack[0] = off
}

func denied(reason string) envelope { return envelope{Ok: false, Error: "capability_denied: " + reason} }
func failed(reason string) envelope { return envelope{Ok: false, Error: reason} }
func okData(d any) envelope {
	b, _ := json.Marshal(d)
	return envelope{Ok: true, Data: b}
}

// readStringArg pulls one string out of a single-i64 stack slot.
func readStringArg(p *extism.CurrentPlugin, slot uint64) (string, error) {
	return p.ReadString(slot)
}

// rawJSONString quotes s as a JSON string literal so it can be embedded
// in envelope.Data without re-marshalling the entire payload through
// json.Marshal. Used by host_kv_get / host_fs_read where Data is a raw
// string (not a structured object) and the value may be large.
func rawJSONString(s string) json.RawMessage {
	b, _ := json.Marshal(s)
	return json.RawMessage(b)
}
