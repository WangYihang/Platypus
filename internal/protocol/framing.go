// Package protocol provides the wire format for Agent <-> Server communication.
//
// Wire format: [4 bytes big-endian length] [N bytes protobuf Envelope]
//
// Each message is framed with a 4-byte length prefix so the receiver knows
// exactly how many bytes to read for the protobuf payload. This avoids the
// delimiter/streaming issues of raw protobuf over TCP.
package protocol

import (
	"encoding/binary"
	"fmt"
	"io"
	"sync"
	"sync/atomic"

	"google.golang.org/protobuf/proto"

	agentpb "github.com/WangYihang/Platypus/pkg/proto/agent/v1"
)

const (
	// MaxMessageSize is the maximum allowed message size (16 MB).
	MaxMessageSize = 16 * 1024 * 1024
	// HeaderSize is the length prefix size in bytes.
	HeaderSize = 4
)

// ProtoCodec provides thread-safe reading/writing of length-prefixed
// protobuf Envelope messages over a stream (typically a TLS connection).
//
// The codec also maintains best-effort lifetime byte/message counters
// for observability. Counters start at zero when the codec is created
// and increment monotonically on each successful Send / Recv. Non-mesh
// callers simply ignore them; the mesh Link layer exposes them via
// MeshKeepalive and LinkStats for the Topology visualisation.
type ProtoCodec struct {
	reader  io.Reader
	writer  io.Writer
	readMu  sync.Mutex
	writeMu sync.Mutex

	bytesSent atomic.Uint64
	bytesRecv atomic.Uint64
	msgsSent  atomic.Uint64
	msgsRecv  atomic.Uint64
}

// NewProtoCodec creates a ProtoCodec from a ReadWriter.
func NewProtoCodec(rw io.ReadWriter) *ProtoCodec {
	return &ProtoCodec{reader: rw, writer: rw}
}

// NewProtoCodecFromParts creates a ProtoCodec from separate reader/writer.
func NewProtoCodecFromParts(r io.Reader, w io.Writer) *ProtoCodec {
	return &ProtoCodec{reader: r, writer: w}
}

// Send marshals an Envelope and writes it with a length prefix.
func (c *ProtoCodec) Send(env *agentpb.Envelope) error {
	data, err := proto.Marshal(env)
	if err != nil {
		return fmt.Errorf("marshal envelope: %w", err)
	}
	if len(data) > MaxMessageSize {
		return fmt.Errorf("message too large: %d > %d", len(data), MaxMessageSize)
	}

	frame := make([]byte, HeaderSize+len(data))
	binary.BigEndian.PutUint32(frame[:HeaderSize], uint32(len(data)))
	copy(frame[HeaderSize:], data)

	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_, err = c.writer.Write(frame)
	if err != nil {
		return fmt.Errorf("write frame: %w", err)
	}
	c.bytesSent.Add(uint64(len(frame)))
	c.msgsSent.Add(1)
	return nil
}

// Recv reads a length-prefixed frame and unmarshals it into an Envelope.
func (c *ProtoCodec) Recv() (*agentpb.Envelope, error) {
	c.readMu.Lock()
	defer c.readMu.Unlock()

	// Read length header
	header := make([]byte, HeaderSize)
	if _, err := io.ReadFull(c.reader, header); err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}

	length := binary.BigEndian.Uint32(header)
	if length > MaxMessageSize {
		return nil, fmt.Errorf("message too large: %d > %d", length, MaxMessageSize)
	}

	// Read payload
	payload := make([]byte, length)
	if _, err := io.ReadFull(c.reader, payload); err != nil {
		return nil, fmt.Errorf("read payload: %w", err)
	}

	env := &agentpb.Envelope{}
	if err := proto.Unmarshal(payload, env); err != nil {
		return nil, fmt.Errorf("unmarshal envelope: %w", err)
	}
	c.bytesRecv.Add(uint64(HeaderSize + len(payload)))
	c.msgsRecv.Add(1)
	return env, nil
}

// BytesSent returns the total number of wire bytes (including the
// 4-byte length header) written successfully by Send.
func (c *ProtoCodec) BytesSent() uint64 { return c.bytesSent.Load() }

// BytesRecv returns the total number of wire bytes read successfully
// by Recv.
func (c *ProtoCodec) BytesRecv() uint64 { return c.bytesRecv.Load() }

// MsgsSent returns the total envelopes successfully written.
func (c *ProtoCodec) MsgsSent() uint64 { return c.msgsSent.Load() }

// MsgsRecv returns the total envelopes successfully read.
func (c *ProtoCodec) MsgsRecv() uint64 { return c.msgsRecv.Load() }
