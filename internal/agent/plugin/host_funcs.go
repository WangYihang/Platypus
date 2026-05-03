package plugin

import (
	"context"
	"encoding/json"
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
// host_fs.go, host_exec.go, host_sysinfo.go, host_http.go.
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
		newHostFunc("host_sysinfo", []api.ValueType{},
			[]api.ValueType{api.ValueTypeI64}, pctx.hostSysInfo),
		newHostFunc("host_uname", []api.ValueType{},
			[]api.ValueType{api.ValueTypeI64}, pctx.hostUname),
		newHostFunc("host_http", []api.ValueType{api.ValueTypeI64},
			[]api.ValueType{api.ValueTypeI64}, pctx.hostHTTP),
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
