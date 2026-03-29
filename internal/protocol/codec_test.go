package protocol

import (
	"bytes"
	"testing"

	"github.com/WangYihang/Platypus/internal/utils/message"
)

func init() {
	message.RegisterGob()
}

func TestCodecSendRecv(t *testing.T) {
	var buf bytes.Buffer
	codec := NewCodec(&buf)

	sent := message.Message{
		Type: message.STDIO,
		Body: message.BodyStdio{
			Key:  "test-key",
			Data: []byte("hello world"),
		},
	}

	if err := codec.Send(sent); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	var received message.Message
	if err := codec.Recv(&received); err != nil {
		t.Fatalf("Recv failed: %v", err)
	}

	if received.Type != message.STDIO {
		t.Fatalf("expected type STDIO, got %d", received.Type)
	}

	body, ok := received.Body.(*message.BodyStdio)
	if !ok {
		t.Fatal("expected body type *BodyStdio")
	}
	if body.Key != "test-key" {
		t.Fatalf("expected key 'test-key', got '%s'", body.Key)
	}
	if string(body.Data) != "hello world" {
		t.Fatalf("expected data 'hello world', got '%s'", string(body.Data))
	}
}

func TestCodecRecvEmpty(t *testing.T) {
	var buf bytes.Buffer
	codec := NewCodec(&buf)

	var msg message.Message
	err := codec.Recv(&msg)
	if err == nil {
		t.Fatal("expected error on empty buffer")
	}
}
