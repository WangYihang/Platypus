package agent

import (
	"bytes"
	"context"
	"io"
	"net"
	"testing"
	"time"

	"github.com/WangYihang/Platypus/internal/link"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// HandleTunnelPullStream dials req.Target and splices the resulting
// TCP conn with the yamux stream. First frame is a
// TunnelPullResponse (error populated on dial failure); after that,
// the stream carries raw TCP bytes bidirectionally — no protobuf
// framing.

// Bidirectional splice: agent echoes bytes that arrive at the
// target conn back to whoever wrote them.
func TestHandleTunnelPullStream_Bidirectional(t *testing.T) {
	// Stand up an echo server the agent will dial.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				_, _ = io.Copy(c, c)
			}(c)
		}
	}()

	client, server := pairedProcessStreams(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- HandleTunnelPullStream(ctx, server, &v2pb.TunnelPullRequest{
			Target: ln.Addr().String(),
		})
	}()

	// Read the ack.
	var ack v2pb.TunnelPullResponse
	if err := link.ReadFrame(client, &ack); err != nil {
		t.Fatalf("read ack: %v", err)
	}
	if ack.Error != "" {
		t.Fatalf("ack error: %s", ack.Error)
	}

	// Write raw bytes to the client side; echo server should bounce
	// them back through the agent's splice.
	payload := []byte("hello over tunnel\n")
	if _, err := client.Write(payload); err != nil {
		t.Fatalf("client write: %v", err)
	}
	// Read exactly len(payload) bytes back.
	got := make([]byte, len(payload))
	if _, err := io.ReadFull(client, got); err != nil {
		t.Fatalf("client readfull: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("echo mismatch: got %q; want %q", got, payload)
	}

	// Close client side → agent splice terminates → handler returns.
	_ = client.Close()
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("handler did not return after client close")
	}
}

// Dial failure (nothing listening at target) surfaces in ack.Error.
func TestHandleTunnelPullStream_DialFails(t *testing.T) {
	client, server := pairedProcessStreams(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- HandleTunnelPullStream(ctx, server, &v2pb.TunnelPullRequest{
			// Port 1 is reserved "user protocol"; no service listens there.
			Target:        "127.0.0.1:1",
			DialTimeoutMs: 300,
		})
	}()

	var ack v2pb.TunnelPullResponse
	if err := link.ReadFrame(client, &ack); err != nil {
		t.Fatalf("read ack: %v", err)
	}
	if ack.Error == "" {
		t.Fatal("expected ack.Error for dial failure")
	}
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("handler did not return after dial failure")
	}
}
