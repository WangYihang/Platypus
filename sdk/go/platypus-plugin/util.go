package platypus

import (
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
)

// errString returns a plain Go error wrapping the host's error
// message.
func errString(s string) error {
	if s == "" {
		return errors.New("platypus: host returned ok=false with empty error")
	}
	return errors.New(s)
}

// encodeBase64 / decodeBase64 mirror the encoding the host fns use
// to ferry binary blobs through JSON envelopes (see
// internal/agent/plugin/host_kv.go,host_fs.go).
func encodeBase64(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

// strconvUint32 formats a uint32 as a base-10 ASCII number.
// Wrapper around strconv.FormatUint; exists so callers in host.go
// don't have to import strconv (keeps the host-fn file's import
// set small).
func strconvUint32(v uint32) string {
	return strconv.FormatUint(uint64(v), 10)
}

// ---- TinyGo-safe JSON encoding helpers -----------------------------
//
// Background: TinyGo's `encoding/json` package panics on Marshal AND
// Unmarshal once any reachable type in the wasm binary contains a
// `map[K]V`. The panic is in TinyGo's reflect package (incomplete
// `Type.Implements` support); see TinyGo issue tracker for the
// long-standing gap.  The SDK avoids the trap by:
//
//   - Keeping no map types in the SDK's exposed structs.
//   - Providing the helpers below for plugin authors to compose
//     JSON object/array bodies by hand without touching reflect.
//
// json.Unmarshal of map-free shapes (Envelope, ExecResponse,
// ProcessSpawnResult, NetDialResult, FSListEntry, …) works fine
// because none of the reachable types are maps.

// EncodeJSONStringArray emits `["a","b",…]` for a []string.
// Hand-rolled to dodge TinyGo's reflect-based json.Marshal.
func EncodeJSONStringArray(xs []string) string {
	var b strings.Builder
	writeJSONStringArray(&b, xs)
	return b.String()
}

// EncodeJSONString quotes s as a JSON string literal.
func EncodeJSONString(s string) string {
	var b strings.Builder
	encodeJSONString(&b, s)
	return b.String()
}

// MapBuilder accumulates a JSON object body — `{"k1":"v1",…}` — by
// repeated Add(key, value) calls. Plugin authors compose env maps
// (and similar string→string maps) without ever introducing a Go
// `map[K]V` value into the binary; TinyGo's reflect is incomplete
// for map types and any reachable map[K]V trips json.{Marshal,
// Unmarshal} (see SDK doc comment for full background).
type MapBuilder struct {
	b     strings.Builder
	first bool
}

// NewMapBuilder returns an empty builder ready for Add calls.
func NewMapBuilder() *MapBuilder {
	mb := &MapBuilder{first: true}
	mb.b.WriteByte('{')
	return mb
}

// Add appends one (key,value) pair.
func (m *MapBuilder) Add(key, value string) {
	if !m.first {
		m.b.WriteByte(',')
	}
	m.first = false
	encodeJSONString(&m.b, key)
	m.b.WriteByte(':')
	encodeJSONString(&m.b, value)
}

// Done finalises and returns the JSON object string.  Calling Add
// after Done is undefined behaviour — typical use is build-once.
func (m *MapBuilder) Done() string {
	m.b.WriteByte('}')
	return m.b.String()
}

func writeJSONStringArray(b *strings.Builder, xs []string) {
	b.WriteByte('[')
	for i, x := range xs {
		if i > 0 {
			b.WriteByte(',')
		}
		encodeJSONString(b, x)
	}
	b.WriteByte(']')
}

// encodeJSONString quotes s as a JSON string. Handles the standard
// escapes (`"` `\` `\b` `\f` `\n` `\r` `\t`) and emits any control
// byte < 0x20 as \u00XX. Multi-byte UTF-8 runes pass through
// verbatim — encoding/json upstream does the same.
func encodeJSONString(b *strings.Builder, s string) {
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if c < 0x20 {
				const hex = "0123456789abcdef"
				b.WriteString(`\u00`)
				b.WriteByte(hex[c>>4])
				b.WriteByte(hex[c&0xf])
			} else {
				b.WriteByte(c)
			}
		}
	}
	b.WriteByte('"')
}
func decodeBase64(s string) ([]byte, error) {
	if s == "" {
		return nil, nil
	}
	return base64.StdEncoding.DecodeString(s)
}

// sprintf is a tiny fmt.Sprintf-shaped helper that supports the
// minimum set of verbs LogInfof / LogWarnf etc. need (`%s`, `%v`,
// `%d`). Hand-rolled because tinygo's fmt package adds noticeable
// binary size for non-trivial format strings; for simple log lines
// this is enough and keeps the plugin under 200 KB.
//
// Falls back to format-as-is when a verb isn't recognised — better
// to log a slightly imprecise line than to panic on a malformed
// format string at runtime.
func sprintf(format string, args ...any) string {
	out := make([]byte, 0, len(format)+16)
	argIdx := 0
	for i := 0; i < len(format); i++ {
		if format[i] != '%' || i+1 >= len(format) {
			out = append(out, format[i])
			continue
		}
		i++
		switch format[i] {
		case '%':
			out = append(out, '%')
		case 's', 'v':
			if argIdx >= len(args) {
				out = append(out, '%', format[i])
				continue
			}
			out = append(out, anyToString(args[argIdx])...)
			argIdx++
		case 'd':
			if argIdx >= len(args) {
				out = append(out, '%', format[i])
				continue
			}
			out = append(out, intToString(args[argIdx])...)
			argIdx++
		default:
			out = append(out, '%', format[i])
		}
	}
	return string(out)
}

func anyToString(a any) string {
	switch v := a.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	case error:
		if v == nil {
			return "<nil>"
		}
		return v.Error()
	case int:
		return strconv.Itoa(v)
	case int32:
		return strconv.FormatInt(int64(v), 10)
	case int64:
		return strconv.FormatInt(v, 10)
	case uint:
		return strconv.FormatUint(uint64(v), 10)
	case uint32:
		return strconv.FormatUint(uint64(v), 10)
	case uint64:
		return strconv.FormatUint(v, 10)
	case bool:
		if v {
			return "true"
		}
		return "false"
	case nil:
		return "<nil>"
	}
	return "?"
}

func intToString(a any) string {
	switch v := a.(type) {
	case int:
		return strconv.Itoa(v)
	case int32:
		return strconv.FormatInt(int64(v), 10)
	case int64:
		return strconv.FormatInt(v, 10)
	case uint:
		return strconv.FormatUint(uint64(v), 10)
	case uint32:
		return strconv.FormatUint(uint64(v), 10)
	case uint64:
		return strconv.FormatUint(v, 10)
	}
	return "?"
}
