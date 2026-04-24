// Package link holds the yamux-over-WebSocket transport shared
// between the v2 agent and server: frame encoding for stream
// headers and per-stream requests, the WS dialer on the agent
// side, the accept loop on the server side. Anything concerned
// with how a stream's first messages travel the wire lives here;
// per-stream-type payload handling (PTY, tunnel, file, RPC,
// event) is dispatched by callers after reading the first frame.
package link

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"google.golang.org/protobuf/proto"
)

// FrameMaxBytes caps the size of a single framed protobuf message on
// a yamux stream. Deliberately smaller than the v1 ProtoCodec's
// 16 MiB ceiling: individual v2 frames are per-stream-header or
// per-RPC payloads; anything that needs to carry bulk bytes (file
// contents, tunnel data) should be written directly to the stream
// after the initial framed handshake, not wrapped in another frame.
const FrameMaxBytes = 1 << 20 // 1 MiB

// ErrFrameTooLarge is returned when either the writer's marshalled
// message or the reader's declared header length exceeds FrameMaxBytes.
// Callers close the stream on this error — a mismatch indicates a
// buggy or hostile peer.
var ErrFrameTooLarge = errors.New("v2pb: frame exceeds FrameMaxBytes")

// frameHeaderSize is the length prefix in bytes. Big-endian u32;
// matches the v1 codec style.
const frameHeaderSize = 4

// WriteFrame marshals m and writes it to w with a 4-byte big-endian
// length prefix. Nothing is written on size-limit violation.
func WriteFrame(w io.Writer, m proto.Message) error {
	payload, err := proto.Marshal(m)
	if err != nil {
		return fmt.Errorf("v2pb: WriteFrame marshal: %w", err)
	}
	if len(payload) > FrameMaxBytes {
		return fmt.Errorf("%w: %d > %d", ErrFrameTooLarge, len(payload), FrameMaxBytes)
	}
	header := make([]byte, frameHeaderSize)
	binary.BigEndian.PutUint32(header, uint32(len(payload)))
	if _, err := w.Write(header); err != nil {
		return fmt.Errorf("v2pb: WriteFrame header: %w", err)
	}
	if _, err := w.Write(payload); err != nil {
		return fmt.Errorf("v2pb: WriteFrame payload: %w", err)
	}
	return nil
}

// ReadFrame reads exactly one length-prefixed frame from r and
// unmarshals it into m. Returns io.EOF when r is cleanly at EOF on
// a header boundary, io.ErrUnexpectedEOF when a header was read but
// the payload is short, and ErrFrameTooLarge when the header
// advertises more bytes than FrameMaxBytes. Callers must treat any
// non-EOF error as fatal for the stream.
func ReadFrame(r io.Reader, m proto.Message) error {
	header := make([]byte, frameHeaderSize)
	if _, err := io.ReadFull(r, header); err != nil {
		// io.ReadFull turns EOF-after-zero-bytes into io.EOF and
		// EOF-after-partial-bytes into io.ErrUnexpectedEOF; we
		// bubble both unchanged so callers can distinguish "stream
		// cleanly closed" from "stream died mid-frame".
		return err
	}
	length := binary.BigEndian.Uint32(header)
	if int(length) > FrameMaxBytes {
		return fmt.Errorf("%w: %d > %d", ErrFrameTooLarge, length, FrameMaxBytes)
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(r, payload); err != nil {
		// A clean EOF after we've already consumed the 4-byte header
		// is not a graceful close — the peer promised length bytes
		// and delivered zero. Surface it as ErrUnexpectedEOF so
		// callers don't conflate it with "stream closed at frame
		// boundary".
		if errors.Is(err, io.EOF) {
			return io.ErrUnexpectedEOF
		}
		return err
	}
	if err := proto.Unmarshal(payload, m); err != nil {
		return fmt.Errorf("v2pb: ReadFrame unmarshal: %w", err)
	}
	return nil
}
