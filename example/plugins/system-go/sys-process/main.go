// sys-process-go is the TinyGo port of example/plugins/system/sys-process.
// Two entry points share the same plugin id (one wasm module, two
// exports):
//
//   exec  (RPC)              ← capability `exec`
//   open  (stream pump)      ← capability `process`
//
// `exec` is the synchronous one-shot path: ExecRequest in,
// ExecResponse out. The Go plugin forwards the bridge's input bytes
// verbatim to host_exec — no decode/re-encode dance, which would
// trip TinyGo's reflect/json gap on the env map field anyway.
//
// `open` parses the proto-encoded ProcessOpenRequest, builds a JSON
// spawn spec by hand (TinyGo-safe — no reflect), and hands off to
// host_process_spawn + host_process_relay; host owns the
// bidirectional pump until the child exits.
//
// Build: tinygo build -target wasi -o sys_process.wasm .
package main

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/extism/go-pdk"

	platypus "github.com/WangYihang/Platypus/sdk/go/platypus-plugin"
)

// Wire types from proto.
const (
	wireVarint = 0
	wireLen    = 2
)

// ============================================================
// exec (RPC)
// ============================================================

//export exec
func execEntry() int32 {
	// Input is the bridge's already-marshalled JSON ExecRequest.
	// Forward verbatim; host_exec parses + executes + returns the
	// JSON-shaped envelope, which HostExecRaw decodes.
	resp, err := platypus.HostExecRaw(pdk.Input())
	if err != nil {
		emitJSON(platypus.ExecResponse{Error: err.Error()})
		return 0
	}
	emitJSON(resp)
	return 0
}

func emitJSON(v platypus.ExecResponse) {
	body, err := json.Marshal(v)
	if err != nil {
		platypus.LogErrorf("sys-process-go: marshal: %s", err.Error())
		return
	}
	pdk.OutputString(string(body))
}

// ============================================================
// open (stream)
// ============================================================
//
// Wasm stays thin: parse the proto-encoded ProcessOpenRequest, spawn
// via the host, ack with a length-prefixed ProcessOpenResponse,
// then ProcessRelay (host writes the terminal exit frame on its own).

//export open
func open() int32 {
	req := parseProcessOpenRequest(pdk.Input())
	if req.Command == "" {
		_ = writeOpenResponse(0, "empty_command")
		return 0
	}

	specJSON := buildSpawnSpecJSON(req)
	spawn, err := platypus.ProcessSpawnRaw([]byte(specJSON))
	if err != nil {
		_ = writeOpenResponse(0, err.Error())
		return 0
	}

	if err := writeOpenResponse(int64(spawn.PID), ""); err != nil {
		platypus.LogErrorf("sys-process-go: write open ack: %s", err.Error())
		return 1
	}

	if err := platypus.ProcessRelay(spawn.Handle); err != nil {
		_ = platypus.ProcessKill(spawn.Handle)
		platypus.LogErrorf("sys-process-go: relay: %s", err.Error())
		return 1
	}
	return 0
}

func main() {}

// buildSpawnSpecJSON emits the JSON object host_process_spawn
// expects.  Hand-rolled to avoid TinyGo's reflect/json gap: the
// plugin must contain ZERO `map[K]V` types in its compilation unit
// or json.Unmarshal panics on first use (interfaceTypeAssert in
// reflect.Type.Implements).  envJSON is already an `{"k":"v",...}`
// object string built incrementally during proto parsing.
func buildSpawnSpecJSON(req processOpenRequest) string {
	var b strings.Builder
	b.WriteByte('{')
	b.WriteString(`"command":`)
	b.WriteString(platypus.EncodeJSONString(req.Command))
	b.WriteString(`,"args":`)
	b.WriteString(platypus.EncodeJSONStringArray(req.Args))
	b.WriteString(`,"cwd":`)
	b.WriteString(platypus.EncodeJSONString(req.Cwd))
	b.WriteString(`,"env":`)
	if req.EnvJSON == "" {
		b.WriteString("{}")
	} else {
		b.WriteString(req.EnvJSON)
	}
	b.WriteString(`,"pty":`)
	if req.PTY {
		b.WriteString("true")
	} else {
		b.WriteString("false")
	}
	b.WriteString(`,"cols":`)
	b.WriteString(strconv.FormatUint(uint64(req.Cols), 10))
	b.WriteString(`,"rows":`)
	b.WriteString(strconv.FormatUint(uint64(req.Rows), 10))
	b.WriteByte('}')
	return b.String()
}

// ---- ProcessOpenResponse encoder --------------------------------

func writeOpenResponse(pid int64, errMsg string) error {
	// ProcessOpenResponse{pid=1:int64, error=2:string}
	buf := make([]byte, 0, 32)
	if pid != 0 {
		buf = appendTag(buf, 1, wireVarint)
		buf = binary.AppendUvarint(buf, uint64(pid))
	}
	if errMsg != "" {
		buf = appendTag(buf, 2, wireLen)
		buf = binary.AppendUvarint(buf, uint64(len(errMsg)))
		buf = append(buf, errMsg...)
	}
	return platypus.HostStreamWrite(buf)
}

// ---- ProcessOpenRequest decoder ---------------------------------

type processOpenRequest struct {
	Command string
	Args    []string
	Cwd     string
	// EnvJSON is a pre-marshalled JSON object string accumulated as
	// proto field-4 entries are decoded.  Stays a plain string so
	// the plugin's reachable type set has no `map[K]V` (which would
	// trip TinyGo's reflect/json panic — see SDK util.go).
	EnvJSON string
	Cols    uint32
	Rows    uint32
	PTY     bool
}

func parseProcessOpenRequest(buf []byte) processOpenRequest {
	req := processOpenRequest{}
	env := platypus.NewMapBuilder()
	for len(buf) > 0 {
		tag, n := binary.Uvarint(buf)
		if n <= 0 {
			req.EnvJSON = env.Done()
			return req
		}
		buf = buf[n:]
		field := uint32(tag >> 3)
		wire := uint32(tag & 0x7)
		switch {
		case field == 1 && wire == wireLen:
			s, rest, ok := readLenString(buf)
			if !ok {
				req.EnvJSON = env.Done()
				return req
			}
			req.Command = s
			buf = rest
		case field == 2 && wire == wireLen:
			s, rest, ok := readLenString(buf)
			if !ok {
				req.EnvJSON = env.Done()
				return req
			}
			req.Args = append(req.Args, s)
			buf = rest
		case field == 3 && wire == wireLen:
			s, rest, ok := readLenString(buf)
			if !ok {
				req.EnvJSON = env.Done()
				return req
			}
			req.Cwd = s
			buf = rest
		case field == 4 && wire == wireLen:
			body, rest, ok := readLenBytes(buf)
			if !ok {
				req.EnvJSON = env.Done()
				return req
			}
			k, v := parseMapEntry(body)
			if k != "" {
				env.Add(k, v)
			}
			buf = rest
		case field == 5 && wire == wireVarint:
			v, m := binary.Uvarint(buf)
			if m <= 0 {
				req.EnvJSON = env.Done()
				return req
			}
			req.Cols = uint32(v)
			buf = buf[m:]
		case field == 6 && wire == wireVarint:
			v, m := binary.Uvarint(buf)
			if m <= 0 {
				req.EnvJSON = env.Done()
				return req
			}
			req.Rows = uint32(v)
			buf = buf[m:]
		case field == 7 && wire == wireVarint:
			v, m := binary.Uvarint(buf)
			if m <= 0 {
				req.EnvJSON = env.Done()
				return req
			}
			req.PTY = v != 0
			buf = buf[m:]
		default:
			b, err := skipField(buf, wire)
			if err != nil {
				req.EnvJSON = env.Done()
				return req
			}
			buf = b
		}
	}
	req.EnvJSON = env.Done()
	return req
}

func parseMapEntry(buf []byte) (key, val string) {
	for len(buf) > 0 {
		tag, n := binary.Uvarint(buf)
		if n <= 0 {
			return key, val
		}
		buf = buf[n:]
		field := uint32(tag >> 3)
		wire := uint32(tag & 0x7)
		if wire != wireLen {
			b, err := skipField(buf, wire)
			if err != nil {
				return key, val
			}
			buf = b
			continue
		}
		s, rest, ok := readLenString(buf)
		if !ok {
			return key, val
		}
		switch field {
		case 1:
			key = s
		case 2:
			val = s
		}
		buf = rest
	}
	return key, val
}

// ---- proto wire helpers ------------------------------------------

func readLenString(buf []byte) (string, []byte, bool) {
	body, rest, ok := readLenBytes(buf)
	if !ok {
		return "", buf, false
	}
	return string(body), rest, true
}

func readLenBytes(buf []byte) ([]byte, []byte, bool) {
	ln, n := binary.Uvarint(buf)
	if n <= 0 {
		return nil, buf, false
	}
	buf = buf[n:]
	if uint64(len(buf)) < ln {
		return nil, buf, false
	}
	return buf[:ln], buf[ln:], true
}

func appendTag(buf []byte, field, wire uint32) []byte {
	return binary.AppendUvarint(buf, uint64((field<<3)|wire))
}

func skipField(buf []byte, wire uint32) ([]byte, error) {
	switch wire {
	case wireVarint:
		_, n := binary.Uvarint(buf)
		if n <= 0 {
			return nil, errors.New("truncated varint")
		}
		return buf[n:], nil
	case wireLen:
		ln, n := binary.Uvarint(buf)
		if n <= 0 {
			return nil, errors.New("truncated len")
		}
		buf = buf[n:]
		if uint64(len(buf)) < ln {
			return nil, errors.New("truncated body")
		}
		return buf[ln:], nil
	default:
		return nil, errors.New("unsupported wire type")
	}
}
