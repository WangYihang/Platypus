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
func _hostUname() uint64

// UnameResult mirrors the JSON shape internal/agent/plugin/host_uname.go
// returns inside the standard envelope.
type UnameResult struct {
	OS   string `json:"os"`
	Arch string `json:"arch"`
}

// HostUname returns the agent's reported OS + architecture. The
// plugin's manifest must declare `capabilities.sysinfo: true`.
func HostUname() (UnameResult, error) {
	out := _hostUname()
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

// ---- host_fs_read / host_fs_listdir / host_fs_stat (cap: fs.read) --

//go:wasmimport platypus host_fs_read
func _hostFSRead(pathPtr uint64) uint64

// HostFSRead returns the entire contents of `path`, capped at the
// agent's max_file_read_size (default 4 MiB). Plugin manifest must
// declare `capabilities.fs.read.paths` covering the file.
//
// host_fs_read takes a raw string (no JSON envelope on input);
// returns envelope with Data being a JSON-string of the file body.
func HostFSRead(path string) ([]byte, error) {
	in := pdk.AllocateString(path)
	out := _hostFSRead(in.Offset())
	mem := pdk.FindMemory(out)
	env, err := decodeEnvelope(mem.ReadBytes())
	if err != nil {
		return nil, err
	}
	if !env.Ok {
		return nil, errString(env.Error)
	}
	var contents string
	if err := json.Unmarshal(env.Data, &contents); err != nil {
		return nil, err
	}
	return []byte(contents), nil
}

// HostFSReadString is the convenience wrapper: same as HostFSRead but
// returns the body as a string for cleaner /proc-style parsing.
func HostFSReadString(path string) (string, error) {
	b, err := HostFSRead(path)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

//go:wasmimport platypus host_fs_listdir
func _hostFSListdir(pathPtr uint64) uint64

// FSListEntry is the wire shape of one entry in a directory listing.
// Mirrors fsListEntry in internal/agent/plugin/host_fs.go.
type FSListEntry struct {
	Name      string `json:"name"`
	IsDir     bool   `json:"is_dir"`
	Size      int64  `json:"size"`
	MTimeUnix int64  `json:"mtime_unix"`
	Mode      uint32 `json:"mode"`
}

// HostFSListDir lists `path`'s direct children. Same allowlist
// semantics as HostFSRead.
func HostFSListDir(path string) ([]FSListEntry, error) {
	in := pdk.AllocateString(path)
	out := _hostFSListdir(in.Offset())
	mem := pdk.FindMemory(out)
	env, err := decodeEnvelope(mem.ReadBytes())
	if err != nil {
		return nil, err
	}
	if !env.Ok {
		return nil, errString(env.Error)
	}
	var entries []FSListEntry
	if err := json.Unmarshal(env.Data, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

// ---- host_kv (cap: kv) ---------------------------------------------

//go:wasmimport platypus host_kv_get
func _hostKVGet(keyPtr uint64) uint64

//go:wasmimport platypus host_kv_put
func _hostKVPut(keyPtr, valPtr uint64) uint64

// KVGet reads a value from the per-plugin KV namespace. Returns
// (value, true) on hit; (nil, false) on miss; error on transport or
// capability denial. Plugin manifest must declare `capabilities.kv: true`.
//
// The key is passed as a raw string (not a JSON envelope) — see
// internal/agent/plugin/host_kv.go's readStringArg(stack[0]).
func KVGet(key string) ([]byte, bool, error) {
	in := pdk.AllocateString(key)
	out := _hostKVGet(in.Offset())
	mem := pdk.FindMemory(out)
	env, err := decodeEnvelope(mem.ReadBytes())
	if err != nil {
		return nil, false, err
	}
	if !env.Ok {
		// Convention: a miss surfaces as ok=false with
		// error="not_found"; treat that distinctly from transport
		// errors so callers can `if err == ErrNotFound` cleanly.
		if env.Error == "not_found" {
			return nil, false, nil
		}
		return nil, false, errString(env.Error)
	}
	var v string
	if err := json.Unmarshal(env.Data, &v); err != nil {
		return nil, false, err
	}
	return []byte(v), true, nil
}

// KVPut writes a value into the per-plugin KV namespace. Both args
// are raw strings (host accepts any UTF-8; binary blobs should be
// caller-encoded).
func KVPut(key string, value []byte) error {
	keyMem := pdk.AllocateString(key)
	valMem := pdk.AllocateBytes(value)
	out := _hostKVPut(keyMem.Offset(), valMem.Offset())
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
