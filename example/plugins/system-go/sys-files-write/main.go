// sys-files-write-go is the TinyGo port of
// example/plugins/system/sys-files-write. Six entry points share the
// same plugin id (one wasm module):
//
//   mkdir + chmod + delete + rename   ← cap fs.write (RPC each)
//   write (stream pump)               ← cap fs.write
//
// Wire formats stay byte-for-byte identical to the Rust crate's
// output (FileWriteResponse + FileWriteResult for the stream;
// ErrorOnlyResponse JSON for the RPCs).  Different plugin id
// (-go suffix) so both Rust and Go versions ship side-by-side.
//
// Build: tinygo build -target wasi -o sys_files_write.wasm .
package main

import (
	"encoding/binary"
	"errors"
	"strings"

	"github.com/extism/go-pdk"

	platypus "github.com/WangYihang/Platypus/sdk/go/platypus-plugin"
)

const (
	wireVarint = 0
	wireLen    = 2
)

// ============================================================
// mkdir + chmod + delete + rename (RPCs)
// ============================================================

// emitErrorOnly emits the JSON envelope every RPC entry returns —
// either {"error":"…"} on failure or {} on success.
func emitErrorOnly(err error) int32 {
	if err != nil {
		pdk.OutputString(`{"error":` + platypus.EncodeJSONString(err.Error()) + `}`)
	} else {
		pdk.OutputString(`{}`)
	}
	return 0
}

//export mkdir
func mkdir() int32 {
	req := parseStringMode(pdk.Input(), false /*recursiveField*/)
	return emitErrorOnly(platypus.HostFSMkdir(req.path, req.mode, req.flag))
}

//export chmod
func chmod() int32 {
	req := parseStringMode(pdk.Input(), false)
	return emitErrorOnly(platypus.HostFSChmod(req.path, req.mode))
}

//export delete
func deleteEntry() int32 {
	req := parseStringMode(pdk.Input(), true /*recursiveField interpreted as recursive flag*/)
	return emitErrorOnly(platypus.HostFSDelete(req.path, req.flag))
}

//export rename
func rename() int32 {
	from, to := parseRename(pdk.Input())
	return emitErrorOnly(platypus.HostFSRename(from, to))
}

// ============================================================
// write (stream)
// ============================================================
//
// Receives a FileWriteRequest as input, opens the destination via
// HostFSWriteRange (truncate first call), reads incoming FileChunk
// frames from the wire via HostStreamRead, writes each chunk's data
// through subsequent HostFSWriteRange calls at running offsets,
// emits FileWriteResponse + FileWriteResult frames matching the
// legacy wire contract.

//export write
func writeStream() int32 {
	req := parseFileWriteRequest(pdk.Input())
	if req.Path == "" {
		_ = writeResponseFrame("empty path")
		return 0
	}

	// Open / truncate / mkdirs check via a zero-byte first write.
	if err := platypus.HostFSWriteRange(req.Path, 0, nil, req.Mode, req.Mkdirs, !req.Append); err != nil {
		_ = writeResponseFrame(err.Error())
		return 0
	}

	if err := writeResponseFrame(""); err != nil {
		platypus.LogErrorf("sys-files-write-go: write response: %s", err.Error())
		return 1
	}

	var bytesWritten int64
	var firstError string
	for {
		body, err := platypus.HostStreamRead()
		if err != nil {
			firstError = "read frame: " + err.Error()
			break
		}
		if len(body) == 0 {
			firstError = "stream closed before eof chunk"
			break
		}
		chunk := parseFileChunk(body)
		if chunk.Error != "" && firstError == "" {
			firstError = chunk.Error
		}
		if len(chunk.Data) > 0 {
			if err := platypus.HostFSWriteRange(req.Path, bytesWritten, chunk.Data, req.Mode, false, false); err != nil {
				if firstError == "" {
					firstError = "write @ " + strFromInt64(bytesWritten) + ": " + err.Error()
				}
				break
			}
			bytesWritten += int64(len(chunk.Data))
		}
		if chunk.EOF {
			break
		}
	}

	_ = writeResultFrame(bytesWritten, firstError)
	return 0
}

func main() {}

// ---- proto encoders ---------------------------------------------

func writeResponseFrame(errMsg string) error {
	// FileWriteResponse{error=1:string}
	buf := make([]byte, 0, len(errMsg)+8)
	if errMsg != "" {
		buf = appendTag(buf, 1, wireLen)
		buf = binary.AppendUvarint(buf, uint64(len(errMsg)))
		buf = append(buf, errMsg...)
	}
	return platypus.HostStreamWrite(buf)
}

func writeResultFrame(bytesWritten int64, errMsg string) error {
	// FileWriteResult{bytes_written=1:int64, error=2:string}
	buf := make([]byte, 0, len(errMsg)+16)
	if bytesWritten != 0 {
		buf = appendTag(buf, 1, wireVarint)
		buf = binary.AppendUvarint(buf, uint64(bytesWritten))
	}
	if errMsg != "" {
		buf = appendTag(buf, 2, wireLen)
		buf = binary.AppendUvarint(buf, uint64(len(errMsg)))
		buf = append(buf, errMsg...)
	}
	return platypus.HostStreamWrite(buf)
}

// ---- request decoders -------------------------------------------

// stringModeReq is the parsed shape used by mkdir/chmod/delete: a
// path field, a numeric field (mode for mkdir/chmod, ignored for
// delete), and one bool flag (mkdirs for mkdir, recursive for
// delete, ignored for chmod). The bridge sends already-marshalled
// JSON of MkdirRequest/ChmodRequest/DeleteRequest; we hand-decode
// the small subset.
type stringModeReq struct {
	path string
	mode uint32
	flag bool // mkdirs (mkdir) / recursive (delete)
}

// parseStringMode handles the {path,mode,mkdirs|recursive} JSON
// shape that mkdir/chmod/delete share. recursiveField=true tells
// the parser to map a "recursive" bool into req.flag (for delete);
// false maps "mkdirs" into req.flag (for mkdir/chmod).
func parseStringMode(buf []byte, recursiveField bool) stringModeReq {
	var req stringModeReq
	scanJSONFields(buf, func(key string, raw []byte) {
		switch key {
		case "path":
			req.path = decodeJSONString(raw)
		case "mode":
			req.mode = uint32(decodeJSONInt(raw))
		case "mkdirs":
			if !recursiveField {
				req.flag = decodeJSONBool(raw)
			}
		case "recursive":
			if recursiveField {
				req.flag = decodeJSONBool(raw)
			}
		}
	})
	return req
}

func parseRename(buf []byte) (from, to string) {
	scanJSONFields(buf, func(key string, raw []byte) {
		switch key {
		case "from":
			from = decodeJSONString(raw)
		case "to":
			to = decodeJSONString(raw)
		}
	})
	return from, to
}

// fileWriteRequest is the proto-encoded request the bridge sends as
// stream metadata.  Hand-decoded with the same wire idiom the rest
// of the streaming plugins use.
type fileWriteRequest struct {
	Path   string
	Append bool
	Mode   uint32
	Mkdirs bool
}

func parseFileWriteRequest(buf []byte) fileWriteRequest {
	var req fileWriteRequest
	for len(buf) > 0 {
		tag, n := binary.Uvarint(buf)
		if n <= 0 {
			return req
		}
		buf = buf[n:]
		field := uint32(tag >> 3)
		wire := uint32(tag & 0x7)
		switch {
		case field == 1 && wire == wireLen:
			s, rest, ok := readLenString(buf)
			if !ok {
				return req
			}
			req.Path = s
			buf = rest
		case field == 2 && wire == wireVarint:
			v, m := binary.Uvarint(buf)
			if m <= 0 {
				return req
			}
			req.Append = v != 0
			buf = buf[m:]
		case field == 3 && wire == wireVarint:
			v, m := binary.Uvarint(buf)
			if m <= 0 {
				return req
			}
			req.Mode = uint32(v)
			buf = buf[m:]
		case field == 4 && wire == wireVarint:
			v, m := binary.Uvarint(buf)
			if m <= 0 {
				return req
			}
			req.Mkdirs = v != 0
			buf = buf[m:]
		default:
			b, err := skipField(buf, wire)
			if err != nil {
				return req
			}
			buf = b
		}
	}
	return req
}

// fileChunk is the proto shape the upstream FILE_WRITE wire emits
// per chunk. Fields: 1 bytes data, 2 bool eof, 3 string error.
type fileChunk struct {
	Data  []byte
	EOF   bool
	Error string
}

func parseFileChunk(buf []byte) fileChunk {
	var c fileChunk
	for len(buf) > 0 {
		tag, n := binary.Uvarint(buf)
		if n <= 0 {
			return c
		}
		buf = buf[n:]
		field := uint32(tag >> 3)
		wire := uint32(tag & 0x7)
		switch {
		case field == 1 && wire == wireLen:
			body, rest, ok := readLenBytes(buf)
			if !ok {
				return c
			}
			c.Data = append([]byte(nil), body...)
			buf = rest
		case field == 2 && wire == wireVarint:
			v, m := binary.Uvarint(buf)
			if m <= 0 {
				return c
			}
			c.EOF = v != 0
			buf = buf[m:]
		case field == 3 && wire == wireLen:
			s, rest, ok := readLenString(buf)
			if !ok {
				return c
			}
			c.Error = s
			buf = rest
		default:
			b, err := skipField(buf, wire)
			if err != nil {
				return c
			}
			buf = b
		}
	}
	return c
}

// ---- shared proto helpers ---------------------------------------

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

// ---- minimal JSON field scanner ---------------------------------
//
// scanJSONFields walks one level of a JSON object {…}, calling cb
// with each (key, raw-value-bytes) pair. Sufficient for the small
// flat objects the bridge sends to mkdir/chmod/delete/rename. Used
// in place of encoding/json.Unmarshal to dodge TinyGo's reflect
// limitations (see SDK doc comment).

func scanJSONFields(buf []byte, cb func(key string, raw []byte)) {
	i := skipWhitespace(buf, 0)
	if i >= len(buf) || buf[i] != '{' {
		return
	}
	i++
	for {
		i = skipWhitespace(buf, i)
		if i >= len(buf) || buf[i] == '}' {
			return
		}
		if buf[i] != '"' {
			return
		}
		key, ni, ok := scanString(buf, i)
		if !ok {
			return
		}
		i = skipWhitespace(buf, ni)
		if i >= len(buf) || buf[i] != ':' {
			return
		}
		i = skipWhitespace(buf, i+1)
		start := i
		ni, ok = scanValue(buf, i)
		if !ok {
			return
		}
		raw := buf[start:ni]
		cb(key, raw)
		i = skipWhitespace(buf, ni)
		if i >= len(buf) {
			return
		}
		if buf[i] == ',' {
			i++
			continue
		}
		return
	}
}

func skipWhitespace(buf []byte, i int) int {
	for i < len(buf) {
		switch buf[i] {
		case ' ', '\t', '\n', '\r':
			i++
		default:
			return i
		}
	}
	return i
}

// scanString reads a JSON string starting at i and returns its
// decoded value + the position after the closing quote.
func scanString(buf []byte, i int) (string, int, bool) {
	if i >= len(buf) || buf[i] != '"' {
		return "", i, false
	}
	i++
	var b strings.Builder
	for i < len(buf) {
		c := buf[i]
		switch c {
		case '"':
			return b.String(), i + 1, true
		case '\\':
			i++
			if i >= len(buf) {
				return "", i, false
			}
			esc := buf[i]
			i++
			switch esc {
			case '"', '\\', '/':
				b.WriteByte(esc)
			case 'b':
				b.WriteByte('\b')
			case 'f':
				b.WriteByte('\f')
			case 'n':
				b.WriteByte('\n')
			case 'r':
				b.WriteByte('\r')
			case 't':
				b.WriteByte('\t')
			default:
				return "", i, false // \u escapes not needed for our inputs
			}
		default:
			b.WriteByte(c)
			i++
		}
	}
	return "", i, false
}

// scanValue advances past one complete JSON value starting at i.
// Doesn't decode — just returns the position after the value so the
// caller can slice raw bytes.
func scanValue(buf []byte, i int) (int, bool) {
	if i >= len(buf) {
		return i, false
	}
	c := buf[i]
	switch {
	case c == '"':
		_, ni, ok := scanString(buf, i)
		return ni, ok
	case c == '{':
		return scanBraced(buf, i, '{', '}')
	case c == '[':
		return scanBraced(buf, i, '[', ']')
	case c == 't' || c == 'f' || c == 'n':
		// true / false / null
		for i < len(buf) && ((buf[i] >= 'a' && buf[i] <= 'z')) {
			i++
		}
		return i, true
	default:
		// number
		for i < len(buf) {
			b := buf[i]
			if b == '-' || b == '+' || b == '.' || b == 'e' || b == 'E' || (b >= '0' && b <= '9') {
				i++
				continue
			}
			break
		}
		return i, true
	}
}

func scanBraced(buf []byte, i int, open, close byte) (int, bool) {
	depth := 0
	for i < len(buf) {
		c := buf[i]
		switch c {
		case open:
			depth++
			i++
		case close:
			depth--
			i++
			if depth == 0 {
				return i, true
			}
		case '"':
			_, ni, ok := scanString(buf, i)
			if !ok {
				return ni, false
			}
			i = ni
		default:
			i++
		}
	}
	return i, false
}

func decodeJSONString(raw []byte) string {
	if len(raw) == 0 || raw[0] != '"' {
		return ""
	}
	s, _, ok := scanString(raw, 0)
	if !ok {
		return ""
	}
	return s
}

func decodeJSONBool(raw []byte) bool {
	return string(raw) == "true"
}

// decodeJSONInt parses a positive JSON integer literal. Used for
// the `mode` field in mkdir/chmod requests; doesn't handle floats /
// negatives because the wire never produces those.
func decodeJSONInt(raw []byte) uint64 {
	var v uint64
	for _, b := range raw {
		if b < '0' || b > '9' {
			break
		}
		v = v*10 + uint64(b-'0')
	}
	return v
}

func strFromInt64(v int64) string {
	// strconv import would re-pull strconv, but we already have it
	// transitively via encoding/binary. Inline conversion.
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
