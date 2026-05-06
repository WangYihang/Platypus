// sys-tunnel-pull-go is the TinyGo port of
// example/plugins/system/sys-tunnel-pull. The wasm side stays thin:
// parse TunnelPullRequest, dial via host_net_dial, ack with a
// TunnelPullResponse, hand off to host_net_relay (host owns the
// bidirectional pump). Same flow as the Rust crate; same wire
// shape on both sides.
//
// Build: tinygo build -target wasi -o sys_tunnel_pull.wasm .
package main

import (
	"encoding/binary"
	"errors"

	"github.com/extism/go-pdk"

	platypus "github.com/WangYihang/Platypus/sdk/go/platypus-plugin"
)

// Wire types from proto: VARINT=0, LEN=2.
const (
	wireVarint = 0
	wireLen    = 2
)

// TunnelPullRequest matches v2pb.TunnelPullRequest's proto encoding:
// field 1 (string) = target, field 2 (uint32 varint) = dial_timeout_ms.
// Hand-decoded so the wasm artefact doesn't pull google.golang.org/protobuf
// (~hundreds of KiB on TinyGo, kills the binary-size budget).
type TunnelPullRequest struct {
	Target        string
	DialTimeoutMs uint32
}

//export pull
func pull() int32 {
	req := parseTunnelPullRequest(pdk.Input())
	if req.Target == "" {
		_ = writePullResponse("", "empty target")
		return 0
	}

	dial, err := platypus.NetDial(req.Target, req.DialTimeoutMs)
	if err != nil {
		// Dial failure (allowlist denial, refused, timeout) propagates
		// to the operator via the response header. No bytes flow.
		_ = writePullResponse("", err.Error())
		return 0
	}

	if err := writePullResponse(dial.ResolvedAddr, ""); err != nil {
		platypus.LogErrorf("sys-tunnel-pull-go: write header: %s", err.Error())
		return 1
	}

	// Hand off to the host's bidirectional pump. Blocks until either
	// side closes; on relay error we still attempt host_net_close so
	// the agent's conn table doesn't leak.
	if err := platypus.NetRelay(dial.Handle); err != nil {
		_ = platypus.NetClose(dial.Handle)
		platypus.LogErrorf("sys-tunnel-pull-go: relay: %s", err.Error())
		return 1
	}
	return 0
}

func main() {}

// ---- TunnelPullResponse encoder ---------------------------------

// writePullResponse emits a length-prefixed v2pb.TunnelPullResponse
// frame on the wire via host_stream_write. Field 1 = resolved_addr
// (string, optional), field 2 = error (string, optional). Only
// populated fields are emitted, matching protobuf optional-field
// semantics.
func writePullResponse(resolved, errMsg string) error {
	buf := make([]byte, 0, len(resolved)+len(errMsg)+8)
	if resolved != "" {
		buf = appendTag(buf, 1, wireLen)
		buf = binary.AppendUvarint(buf, uint64(len(resolved)))
		buf = append(buf, resolved...)
	}
	if errMsg != "" {
		buf = appendTag(buf, 2, wireLen)
		buf = binary.AppendUvarint(buf, uint64(len(errMsg)))
		buf = append(buf, errMsg...)
	}
	return platypus.HostStreamWrite(buf)
}

// ---- TunnelPullRequest decoder ----------------------------------

func parseTunnelPullRequest(buf []byte) TunnelPullRequest {
	var req TunnelPullRequest
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
			ln, m := binary.Uvarint(buf)
			if m <= 0 {
				return req
			}
			buf = buf[m:]
			if uint64(len(buf)) < ln {
				return req
			}
			req.Target = string(buf[:ln])
			buf = buf[ln:]
		case field == 2 && wire == wireVarint:
			v, m := binary.Uvarint(buf)
			if m <= 0 {
				return req
			}
			buf = buf[m:]
			req.DialTimeoutMs = uint32(v)
		default:
			// Skip unknown field by wire type.
			b, err := skipField(buf, wire)
			if err != nil {
				return req
			}
			buf = b
		}
	}
	return req
}

// appendTag writes a proto field-and-wire-type tag varint.
func appendTag(buf []byte, field, wire uint32) []byte {
	return binary.AppendUvarint(buf, uint64((field<<3)|wire))
}

// skipField advances past a single field of `wire` type. Returns
// the remainder; error when the wire encoding is truncated /
// unsupported.
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
