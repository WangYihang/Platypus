package agent

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"github.com/WangYihang/Platypus/internal/link"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// ServeLink runs the agent-side accept loop: block for each
// incoming stream, look at StreamHeader.Type, and dispatch to the
// matching handler. Streams with no handler get a StreamReject.
// Loop exits on session close or ctx cancel.

// Paired sessions exposed to agent tests: builds yamux-over-pipe
// client + server sessions.
func pairedAgentSessions(t *testing.T) (*link.Session, *link.Session) {
	t.Helper()
	clientConn, serverConn := net.Pipe()
	serverCh := make(chan *link.Session, 1)
	go func() {
		s, err := link.NewServerSession(serverConn)
		if err != nil {
			t.Errorf("server session: %v", err)
			return
		}
		serverCh <- s
	}()
	clientSess, err := link.NewClientSession(clientConn)
	if err != nil {
		t.Fatalf("client session: %v", err)
	}
	serverSess := <-serverCh
	t.Cleanup(func() {
		clientSess.Close()
		serverSess.Close()
	})
	return clientSess, serverSess
}

// Happy path: client opens an RPC stream with an Exec request,
// ServeLink's RPC dispatcher runs, client gets the exec response.
func TestServeLink_DispatchesRPC(t *testing.T) {
	clientSess, agentSess := pairedAgentSessions(t)

	deps := AgentHandlerDeps{
		RPC: AgentRPCHandlers{
			Exec: func(_ context.Context, req *v2pb.ExecRequest) *v2pb.ExecResponse {
				return &v2pb.ExecResponse{Stdout: []byte("from agent: " + req.Command)}
			},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- ServeLink(ctx, agentSess, deps)
	}()

	callCtx, callCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer callCancel()
	resp, err := link.CallRPC(callCtx, clientSess, &v2pb.RpcRequest{
		Payload: &v2pb.RpcRequest_Exec{Exec: &v2pb.ExecRequest{Command: "uname"}},
	}, "test-corr")
	if err != nil {
		t.Fatalf("CallRPC: %v", err)
	}
	exec := resp.GetExec()
	if exec == nil || string(exec.Stdout) != "from agent: uname" {
		t.Fatalf("unexpected response: %+v", resp)
	}

	// Cancel the serve loop and wait for it to exit cleanly.
	cancel()
	agentSess.Close()
	select {
	case err := <-serveErr:
		if err != nil && !errors.Is(err, context.Canceled) && err != io.EOF {
			t.Fatalf("ServeLink returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("ServeLink did not return after cancel")
	}
}

// ServeLink must wait for in-flight per-stream handlers to finish
// before returning. Without the join, the agent's reconnect loop
// (cmd/platypus-agent/main.go) would race a fresh Bootstrap/Serve
// against a previous session's still-draining handlers — process /
// file write handlers can block in synchronous cleanup (fsync, kill
// + wait), causing goroutines to overlap across reconnects and grow
// over time on flapping links.
//
// Post-Phase-B every stream type other than RPC / upgrade /
// plugin-mgmt is dispatched through PluginStream, so we exercise
// the wait-for-inflight semantics through that path. The test
// injects a PluginStreamDispatcher closure that claims the stream
// and blocks until released.
func TestServeLink_WaitsForInflightHandlers(t *testing.T) {
	clientSess, agentSess := pairedAgentSessions(t)

	handlerEntered := make(chan struct{})
	releaseHandler := make(chan struct{})
	deps := AgentHandlerDeps{
		PluginStream: func(_ context.Context, _ v2pb.StreamType, _ io.ReadWriteCloser, _ []byte) (bool, error) {
			close(handlerEntered)
			<-releaseHandler
			return true, nil
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- ServeLink(ctx, agentSess, deps)
	}()

	// Open a stream that the PluginStream dispatcher will claim and
	// block inside. Any non-built-in type works; PROCESS_OPEN is the
	// canonical example since wasm replacements claim it in production.
	stream, err := clientSess.Open(v2pb.StreamType_STREAM_TYPE_PROCESS_OPEN, nil, "stuck-corr")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer stream.Close()

	select {
	case <-handlerEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("handler never entered")
	}

	// Trigger ServeLink to want to return. Our handler is still
	// blocked on releaseHandler — ServeLink must NOT return yet.
	cancel()
	agentSess.Close()

	select {
	case err := <-serveErr:
		t.Fatalf("ServeLink returned before in-flight handler finished: %v", err)
	case <-time.After(150 * time.Millisecond):
	}

	// Release the handler. ServeLink should now return promptly.
	close(releaseHandler)
	select {
	case <-serveErr:
	case <-time.After(2 * time.Second):
		t.Fatal("ServeLink did not return after handler completed")
	}
}

// Unknown stream type: agent sends a StreamReject frame and closes.
// Peer opening such a stream observes the close with no payload.
func TestServeLink_RejectsUnknownStreamType(t *testing.T) {
	clientSess, agentSess := pairedAgentSessions(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- ServeLink(ctx, agentSess, AgentHandlerDeps{})
	}()

	stream, err := clientSess.Open(v2pb.StreamType_STREAM_TYPE_UNSPECIFIED, nil, "")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer stream.Close()

	// Agent writes a StreamReject frame. We don't enforce its
	// contents here; the test just ensures the handler reacts
	// (reject frame arrives, then the agent closes the stream).
	var rej v2pb.StreamReject
	if err := link.ReadFrame(stream, &rej); err != nil {
		t.Fatalf("ReadFrame reject: %v", err)
	}
	if rej.Code == "" {
		t.Fatal("expected non-empty reject code for unknown stream type")
	}
}
