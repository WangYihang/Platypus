package agent

import (
	"context"
	"io"
	"net"
	"time"

	"github.com/WangYihang/Platypus/internal/link"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// defaultTunnelDialTimeout applies when TunnelPullRequest leaves
// dial_timeout_ms at zero.
const defaultTunnelDialTimeout = 10 * time.Second

// HandleTunnelPullStream is the agent-side handler for a
// STREAM_TYPE_TUNNEL_PULL stream (local-to-remote forwarding).
// The agent dials req.Target and splices the resulting TCP conn
// with the yamux stream. The stream's layout is:
//
//	[TunnelPullResponse frame — one protobuf, length-prefixed]
//	[raw bytes, bidirectional, until either side closes]
//
// On dial failure TunnelPullResponse.Error is populated and the
// stream closes immediately — no byte splice is attempted.
func HandleTunnelPullStream(ctx context.Context, stream io.ReadWriteCloser, req *v2pb.TunnelPullRequest) error {
	defer func() { _ = stream.Close() }()
	if req == nil || req.Target == "" {
		return writeTunnelPullAck(stream, "", "empty target")
	}

	dialCtx := ctx
	if req.DialTimeoutMs > 0 {
		var cancel context.CancelFunc
		dialCtx, cancel = context.WithTimeout(ctx, time.Duration(req.DialTimeoutMs)*time.Millisecond)
		defer cancel()
	} else {
		var cancel context.CancelFunc
		dialCtx, cancel = context.WithTimeout(ctx, defaultTunnelDialTimeout)
		defer cancel()
	}

	var d net.Dialer
	conn, err := d.DialContext(dialCtx, "tcp", req.Target)
	if err != nil {
		return writeTunnelPullAck(stream, "", err.Error())
	}
	defer func() { _ = conn.Close() }()

	if err := writeTunnelPullAck(stream, conn.RemoteAddr().String(), ""); err != nil {
		return err
	}

	// Bidirectional splice. Two goroutines, whichever side EOFs
	// first triggers the other to unwind — closing both halves so
	// io.Copy on the other side returns.
	return spliceBidirectional(ctx, stream, conn)
}

// spliceBidirectional runs two io.Copy calls — one in each
// direction — between a and b. Returns when either direction
// finishes; closes both ends to unblock the surviving Copy.
func spliceBidirectional(ctx context.Context, a, b io.ReadWriteCloser) error {
	done := make(chan error, 2)
	go func() {
		_, err := io.Copy(b, a)
		_ = b.Close()
		done <- err
	}()
	go func() {
		_, err := io.Copy(a, b)
		_ = a.Close()
		done <- err
	}()
	// Wait for the first direction to finish.
	select {
	case <-done:
	case <-ctx.Done():
		_ = a.Close()
		_ = b.Close()
	}
	// Drain the second so we don't leak a goroutine.
	<-done
	return nil
}

func writeTunnelPullAck(stream io.Writer, resolved, errMsg string) error {
	return link.WriteFrame(stream, &v2pb.TunnelPullResponse{
		ResolvedAddr: resolved,
		Error:        errMsg,
	})
}
