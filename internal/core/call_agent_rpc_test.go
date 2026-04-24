package core

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/WangYihang/Platypus/internal/link"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// CallAgentRPC is the glue between server-side HTTP handlers and
// the v2 RPC primitive: given an agent_id, look up the Session in
// AgentLinkService, and delegate to link.CallRPC. Handlers get a
// one-call path without having to know about AgentLinkService
// internals.

func TestCallAgentRPC_Roundtrip(t *testing.T) {
	// Pair a client + server yamux Session over net.Pipe, register
	// the server half in the AgentLinkService under a test agent id.
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

	svc := NewAgentLinkService()
	// In production the server keeps the live peer-facing Session
	// (the one yamux-wraps the WS connection). Here, to test the
	// helper without a WS layer, we register the client side as the
	// "agent" and handle streams on the server side.
	svc.Register("agent-x", clientSess)

	// Server goroutine: accept, echo an ExecResponse.
	go func() {
		_, stream, err := serverSess.Accept()
		if err != nil {
			return
		}
		defer stream.Close()
		var req v2pb.RpcRequest
		if err := link.ReadFrame(stream, &req); err != nil {
			return
		}
		_ = link.WriteFrame(stream, &v2pb.RpcResponse{
			Payload: &v2pb.RpcResponse_Exec{Exec: &v2pb.ExecResponse{
				Stdout:   []byte("from handler: " + req.GetExec().Command),
				ExitCode: 0,
			}},
		})
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := CallAgentRPC(ctx, svc, "agent-x", &v2pb.RpcRequest{
		Payload: &v2pb.RpcRequest_Exec{Exec: &v2pb.ExecRequest{Command: "date"}},
	})
	if err != nil {
		t.Fatalf("CallAgentRPC: %v", err)
	}
	if exec := resp.GetExec(); exec == nil || string(exec.Stdout) != "from handler: date" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	_ = clientSess.Close()
	_ = serverSess.Close()
}

func TestCallAgentRPC_UnknownAgent(t *testing.T) {
	svc := NewAgentLinkService()
	_, err := CallAgentRPC(context.Background(), svc, "missing", &v2pb.RpcRequest{})
	if err == nil {
		t.Fatal("want error for unknown agent id")
	}
}
