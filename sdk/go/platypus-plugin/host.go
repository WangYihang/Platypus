package platypus

import (
	"encoding/json"

	"github.com/extism/go-pdk"
)

// Log levels mirror internal/log + the wasm-side PDK constant set.
// Pass these to HostLog.
const (
	LogDebug int32 = 0
	LogInfo  int32 = 1
	LogWarn  int32 = 2
	LogError int32 = 3
)

// ---- host_log ------------------------------------------------------

//go:wasmimport platypus host_log
func _hostLog(level int32, msgPtr uint64) int32

// HostLog emits a log line through the agent's structured log + the
// per-plugin in-memory ring buffer the operator can pull via
// PluginGetLogsResponse.  CapLog is implicit on every plugin (no
// manifest entry needed).
//
// Returns the agent's status code: 0 = ok, non-zero = transport error
// (capability_denied is impossible here since CapLog is implicit).
func HostLog(level int32, msg string) int32 {
	mem := pdk.AllocateString(msg)
	return _hostLog(level, mem.Offset())
}

// LogInfof / LogWarnf / LogErrorf / LogDebugf are convenience wrappers
// around HostLog mirroring the fmt.Sprintf shape; they exist purely
// so plugin code stays concise. Avoid in tight loops if your tinygo
// build minds the fmt cost.
func LogInfof(format string, args ...any)  { HostLog(LogInfo, sprintf(format, args...)) }
func LogWarnf(format string, args ...any)  { HostLog(LogWarn, sprintf(format, args...)) }
func LogErrorf(format string, args ...any) { HostLog(LogError, sprintf(format, args...)) }
func LogDebugf(format string, args ...any) { HostLog(LogDebug, sprintf(format, args...)) }

// ---- host_uname (cap: sysinfo) -------------------------------------

//go:wasmimport platypus host_uname
func _hostUname(reqPtr uint64) uint64

// UnameResult mirrors the JSON shape internal/agent/plugin/host_uname.go
// returns inside the standard envelope.
type UnameResult struct {
	OS   string `json:"os"`
	Arch string `json:"arch"`
}

// HostUname returns the agent's reported OS + architecture. The
// plugin's manifest must declare `capabilities.sysinfo: true`.
func HostUname() (UnameResult, error) {
	empty := pdk.AllocateString("")
	out := _hostUname(empty.Offset())
	mem := pdk.FindMemory(out)
	env, err := decodeEnvelope(mem.ReadBytes())
	if err != nil {
		return UnameResult{}, err
	}
	if !env.Ok {
		return UnameResult{}, errString(env.Error)
	}
	var u UnameResult
	if err := json.Unmarshal(env.Data, &u); err != nil {
		return UnameResult{}, err
	}
	return u, nil
}

// ---- host_kv (cap: kv) ---------------------------------------------

//go:wasmimport platypus host_kv_get
func _hostKVGet(reqPtr uint64) uint64

//go:wasmimport platypus host_kv_put
func _hostKVPut(reqPtr uint64) uint64

// KVGet reads a value from the per-plugin KV namespace. Returns
// (value, true) on hit; (nil, false) on miss; error on transport or
// capability denial. Plugin manifest must declare `capabilities.kv: true`.
func KVGet(key string) ([]byte, bool, error) {
	body, err := json.Marshal(struct {
		Key string `json:"key"`
	}{Key: key})
	if err != nil {
		return nil, false, err
	}
	in := pdk.AllocateBytes(body)
	out := _hostKVGet(in.Offset())
	mem := pdk.FindMemory(out)
	env, err := decodeEnvelope(mem.ReadBytes())
	if err != nil {
		return nil, false, err
	}
	if !env.Ok {
		return nil, false, errString(env.Error)
	}
	var data struct {
		Found bool   `json:"found"`
		Value string `json:"value"` // base64
	}
	if err := json.Unmarshal(env.Data, &data); err != nil {
		return nil, false, err
	}
	if !data.Found {
		return nil, false, nil
	}
	v, err := decodeBase64(data.Value)
	if err != nil {
		return nil, false, err
	}
	return v, true, nil
}

// KVPut writes a value into the per-plugin KV namespace.
func KVPut(key string, value []byte) error {
	body, err := json.Marshal(struct {
		Key   string `json:"key"`
		Value string `json:"value"` // base64
	}{Key: key, Value: encodeBase64(value)})
	if err != nil {
		return err
	}
	in := pdk.AllocateBytes(body)
	out := _hostKVPut(in.Offset())
	mem := pdk.FindMemory(out)
	env, err := decodeEnvelope(mem.ReadBytes())
	if err != nil {
		return err
	}
	if !env.Ok {
		return errString(env.Error)
	}
	return nil
}
