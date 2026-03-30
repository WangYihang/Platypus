package protocol

import (
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	agentpb "github.com/WangYihang/Platypus/pkg/proto/agent/v1"
)

// TestAgentServerRoundTrip simulates a full Agent↔Server communication over net.Pipe.
func TestAgentServerRoundTrip(t *testing.T) {
	agentConn, serverConn := net.Pipe()
	defer agentConn.Close()
	defer serverConn.Close()

	agentCodec := NewProtoCodec(agentConn)
	serverCodec := NewProtoCodec(serverConn)

	var wg sync.WaitGroup

	// Server sends GET_CLIENT_INFO, agent responds
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Server sends request
		err := serverCodec.Send(&agentpb.Envelope{
			Version:   1,
			RequestId: "info-1",
			Payload: &agentpb.Envelope_GetClientInfoRequest{
				GetClientInfoRequest: &agentpb.GetClientInfoRequest{},
			},
		})
		if err != nil {
			t.Errorf("server send: %v", err)
			return
		}

		// Server receives response
		env, err := serverCodec.Recv()
		if err != nil {
			t.Errorf("server recv: %v", err)
			return
		}
		info := env.GetClientInfoResponse()
		if info == nil {
			t.Error("expected ClientInfoResponse")
			return
		}
		if info.Os != "linux" {
			t.Errorf("os: got %q, want %q", info.Os, "linux")
		}
		if info.User != "testuser" {
			t.Errorf("user: got %q, want %q", info.User, "testuser")
		}
	}()

	// Agent receives request, sends response
	wg.Add(1)
	go func() {
		defer wg.Done()
		env, err := agentCodec.Recv()
		if err != nil {
			t.Errorf("agent recv: %v", err)
			return
		}
		if env.GetGetClientInfoRequest() == nil {
			t.Error("expected GetClientInfoRequest")
			return
		}

		err = agentCodec.Send(&agentpb.Envelope{
			Version:   1,
			RequestId: env.RequestId,
			Payload: &agentpb.Envelope_ClientInfoResponse{
				ClientInfoResponse: &agentpb.ClientInfoResponse{
					Version:  "1.0.0",
					Os:       "linux",
					Arch:     "amd64",
					User:     "testuser",
					Hostname: "testhost",
					NetworkInterfaces: map[string]string{
						"eth0": "00:11:22:33:44:55",
					},
					AvailableLanguages: []string{"python3"},
				},
			},
		})
		if err != nil {
			t.Errorf("agent send: %v", err)
		}
	}()

	wg.Wait()
}

// TestExecRPC simulates the command execution RPC flow.
func TestExecRPC(t *testing.T) {
	agentConn, serverConn := net.Pipe()
	defer agentConn.Close()
	defer serverConn.Close()

	agentCodec := NewProtoCodec(agentConn)
	serverCodec := NewProtoCodec(serverConn)

	var wg sync.WaitGroup

	// Server sends exec request
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := serverCodec.Send(&agentpb.Envelope{
			Version:   1,
			RequestId: "exec-123",
			Payload: &agentpb.Envelope_ExecRequest{
				ExecRequest: &agentpb.ExecRequest{Command: "whoami"},
			},
		})
		if err != nil {
			t.Errorf("server send: %v", err)
			return
		}

		env, err := serverCodec.Recv()
		if err != nil {
			t.Errorf("server recv: %v", err)
			return
		}
		resp := env.GetExecResponse()
		if resp == nil {
			t.Error("expected ExecResponse")
			return
		}
		if string(resp.Output) != "root\n" {
			t.Errorf("output: got %q, want %q", string(resp.Output), "root\n")
		}
		if resp.ExitCode != 0 {
			t.Errorf("exit_code: got %d, want 0", resp.ExitCode)
		}
		if env.RequestId != "exec-123" {
			t.Errorf("request_id: got %q, want %q", env.RequestId, "exec-123")
		}
	}()

	// Agent receives exec, responds
	wg.Add(1)
	go func() {
		defer wg.Done()
		env, err := agentCodec.Recv()
		if err != nil {
			t.Errorf("agent recv: %v", err)
			return
		}
		req := env.GetExecRequest()
		if req == nil {
			t.Error("expected ExecRequest")
			return
		}
		if req.Command != "whoami" {
			t.Errorf("command: got %q, want %q", req.Command, "whoami")
		}

		err = agentCodec.Send(&agentpb.Envelope{
			Version:   1,
			RequestId: env.RequestId,
			Payload: &agentpb.Envelope_ExecResponse{
				ExecResponse: &agentpb.ExecResponse{
					Output:   []byte("root\n"),
					ExitCode: 0,
				},
			},
		})
		if err != nil {
			t.Errorf("agent send: %v", err)
		}
	}()

	wg.Wait()
}

// TestFileOpsRPC simulates file read/write RPC.
func TestFileOpsRPC(t *testing.T) {
	agentConn, serverConn := net.Pipe()
	defer agentConn.Close()
	defer serverConn.Close()

	agentCodec := NewProtoCodec(agentConn)
	serverCodec := NewProtoCodec(serverConn)

	var wg sync.WaitGroup

	// Server sends file size request
	wg.Add(1)
	go func() {
		defer wg.Done()
		serverCodec.Send(&agentpb.Envelope{
			Version:   1,
			RequestId: "fsize-1",
			Payload: &agentpb.Envelope_FileSizeRequest{
				FileSizeRequest: &agentpb.FileSizeRequest{Path: "/etc/passwd"},
			},
		})

		env, _ := serverCodec.Recv()
		resp := env.GetFileSizeResponse()
		if resp == nil {
			t.Error("expected FileSizeResponse")
			return
		}
		if resp.Size != 1234 {
			t.Errorf("size: got %d, want 1234", resp.Size)
		}

		// Now test read file
		serverCodec.Send(&agentpb.Envelope{
			Version:   1,
			RequestId: "read-1",
			Payload: &agentpb.Envelope_ReadFileRequest{
				ReadFileRequest: &agentpb.ReadFileRequest{Path: "/etc/passwd", Offset: 0, Size: 100},
			},
		})

		env, _ = serverCodec.Recv()
		readResp := env.GetReadFileResponse()
		if readResp == nil {
			t.Error("expected ReadFileResponse")
			return
		}
		if string(readResp.Data) != "root:x:0:0:root" {
			t.Errorf("data: got %q", string(readResp.Data))
		}
	}()

	// Agent handles both requests
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Handle file size
		env, _ := agentCodec.Recv()
		agentCodec.Send(&agentpb.Envelope{
			RequestId: env.RequestId,
			Payload: &agentpb.Envelope_FileSizeResponse{
				FileSizeResponse: &agentpb.FileSizeResponse{Size: 1234},
			},
		})

		// Handle read file
		env, _ = agentCodec.Recv()
		agentCodec.Send(&agentpb.Envelope{
			RequestId: env.RequestId,
			Payload: &agentpb.Envelope_ReadFileResponse{
				ReadFileResponse: &agentpb.ReadFileResponse{Data: []byte("root:x:0:0:root")},
			},
		})
	}()

	wg.Wait()
}

// TestStdioStreaming simulates bidirectional terminal I/O.
func TestStdioStreaming(t *testing.T) {
	agentConn, serverConn := net.Pipe()
	defer agentConn.Close()
	defer serverConn.Close()

	agentCodec := NewProtoCodec(agentConn)
	serverCodec := NewProtoCodec(serverConn)

	key := "proc-abc"
	const count = 10

	// Agent echoes back in background
	go func() {
		for i := 0; i < count; i++ {
			env, err := agentCodec.Recv()
			if err != nil {
				return
			}
			agentCodec.Send(&agentpb.Envelope{
				Payload: &agentpb.Envelope_Stdio{
					Stdio: &agentpb.StdioData{Key: env.GetStdio().Key, Data: env.GetStdio().Data},
				},
			})
		}
	}()

	// Server sends then receives one at a time (net.Pipe is synchronous)
	for i := 0; i < count; i++ {
		serverCodec.Send(&agentpb.Envelope{
			Payload: &agentpb.Envelope_Stdio{
				Stdio: &agentpb.StdioData{Key: key, Data: []byte{byte('a' + i)}},
			},
		})

		env, err := serverCodec.Recv()
		if err != nil {
			t.Fatalf("recv output %d: %v", i, err)
		}
		data := env.GetStdio()
		if data == nil || data.Key != key {
			t.Fatalf("output %d: wrong key", i)
		}
		if len(data.Data) != 1 || data.Data[0] != byte('a'+i) {
			t.Fatalf("output %d: got %v, want %v", i, data.Data, byte('a'+i))
		}
	}
}

// TestProtocolVersionMismatch tests that version field is properly preserved.
func TestProtocolVersionMismatch(t *testing.T) {
	agentConn, serverConn := net.Pipe()
	defer agentConn.Close()
	defer serverConn.Close()

	agentCodec := NewProtoCodec(agentConn)
	serverCodec := NewProtoCodec(serverConn)

	done := make(chan struct{})
	go func() {
		defer close(done)
		serverCodec.Send(&agentpb.Envelope{
			Version: 99,
			Payload: &agentpb.Envelope_GetClientInfoRequest{
				GetClientInfoRequest: &agentpb.GetClientInfoRequest{},
			},
		})
	}()

	env, err := agentCodec.Recv()
	if err != nil {
		t.Fatalf("recv: %v", err)
	}
	if env.Version != 99 {
		t.Errorf("version: got %d, want 99", env.Version)
	}
	<-done
}

// TestLargePayload tests sending a large file chunk.
func TestLargePayload(t *testing.T) {
	agentConn, serverConn := net.Pipe()
	defer agentConn.Close()
	defer serverConn.Close()

	agentCodec := NewProtoCodec(agentConn)
	serverCodec := NewProtoCodec(serverConn)

	// 1MB payload
	data := make([]byte, 1024*1024)
	for i := range data {
		data[i] = byte(i % 256)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		serverCodec.Send(&agentpb.Envelope{
			Version:   1,
			RequestId: "big",
			Payload: &agentpb.Envelope_ReadFileResponse{
				ReadFileResponse: &agentpb.ReadFileResponse{Data: data},
			},
		})
	}()

	env, err := agentCodec.Recv()
	if err != nil {
		t.Fatalf("recv: %v", err)
	}
	resp := env.GetReadFileResponse()
	if resp == nil {
		t.Fatal("expected ReadFileResponse")
	}
	if len(resp.Data) != 1024*1024 {
		t.Errorf("data length: got %d, want %d", len(resp.Data), 1024*1024)
	}
	<-done
}

// TestConcurrentRPCs simulates multiple concurrent RPC calls.
func TestConcurrentRPCs(t *testing.T) {
	agentConn, serverConn := net.Pipe()
	defer agentConn.Close()
	defer serverConn.Close()

	agentCodec := NewProtoCodec(agentConn)
	serverCodec := NewProtoCodec(serverConn)

	const numRPCs = 20
	var wg sync.WaitGroup

	// Agent handler: reads requests, sends responses
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < numRPCs; i++ {
			env, err := agentCodec.Recv()
			if err != nil {
				t.Errorf("agent recv %d: %v", i, err)
				return
			}
			agentCodec.Send(&agentpb.Envelope{
				Version:   1,
				RequestId: env.RequestId,
				Payload: &agentpb.Envelope_ExecResponse{
					ExecResponse: &agentpb.ExecResponse{
						Output:   []byte("ok-" + env.RequestId),
						ExitCode: 0,
					},
				},
			})
		}
	}()

	// Server sends all requests sequentially (net.Pipe is synchronous)
	responses := make([]*agentpb.Envelope, numRPCs)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < numRPCs; i++ {
			serverCodec.Send(&agentpb.Envelope{
				Version:   1,
				RequestId: fmt.Sprintf("rpc-%d", i),
				Payload: &agentpb.Envelope_ExecRequest{
					ExecRequest: &agentpb.ExecRequest{Command: "test"},
				},
			})

			env, err := serverCodec.Recv()
			if err != nil {
				t.Errorf("server recv %d: %v", i, err)
				return
			}
			responses[i] = env
		}
	}()

	wg.Wait()

	for i, env := range responses {
		if env == nil {
			t.Errorf("response %d is nil", i)
			continue
		}
		expected := fmt.Sprintf("rpc-%d", i)
		if env.RequestId != expected {
			t.Errorf("response %d: request_id got %q, want %q", i, env.RequestId, expected)
		}
	}
}

// TestConnectionClose tests behavior when one side closes.
func TestConnectionClose(t *testing.T) {
	agentConn, serverConn := net.Pipe()
	serverCodec := NewProtoCodec(serverConn)

	// Close agent side immediately
	agentConn.Close()

	// Server should get error
	_, err := serverCodec.Recv()
	if err == nil {
		t.Fatal("expected error after connection close")
	}
}

// TestTimeout tests that operations fail properly with deadlines.
func TestTimeout(t *testing.T) {
	agentConn, serverConn := net.Pipe()
	defer agentConn.Close()
	defer serverConn.Close()

	serverCodec := NewProtoCodec(serverConn)

	// Set a short deadline
	serverConn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))

	_, err := serverCodec.Recv()
	if err == nil {
		t.Fatal("expected timeout error")
	}
}
