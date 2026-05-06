package platypus

import (
	"strconv"
	"strings"

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

// ---- host_stream_* — streaming plugin primitives -------------------
//
// In raw-wire mode (DispatchLegacyWasmStream — typed stream types
// like FILE_READ/WRITE/PROCESS_OPEN/TUNNEL_PULL) host_stream_write
// emits one length-prefixed frame straight to the wire and
// host_stream_read pulls the next length-prefixed frame off the wire.
//
// In pump mode (DispatchPluginStream) the same fns enqueue/dequeue
// against the dispatcher's PluginStreamFrame envelope pumps; the
// SDK call sites are identical.

//go:wasmimport platypus host_stream_write
func _hostStreamWrite(bytesPtr uint64) uint64

//go:wasmimport platypus host_stream_read
func _hostStreamRead() uint64

//go:wasmimport platypus host_stream_close
func _hostStreamClose() uint64

// HostStreamWrite emits one frame of bytes on the active stream.
// Returns nil on success, error on transport / write_closed /
// stream_not_active.
func HostStreamWrite(body []byte) error {
	in := pdk.AllocateBytes(body)
	out := _hostStreamWrite(in.Offset())
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

// HostStreamRead reads one frame from the active stream. Returns
// (nil, nil) on EOF (the wire's clean close); (data, nil) on a
// non-empty frame; (nil, error) on transport failure.
func HostStreamRead() ([]byte, error) {
	out := _hostStreamRead()
	mem := pdk.FindMemory(out)
	env, err := decodeEnvelope(mem.ReadBytes())
	if err != nil {
		return nil, err
	}
	if !env.Ok {
		return nil, errString(env.Error)
	}
	if len(env.Data) == 0 {
		return nil, nil
	}
	// envelope.Data carries a base64-encoded JSON string.
	p := jsonParser{buf: env.Data}
	b64, err := p.readString()
	if err != nil {
		return nil, err
	}
	return decodeBase64(b64)
}

// HostStreamClose marks the outbound side EOF. Idempotent;
// subsequent writes return write_closed.
func HostStreamClose() error {
	out := _hostStreamClose()
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

// ---- host_net (cap: net.dial) --------------------------------------

//go:wasmimport platypus host_net_dial
func _hostNetDial(specPtr uint64) uint64

//go:wasmimport platypus host_net_relay
func _hostNetRelay(reqPtr uint64) uint64

//go:wasmimport platypus host_net_close
func _hostNetClose(reqPtr uint64) uint64

// NetDialResult mirrors the JSON returned inside the envelope.
type NetDialResult struct {
	Handle       uint32 `json:"handle"`
	ResolvedAddr string `json:"resolved_addr"`
}

// NetDial opens a TCP connection to `target` (host:port). Plugin
// manifest must declare `capabilities.net.dial.targets` covering
// the destination. dialTimeoutMs=0 falls back to the host default.
func NetDial(target string, dialTimeoutMs uint32) (NetDialResult, error) {
	// Hand-build the spec JSON; same TinyGo-no-encoding/json
	// rationale as the rest of the SDK.
	body := `{"target":` + EncodeJSONString(target) +
		`,"dial_timeout_ms":` + strconvUint32(dialTimeoutMs) + `}`
	in := pdk.AllocateString(body)
	out := _hostNetDial(in.Offset())
	mem := pdk.FindMemory(out)
	env, err := decodeEnvelope(mem.ReadBytes())
	if err != nil {
		return NetDialResult{}, err
	}
	if !env.Ok {
		return NetDialResult{}, errString(env.Error)
	}
	var r NetDialResult
	p := jsonParser{buf: env.Data}
	err = parseObject(&p, []fieldHandler{
		{"handle", func(p *jsonParser) error {
			v, err := p.readUint64()
			if err == nil {
				r.Handle = uint32(v)
			}
			return err
		}},
		{"resolved_addr", func(p *jsonParser) error {
			s, err := p.readString()
			if err == nil {
				r.ResolvedAddr = s
			}
			return err
		}},
	})
	if err != nil {
		return NetDialResult{}, err
	}
	return r, nil
}

// NetRelay hands the dialed conn off to the host's bidirectional
// pump and blocks until either side closes. The host pumps wire ↔
// TCP raw bytes; plugins typically invoke this immediately after
// the response header has been emitted via HostStreamWrite.
func NetRelay(handle uint32) error {
	// Rust crate calls host_net_relay with the handle as a bare JSON
	// number (not an object); mirror that for wire compat.
	body := strconvUint32(handle)
	in := pdk.AllocateString(body)
	out := _hostNetRelay(in.Offset())
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

// NetClose tears down a dialed conn. Best-effort; failures are
// usually because the conn was already torn down by NetRelay's
// terminating side.
func NetClose(handle uint32) error {
	body := strconvUint32(handle)
	in := pdk.AllocateString(body)
	out := _hostNetClose(in.Offset())
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

// ---- host_exec / host_process_* (caps: exec, process) -------------
//
// IMPORTANT TinyGo constraint: the SDK's API for these host fns is
// raw-bytes only.  TinyGo's `encoding/json` package panics
// (interfaceTypeAssert in reflect.Type.Implements) when ANY type in
// the binary contains `map[K]V` and json.{Marshal,Unmarshal} ever
// runs — even on an unrelated value.  The only way to keep
// json.Unmarshal usable for response decoding (Envelope, simple
// struct returns) is to keep maps OUT of the binary's type graph
// entirely.  So the request side accepts pre-encoded JSON bytes;
// the plugin author hand-encodes (use the EncodeJSON helpers below).

//go:wasmimport platypus host_exec
func _hostExec(reqPtr uint64) uint64

type ExecResponse struct {
	ExitCode int32  `json:"exit_code,omitempty"`
	Stdout   string `json:"stdout,omitempty"`
	Stderr   string `json:"stderr,omitempty"`
	Error    string `json:"error,omitempty"`
}

// HostExecRaw invokes the agent's host_exec with the supplied
// pre-marshalled JSON request bytes (shape:
// {"command":"...","args":[...],"env":{...},"cwd":"...","timeout_ms":N}).
// Use EncodeJSON / NewMapBuilder helpers to compose the env map.
//
// The plugin's manifest must declare `capabilities.exec.commands`
// covering the supplied command.
func HostExecRaw(rawJSON []byte) (ExecResponse, error) {
	in := pdk.AllocateBytes(rawJSON)
	out := _hostExec(in.Offset())
	mem := pdk.FindMemory(out)
	env, err := decodeEnvelope(mem.ReadBytes())
	if err != nil {
		return ExecResponse{}, err
	}
	if !env.Ok {
		return ExecResponse{Error: env.Error}, nil
	}
	var resp ExecResponse
	p := jsonParser{buf: env.Data}
	err = parseObject(&p, []fieldHandler{
		{"exit_code", func(p *jsonParser) error {
			v, err := p.readInt64()
			if err == nil {
				resp.ExitCode = int32(v)
			}
			return err
		}},
		{"stdout", func(p *jsonParser) error {
			s, err := p.readString()
			if err == nil {
				resp.Stdout = s
			}
			return err
		}},
		{"stderr", func(p *jsonParser) error {
			s, err := p.readString()
			if err == nil {
				resp.Stderr = s
			}
			return err
		}},
		{"error", func(p *jsonParser) error {
			s, err := p.readString()
			if err == nil {
				resp.Error = s
			}
			return err
		}},
	})
	if err != nil {
		return ExecResponse{}, err
	}
	return resp, nil
}

//go:wasmimport platypus host_process_spawn
func _hostProcessSpawn(specPtr uint64) uint64

//go:wasmimport platypus host_process_relay
func _hostProcessRelay(reqPtr uint64) uint64

//go:wasmimport platypus host_process_kill
func _hostProcessKill(reqPtr uint64) uint64

type ProcessSpawnResult struct {
	Handle uint32 `json:"handle"`
	PID    int32  `json:"pid"`
}

// ProcessSpawnRaw opens a new child process from a pre-marshalled
// JSON spec (shape:
// {"command":"...","args":[...],"cwd":"...","env":{...},"pty":bool,
//  "cols":N,"rows":N}). Returns the host-side handle the relay/kill
// calls reference. Plugin manifest must declare
// `capabilities.process.commands` covering the supplied command.
func ProcessSpawnRaw(rawJSON []byte) (ProcessSpawnResult, error) {
	in := pdk.AllocateBytes(rawJSON)
	out := _hostProcessSpawn(in.Offset())
	mem := pdk.FindMemory(out)
	env, err := decodeEnvelope(mem.ReadBytes())
	if err != nil {
		return ProcessSpawnResult{}, err
	}
	if !env.Ok {
		return ProcessSpawnResult{}, errString(env.Error)
	}
	var r ProcessSpawnResult
	p := jsonParser{buf: env.Data}
	err = parseObject(&p, []fieldHandler{
		{"handle", func(p *jsonParser) error {
			v, err := p.readUint64()
			if err == nil {
				r.Handle = uint32(v)
			}
			return err
		}},
		{"pid", func(p *jsonParser) error {
			v, err := p.readInt64()
			if err == nil {
				r.PID = int32(v)
			}
			return err
		}},
	})
	if err != nil {
		return ProcessSpawnResult{}, err
	}
	return r, nil
}

// ProcessRelay hands off the bidirectional pump to the host. Blocks
// until the child exits OR the wire closes; the host emits the
// terminal ProcessFrame.exit itself before returning.
func ProcessRelay(handle uint32) error {
	body := strconvUint32(handle)
	in := pdk.AllocateString(body)
	out := _hostProcessRelay(in.Offset())
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

// ProcessKill tears the spawned child down. Best-effort cleanup
// path used after a relay error.
func ProcessKill(handle uint32) error {
	body := strconvUint32(handle)
	in := pdk.AllocateString(body)
	out := _hostProcessKill(in.Offset())
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
	p := jsonParser{buf: env.Data}
	err = parseObject(&p, []fieldHandler{
		{"os", func(p *jsonParser) error {
			s, err := p.readString()
			if err == nil {
				u.OS = s
			}
			return err
		}},
		{"arch", func(p *jsonParser) error {
			s, err := p.readString()
			if err == nil {
				u.Arch = s
			}
			return err
		}},
	})
	if err != nil {
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
	p := jsonParser{buf: env.Data}
	contents, err := p.readString()
	if err != nil {
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
	p := jsonParser{buf: env.Data}
	p.skipWhitespace()
	if !p.consume('[') {
		return nil, parseErr("expect '['", p.pos)
	}
	var entries []FSListEntry
	for {
		p.skipWhitespace()
		if p.consume(']') {
			return entries, nil
		}
		var e FSListEntry
		err := parseObject(&p, []fieldHandler{
			{"name", func(p *jsonParser) error {
				s, err := p.readString()
				e.Name = s
				return err
			}},
			{"is_dir", func(p *jsonParser) error {
				b, err := p.readBool()
				e.IsDir = b
				return err
			}},
			{"size", func(p *jsonParser) error {
				v, err := p.readInt64()
				e.Size = v
				return err
			}},
			{"mtime_unix", func(p *jsonParser) error {
				v, err := p.readInt64()
				e.MTimeUnix = v
				return err
			}},
			{"mode", func(p *jsonParser) error {
				v, err := p.readUint64()
				e.Mode = uint32(v)
				return err
			}},
		})
		if err != nil {
			return nil, err
		}
		entries = append(entries, e)
		p.skipWhitespace()
		if p.consume(',') {
			continue
		}
		p.skipWhitespace()
		if !p.consume(']') {
			return nil, parseErr("expect ',' or ']'", p.pos)
		}
		return entries, nil
	}
}

// ---- host_fs_write family (cap: fs.write) --------------------------

//go:wasmimport platypus host_fs_mkdir
func _hostFSMkdir(reqPtr uint64) uint64

//go:wasmimport platypus host_fs_chmod
func _hostFSChmod(reqPtr uint64) uint64

//go:wasmimport platypus host_fs_delete
func _hostFSDelete(reqPtr uint64) uint64

//go:wasmimport platypus host_fs_rename
func _hostFSRename(reqPtr uint64) uint64

//go:wasmimport platypus host_fs_write_range
func _hostFSWriteRange(reqPtr uint64) uint64

// HostFSMkdir creates a directory. Mode is the unix permission bits;
// 0 falls back to 0o755 host-side. mkdirs controls parent-creation.
func HostFSMkdir(path string, mode uint32, mkdirs bool) error {
	body := buildFSWriteJSON(path, mode, mkdirs, false)
	in := pdk.AllocateString(body)
	return decodeOkEnvelope(_hostFSMkdir(in.Offset()))
}

// HostFSChmod sets the unix permission bits on path.
func HostFSChmod(path string, mode uint32) error {
	body := buildFSWriteJSON(path, mode, false, false)
	in := pdk.AllocateString(body)
	return decodeOkEnvelope(_hostFSChmod(in.Offset()))
}

// HostFSDelete unlinks path. recursive=true rm -rf's directories.
func HostFSDelete(path string, recursive bool) error {
	body := buildFSWriteJSON(path, 0, false, recursive)
	in := pdk.AllocateString(body)
	return decodeOkEnvelope(_hostFSDelete(in.Offset()))
}

// HostFSRename moves a file or directory.
func HostFSRename(from, to string) error {
	body := `{"from":` + EncodeJSONString(from) + `,"to":` + EncodeJSONString(to) + `}`
	in := pdk.AllocateString(body)
	return decodeOkEnvelope(_hostFSRename(in.Offset()))
}

// HostFSWriteRange writes a chunk of bytes at a specific offset.
// Used by streaming-style file-write plugins. truncate=true on the
// first call truncates the destination; subsequent calls extend.
// mkdirs creates parent directories if missing on first call.
func HostFSWriteRange(path string, offset int64, data []byte, mode uint32, mkdirs, truncate bool) error {
	var b strings.Builder
	b.WriteByte('{')
	b.WriteString(`"path":`)
	b.WriteString(EncodeJSONString(path))
	b.WriteString(`,"offset":`)
	b.WriteString(strconv.FormatInt(offset, 10))
	b.WriteString(`,"data":`)
	b.WriteString(EncodeJSONString(encodeBase64(data)))
	if mode != 0 {
		b.WriteString(`,"mode":`)
		b.WriteString(strconvUint32(mode))
	}
	if mkdirs {
		b.WriteString(`,"mkdirs":true`)
	}
	if truncate {
		b.WriteString(`,"truncate":true`)
	}
	b.WriteByte('}')
	in := pdk.AllocateString(b.String())
	return decodeOkEnvelope(_hostFSWriteRange(in.Offset()))
}

// buildFSWriteJSON emits the JSON request body shared by mkdir /
// chmod / delete (the host-side fsWriteRequest struct).  Hand-rolled
// for the same TinyGo-no-encoding/json reason as the rest of the
// SDK.
func buildFSWriteJSON(path string, mode uint32, mkdirs, recursive bool) string {
	var b strings.Builder
	b.WriteByte('{')
	b.WriteString(`"path":`)
	b.WriteString(EncodeJSONString(path))
	if mode != 0 {
		b.WriteString(`,"mode":`)
		b.WriteString(strconvUint32(mode))
	}
	if mkdirs {
		b.WriteString(`,"mkdirs":true`)
	}
	if recursive {
		b.WriteString(`,"recursive":true`)
	}
	b.WriteByte('}')
	return b.String()
}

// decodeOkEnvelope decodes the standard {ok,error} envelope at a
// memory offset. Used by every fs.write fn since they share the
// same ack-only response shape.
func decodeOkEnvelope(out uint64) error {
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
	p := jsonParser{buf: env.Data}
	v, err := p.readString()
	if err != nil {
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
