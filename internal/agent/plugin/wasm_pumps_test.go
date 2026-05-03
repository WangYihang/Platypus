package plugin

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/WangYihang/Platypus/internal/link"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// TDD for the wire-frame pumps that bridge a wasm-streaming plugin
// to the wire. Two pumps per stream:
//
//   pumpInboundFrames   reads PluginStreamFrame from the wire and
//                       feeds DATA frames' bytes onto s.inbound;
//                       closes s.inbound on KIND_EOF or wire close.
//   pumpOutboundFrames  drains s.outbound and writes one
//                       OUTBOUND DATA frame per chunk; emits
//                       OUTBOUND EOF when the channel closes.
//
// These run as a goroutine pair around runActiveStream's wasm
// invocation. The dispatcher (next test) plumbs them.

func TestPumpInboundFrames_DataFrameDeliveredToChannel(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	s := &streamCtx{
		inbound:  make(chan []byte, 4),
		outbound: make(chan []byte, 4),
	}
	go pumpInboundFrames(context.Background(), a, s)

	mustWriteFrame(t, b, &v2pb.PluginStreamFrame{
		Source: v2pb.PluginStreamFrame_SOURCE_INBOUND,
		Kind:   v2pb.PluginStreamFrame_KIND_DATA,
		Data:   []byte("hello"),
	})

	select {
	case got := <-s.inbound:
		if string(got) != "hello" {
			t.Errorf("inbound = %q, want hello", got)
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for inbound frame")
	}
}

func TestPumpInboundFrames_EOFFrameClosesChannel(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	s := &streamCtx{
		inbound:  make(chan []byte, 4),
		outbound: make(chan []byte, 4),
	}
	go pumpInboundFrames(context.Background(), a, s)

	mustWriteFrame(t, b, &v2pb.PluginStreamFrame{
		Source: v2pb.PluginStreamFrame_SOURCE_INBOUND,
		Kind:   v2pb.PluginStreamFrame_KIND_EOF,
	})

	select {
	case _, ok := <-s.inbound:
		if ok {
			t.Errorf("expected closed channel after EOF, got value")
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for inbound close")
	}
	if !s.inboundEOF.Load() {
		t.Errorf("inboundEOF flag should be set after EOF frame")
	}
}

func TestPumpInboundFrames_OutboundFramesIgnored(t *testing.T) {
	// Server-sent frames marked OUTBOUND would be a protocol bug
	// (server sends INBOUND, agent emits OUTBOUND). The pump must
	// tolerate and skip them rather than push their data onto
	// s.inbound, which would confuse the wasm reader.
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	s := &streamCtx{
		inbound:  make(chan []byte, 4),
		outbound: make(chan []byte, 4),
	}
	go pumpInboundFrames(context.Background(), a, s)

	mustWriteFrame(t, b, &v2pb.PluginStreamFrame{
		Source: v2pb.PluginStreamFrame_SOURCE_OUTBOUND,
		Kind:   v2pb.PluginStreamFrame_KIND_DATA,
		Data:   []byte("not-mine"),
	})
	mustWriteFrame(t, b, &v2pb.PluginStreamFrame{
		Source: v2pb.PluginStreamFrame_SOURCE_INBOUND,
		Kind:   v2pb.PluginStreamFrame_KIND_DATA,
		Data:   []byte("yes-mine"),
	})

	select {
	case got := <-s.inbound:
		if string(got) != "yes-mine" {
			t.Errorf("inbound first frame = %q; OUTBOUND frame leaked into channel", got)
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out")
	}
}

func TestPumpOutboundFrames_ChannelDataBecomesOutboundFrame(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	s := &streamCtx{
		inbound:  make(chan []byte, 4),
		outbound: make(chan []byte, 4),
	}
	done := make(chan struct{})
	go func() {
		pumpOutboundFrames(context.Background(), a, s)
		close(done)
	}()

	s.outbound <- []byte("from-wasm")
	close(s.outbound)
	s.writeClosed.Store(true)

	frame := mustReadFrame(t, b)
	if frame.GetSource() != v2pb.PluginStreamFrame_SOURCE_OUTBOUND ||
		frame.GetKind() != v2pb.PluginStreamFrame_KIND_DATA {
		t.Errorf("first frame = %+v, want OUTBOUND DATA", frame)
	}
	if string(frame.GetData()) != "from-wasm" {
		t.Errorf("data = %q, want from-wasm", frame.GetData())
	}

	frame = mustReadFrame(t, b)
	if frame.GetKind() != v2pb.PluginStreamFrame_KIND_EOF {
		t.Errorf("second frame = %+v, want OUTBOUND EOF", frame)
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("pumpOutboundFrames did not return after channel close")
	}
}

// helpers ---------------------------------------------------------

func mustWriteFrame(t *testing.T, w net.Conn, frame *v2pb.PluginStreamFrame) {
	t.Helper()
	if err := link.WriteFrame(w, frame); err != nil {
		t.Fatalf("write frame: %v", err)
	}
}

func mustReadFrame(t *testing.T, r net.Conn) *v2pb.PluginStreamFrame {
	t.Helper()
	var frame v2pb.PluginStreamFrame
	if err := link.ReadFrame(r, &frame); err != nil {
		t.Fatalf("read frame: %v", err)
	}
	return &frame
}
