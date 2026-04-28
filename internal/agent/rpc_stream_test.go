package agent

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/WangYihang/Platypus/internal/link"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// ServeRPCStream is the agent-side counterpart of link.CallRPC.
// For each accepted STREAM_TYPE_RPC stream: read one RpcRequest,
// dispatch to a handler selected by the payload oneof tag, write
// one RpcResponse, close.

// Happy path: client issues Exec, handler returns stdout, client
// sees the expected output.
func TestServeRPCStream_DispatchesExec(t *testing.T) {
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
	defer clientSess.Close()
	defer serverSess.Close()

	// Agent-side accept loop running one handler for exec only.
	handlers := AgentRPCHandlers{
		Exec: func(_ context.Context, req *v2pb.ExecRequest) *v2pb.ExecResponse {
			return &v2pb.ExecResponse{
				Stdout:   []byte("ran: " + req.Command),
				ExitCode: 0,
			}
		},
	}
	go func() {
		_, stream, err := serverSess.Accept()
		if err != nil {
			return
		}
		_ = ServeRPCStream(context.Background(), stream, handlers)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := link.CallRPC(ctx, clientSess, &v2pb.RpcRequest{
		Payload: &v2pb.RpcRequest_Exec{Exec: &v2pb.ExecRequest{Command: "ls"}},
	}, "test-corr")
	if err != nil {
		t.Fatalf("CallRPC: %v", err)
	}
	if exec := resp.GetExec(); exec == nil || string(exec.Stdout) != "ran: ls" {
		t.Fatalf("resp Exec = %+v; want stdout=ran: ls", resp)
	}
	if resp.Error != "" {
		t.Fatalf("resp.Error = %q; want empty", resp.Error)
	}
}

// When the agent has no handler for a request type, the response
// still arrives but carries a descriptive Error so clients can
// distinguish "handler missing" from "handler failed".
func TestServeRPCStream_UnsupportedPayload(t *testing.T) {
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
	clientSess, _ := link.NewClientSession(clientConn)
	serverSess := <-serverCh
	defer clientSess.Close()
	defer serverSess.Close()

	// Agent has NO handler for ListDir.
	handlers := AgentRPCHandlers{}
	go func() {
		_, stream, err := serverSess.Accept()
		if err != nil {
			return
		}
		_ = ServeRPCStream(context.Background(), stream, handlers)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := link.CallRPC(ctx, clientSess, &v2pb.RpcRequest{
		Payload: &v2pb.RpcRequest_ListDir{ListDir: &v2pb.ListDirRequest{Path: "/"}},
	}, "test-corr")
	if err != nil {
		t.Fatalf("CallRPC: %v", err)
	}
	if resp.Error == "" {
		t.Fatal("expected non-empty resp.Error for unsupported payload")
	}
	if resp.GetListDir() != nil {
		t.Fatalf("expected no ListDir payload for unsupported type; got %+v", resp.GetListDir())
	}
}
