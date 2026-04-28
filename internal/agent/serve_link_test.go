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
