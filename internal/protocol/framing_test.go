package protocol

import (
	"bytes"
	"sync"
	"testing"
	"time"

	agentpb "github.com/WangYihang/Platypus/pkg/proto/agent/v1"
)

func TestProtoCodecRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	codec := NewProtoCodec(&buf)

	sent := &agentpb.Envelope{
		Version:   1,
		RequestId: "test-123",
		Timestamp: time.Now().UnixNano(),
		Payload: &agentpb.Envelope_Stdio{
			Stdio: &agentpb.StdioData{
				Key:  "process-key",
				Data: []byte("hello world"),
			},
		},
	}

	if err := codec.Send(sent); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	received, err := codec.Recv()
	if err != nil {
		t.Fatalf("Recv failed: %v", err)
	}

	if received.Version != 1 {
		t.Errorf("version: got %d, want 1", received.Version)
	}
	if received.RequestId != "test-123" {
		t.Errorf("request_id: got %q, want %q", received.RequestId, "test-123")
	}

	stdio := received.GetStdio()
	if stdio == nil {
		t.Fatal("expected StdioData payload")
	}
	if stdio.Key != "process-key" {
		t.Errorf("key: got %q, want %q", stdio.Key, "process-key")
	}
	if string(stdio.Data) != "hello world" {
		t.Errorf("data: got %q, want %q", string(stdio.Data), "hello world")
	}
}

func TestProtoCodecExecRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	codec := NewProtoCodec(&buf)

	sent := &agentpb.Envelope{
		Version:   1,
		RequestId: "exec-456",
		Payload: &agentpb.Envelope_ExecRequest{
			ExecRequest: &agentpb.ExecRequest{
				Command: "whoami",
			},
		},
	}

	if err := codec.Send(sent); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	received, err := codec.Recv()
	if err != nil {
		t.Fatalf("Recv failed: %v", err)
	}

	req := received.GetExecRequest()
	if req == nil {
		t.Fatal("expected ExecRequest payload")
	}
	if req.Command != "whoami" {
		t.Errorf("command: got %q, want %q", req.Command, "whoami")
	}
}

func TestProtoCodecMultipleMessages(t *testing.T) {
	var buf bytes.Buffer
	codec := NewProtoCodec(&buf)

	for i := 0; i < 100; i++ {
		env := &agentpb.Envelope{
			Version:   1,
			RequestId: "multi",
			Payload: &agentpb.Envelope_Stdio{
				Stdio: &agentpb.StdioData{
					Key:  "k",
					Data: []byte{byte(i)},
				},
			},
		}
		if err := codec.Send(env); err != nil {
			t.Fatalf("Send %d failed: %v", i, err)
		}
	}

	for i := 0; i < 100; i++ {
		env, err := codec.Recv()
		if err != nil {
			t.Fatalf("Recv %d failed: %v", i, err)
		}
		data := env.GetStdio().Data
		if len(data) != 1 || data[0] != byte(i) {
			t.Errorf("message %d: got %v, want [%d]", i, data, i)
		}
	}
}

func TestProtoCodecEmptyPayload(t *testing.T) {
	var buf bytes.Buffer
	codec := NewProtoCodec(&buf)

	sent := &agentpb.Envelope{
		Version: 1,
		Payload: &agentpb.Envelope_GetClientInfoRequest{
			GetClientInfoRequest: &agentpb.GetClientInfoRequest{},
		},
	}

	if err := codec.Send(sent); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	received, err := codec.Recv()
	if err != nil {
		t.Fatalf("Recv failed: %v", err)
	}

	if received.GetGetClientInfoRequest() == nil {
		t.Fatal("expected GetClientInfoRequest payload")
	}
}

func TestProtoCodecRecvEmpty(t *testing.T) {
	var buf bytes.Buffer
	codec := NewProtoCodec(&buf)

	_, err := codec.Recv()
	if err == nil {
		t.Fatal("expected error on empty buffer")
	}
}

func TestProtoCodecConcurrent(t *testing.T) {
	var buf bytes.Buffer
	codec := NewProtoCodec(&buf)

	var wg sync.WaitGroup
	// Concurrent writes
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codec.Send(&agentpb.Envelope{
				Version: 1,
				Payload: &agentpb.Envelope_Stdio{
					Stdio: &agentpb.StdioData{Key: "k", Data: []byte("x")},
				},
			})
		}()
	}
	wg.Wait()

	// Sequential reads (concurrent reads on bytes.Buffer would race)
	for i := 0; i < 50; i++ {
		_, err := codec.Recv()
		if err != nil {
			t.Fatalf("Recv %d failed: %v", i, err)
		}
	}
}
