package link

import (
	"bytes"
	"errors"
	"io"
	"testing"

	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// WriteFrame / ReadFrame are the smallest wire primitive inside a
// yamux stream: one protobuf message framed with a 4-byte big-endian
// length header. Identical framing idea to internal/protocol, but
// generic over proto.Message so every v2 stream header, request,
// response, and event message can reuse it.

func TestWriteReadFrame_Roundtrip(t *testing.T) {
	in := &v2pb.StreamHeader{
		Type:          v2pb.StreamType_STREAM_TYPE_PROCESS_OPEN,
		Metadata:      []byte("stub metadata"),
		CorrelationId: "corr-1",
	}

	var buf bytes.Buffer
	if err := WriteFrame(&buf, in); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}

	var out v2pb.StreamHeader
	if err := ReadFrame(&buf, &out); err != nil {
		t.Fatalf("ReadFrame: %v", err)
	}
	if out.Type != in.Type {
		t.Fatalf("Type round-trip: got %v; want %v", out.Type, in.Type)
	}
	if !bytes.Equal(out.Metadata, in.Metadata) {
		t.Fatalf("Metadata round-trip: got %q; want %q", out.Metadata, in.Metadata)
	}
	if out.CorrelationId != in.CorrelationId {
		t.Fatalf("CorrelationId round-trip: got %q; want %q", out.CorrelationId, in.CorrelationId)
	}
}

func TestReadFrame_EOFOnEmptyStream(t *testing.T) {
	var buf bytes.Buffer
	var h v2pb.StreamHeader
	if err := ReadFrame(&buf, &h); !errors.Is(err, io.EOF) {
		t.Fatalf("ReadFrame on empty stream: got %v; want io.EOF", err)
	}
}

func TestReadFrame_UnexpectedEOFAfterHeader(t *testing.T) {
	// Write a 4-byte header that claims a 100-byte body, but supply
	// no body. ReadFrame must signal io.ErrUnexpectedEOF so the
	// caller can close the stream rather than returning partial data.
	buf := bytes.NewBuffer([]byte{0x00, 0x00, 0x00, 0x64}) // 100 bytes
	var h v2pb.StreamHeader
	if err := ReadFrame(buf, &h); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("truncated frame: got %v; want io.ErrUnexpectedEOF", err)
	}
}

func TestReadFrame_EnforcesMaxSize(t *testing.T) {
	// Header claims a body bigger than FrameMaxBytes. Even though the
	// underlying reader could in theory supply those bytes, we refuse
	// on the header alone — attacker-controlled large frames must not
	// get to io.ReadFull'd into RAM.
	oversized := FrameMaxBytes + 1
	hdr := []byte{
		byte(oversized >> 24), byte(oversized >> 16),
		byte(oversized >> 8), byte(oversized),
	}
	buf := bytes.NewBuffer(hdr)
	var h v2pb.StreamHeader
	err := ReadFrame(buf, &h)
	if err == nil {
		t.Fatal("ReadFrame accepted oversized frame")
	}
	if !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("err = %v; want ErrFrameTooLarge", err)
	}
}

func TestWriteFrame_RejectsOversizedMessage(t *testing.T) {
	// Stuff a metadata blob big enough to push the marshalled size
	// over FrameMaxBytes.
	huge := make([]byte, FrameMaxBytes+16)
	h := &v2pb.StreamHeader{
		Type:     v2pb.StreamType_STREAM_TYPE_RPC,
		Metadata: huge,
	}
	var buf bytes.Buffer
	if err := WriteFrame(&buf, h); !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("err = %v; want ErrFrameTooLarge", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("WriteFrame emitted %d bytes despite error", buf.Len())
	}
}
