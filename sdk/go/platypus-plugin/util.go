package platypus

import (
	"encoding/base64"
	"errors"
	"strconv"
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
