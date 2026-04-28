package link

import (
	"context"
	"io"
	"testing"
	"time"

	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// CallRPC is the client-side RPC primitive: open a STREAM_TYPE_RPC
// stream, write exactly one RpcRequest, read exactly one RpcResponse,
// close. The counterpart on the agent side (a stream handler) will
// read the request, dispatch to an in-process RPC handler, and write
// the response.

func TestCallRPC_Roundtrip(t *testing.T) {
	client, server := pairedSessions(t)

	// Server side: read the request, echo it back in ExecResponse.stdout.
	done := make(chan error, 1)
	go func() {
		_, stream, err := server.Accept()
		if err != nil {
			done <- err
			return
		}
		defer stream.Close()

		var req v2pb.RpcRequest
		if err := ReadFrame(stream, &req); err != nil {
			done <- err
			return
		}
		exec := req.GetExec()
		if exec == nil {
			done <- errRPCMismatch("expected Exec payload")
			return
		}
		resp := &v2pb.RpcResponse{Payload: &v2pb.RpcResponse_Exec{Exec: &v2pb.ExecResponse{
			Stdout:   []byte("echo: " + exec.Command),
			ExitCode: 0,
		}}}
		if err := WriteFrame(stream, resp); err != nil {
			done <- err
			return
		}
		done <- nil
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := CallRPC(ctx, client, &v2pb.RpcRequest{
		Payload: &v2pb.RpcRequest_Exec{Exec: &v2pb.ExecRequest{Command: "hello"}},
	}, "test-corr")
	if err != nil {
		t.Fatalf("CallRPC: %v", err)
	}
	exec := resp.GetExec()
	if exec == nil {
		t.Fatal("response Exec payload missing")
	}
	if string(exec.Stdout) != "echo: hello" {
		t.Fatalf("stdout = %q; want %q", exec.Stdout, "echo: hello")
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("server goroutine: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("server goroutine timed out")
	}
}

// If the peer never writes a response and the context expires, the
// call must return promptly rather than blocking forever on the read.
func TestCallRPC_ContextTimeout(t *testing.T) {
	client, server := pairedSessions(t)

	// Server: accept the stream, read the request, hang. We don't
	// read the request here either — just hold the stream open.
	go func() {
		_, stream, err := server.Accept()
		if err != nil {
			return
		}
		// Hold the stream until test teardown closes the session.
		time.Sleep(2 * time.Second)
		_ = stream.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := CallRPC(ctx, client, &v2pb.RpcRequest{
		Payload: &v2pb.RpcRequest_Exec{Exec: &v2pb.ExecRequest{Command: "x"}},
	}, "test-corr")
	if err == nil {
		t.Fatal("CallRPC should have returned an error on ctx timeout")
	}
}

// A server that closes the stream before writing a response must
// surface as an error (io.EOF / io.ErrUnexpectedEOF), not a zero
// RpcResponse. Handlers that never received a response need to
// distinguish that from "response says error=” successfully".
func TestCallRPC_StreamClosedBeforeResponse(t *testing.T) {
	client, server := pairedSessions(t)

	go func() {
		_, stream, err := server.Accept()
		if err != nil {
			return
		}
		// Consume the request then close without replying.
		var req v2pb.RpcRequest
		_ = ReadFrame(stream, &req)
		_ = stream.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := CallRPC(ctx, client, &v2pb.RpcRequest{
		Payload: &v2pb.RpcRequest_Exec{Exec: &v2pb.ExecRequest{Command: "x"}},
	}, "test-corr")
	if err == nil {
		t.Fatal("CallRPC should error when server closes without responding")
	}
	// Accept either EOF variant — yamux reports the closed-stream
	// end in slightly different ways depending on whether the close
	// raced the reader.
	if err != io.EOF && err != io.ErrUnexpectedEOF &&
		!contains(err.Error(), "EOF") {
		t.Fatalf("err = %v; want an EOF-shaped error", err)
	}
}

// contains is a tiny helper so the test doesn't reach for strings.
func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// errRPCMismatch is a minimal error type so failures in server
// goroutines land with a useful message.
type rpcMismatchErr struct{ msg string }

func (e *rpcMismatchErr) Error() string { return e.msg }
func errRPCMismatch(msg string) error   { return &rpcMismatchErr{msg: msg} }
