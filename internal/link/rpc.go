package link

import (
	"context"
	"errors"
	"fmt"

	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// CallRPC is the caller-side RPC primitive. It opens a new
// STREAM_TYPE_RPC stream on sess, writes exactly one RpcRequest
// frame, reads exactly one RpcResponse frame, and closes the
// stream. Context cancellation / deadline aborts the call cleanly
// — the underlying stream is closed, releasing any server-side
// reader blocked on it.
//
// correlationID identifies a single request/response pair and is
// echoed in StreamHeader.correlation_id so both peers can grep one
// round-trip out of the log stream. The link-session id (stable for
// the lifetime of the agent connection) is read off sess itself —
// callers no longer need to thread it explicitly.
//
// Errors mean "the RPC did not complete"; callers must not
// interpret a nil err + populated RpcResponse.Error as anything
// other than a service-level failure reported by the peer.
func CallRPC(ctx context.Context, sess *Session, req *v2pb.RpcRequest, correlationID string) (*v2pb.RpcResponse, error) {
	if req == nil {
		return nil, errors.New("link: CallRPC: nil request")
	}

	stream, err := sess.Open(v2pb.StreamType_STREAM_TYPE_RPC, nil, correlationID)
	if err != nil {
		return nil, fmt.Errorf("link: CallRPC open: %w", err)
	}
	// Closing on every path; a done-channel pattern lets us close
	// the stream from the ctx watcher so a blocked ReadFrame
	// returns promptly.
	closed := make(chan struct{})
	defer func() {
		_ = stream.Close()
		close(closed)
	}()
	go func() {
		select {
		case <-ctx.Done():
			_ = stream.Close()
		case <-closed:
		}
	}()

	if err := WriteFrame(stream, req); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("link: CallRPC write: %w", err)
	}

	var resp v2pb.RpcResponse
	if err := ReadFrame(stream, &resp); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	return &resp, nil
}
