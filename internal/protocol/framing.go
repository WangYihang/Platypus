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
type ProtoCodec struct {
	reader  io.Reader
	writer  io.Writer
	readMu  sync.Mutex
	writeMu sync.Mutex
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
	return env, nil
}
