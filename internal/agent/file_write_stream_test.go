package agent

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/WangYihang/Platypus/internal/link"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// HandleFileWriteStream is the agent-side handler for a
// STREAM_TYPE_FILE_WRITE stream. The agent acks with
// FileWriteResponse, consumes FileChunk frames written by the
// server, writes bytes to the destination, and emits a final
// FileWriteResult.

func TestHandleFileWriteStream_Happy(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "out")
	payload := bytes.Repeat([]byte("AB"), 500) // 1000 bytes

	client, server := pairedProcessStreams(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- HandleFileWriteStream(ctx, server, &v2pb.FileWriteRequest{
			Path: dest, Mode: 0o644,
		})
	}()

	// Expect ack.
	var ack v2pb.FileWriteResponse
	if err := link.ReadFrame(client, &ack); err != nil {
		t.Fatalf("read ack: %v", err)
	}
	if ack.Error != "" {
		t.Fatalf("ack error: %s", ack.Error)
	}

	// Send a single chunk with eof=true.
	if err := link.WriteFrame(client, &v2pb.FileChunk{Data: payload, Eof: true}); err != nil {
		t.Fatalf("write chunk: %v", err)
	}

	// Result frame.
	var res v2pb.FileWriteResult
	if err := link.ReadFrame(client, &res); err != nil {
		t.Fatalf("read result: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("result error: %s", res.Error)
	}
	if res.BytesWritten != int64(len(payload)) {
		t.Fatalf("bytes_written = %d; want %d", res.BytesWritten, len(payload))
	}

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read back dest: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("dest contents mismatch: got %d bytes; want %d", len(got), len(payload))
	}
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("handler did not return")
	}
}

// Append mode: second handler run with append=true extends the
// file instead of truncating.
func TestHandleFileWriteStream_Append(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "log")
	if err := os.WriteFile(dest, []byte("one\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	client, server := pairedProcessStreams(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- HandleFileWriteStream(ctx, server, &v2pb.FileWriteRequest{
			Path: dest, Append: true,
		})
	}()

	var ack v2pb.FileWriteResponse
	_ = link.ReadFrame(client, &ack)
	_ = link.WriteFrame(client, &v2pb.FileChunk{Data: []byte("two\n"), Eof: true})
	var res v2pb.FileWriteResult
	_ = link.ReadFrame(client, &res)

	got, _ := os.ReadFile(dest)
	if string(got) != "one\ntwo\n" {
		t.Fatalf("append result = %q; want %q", got, "one\ntwo\n")
	}
	<-done
}

// mkdirs=true creates parent directory tree.
func TestHandleFileWriteStream_Mkdirs(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "a/b/c/leaf")

	client, server := pairedProcessStreams(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- HandleFileWriteStream(ctx, server, &v2pb.FileWriteRequest{
			Path: dest, Mkdirs: true,
		})
	}()

	var ack v2pb.FileWriteResponse
	if err := link.ReadFrame(client, &ack); err != nil {
		t.Fatalf("read ack: %v", err)
	}
	if ack.Error != "" {
		t.Fatalf("ack error: %s", ack.Error)
	}
	_ = link.WriteFrame(client, &v2pb.FileChunk{Data: []byte("x"), Eof: true})
	var res v2pb.FileWriteResult
	_ = link.ReadFrame(client, &res)
	if _, err := os.Stat(dest); err != nil {
		t.Fatalf("dest not created: %v", err)
	}
	<-done
}

// Path in missing parent without mkdirs → FileWriteResponse.Error.
func TestHandleFileWriteStream_NoMkdirsFails(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "does/not/exist/leaf")

	client, server := pairedProcessStreams(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- HandleFileWriteStream(ctx, server, &v2pb.FileWriteRequest{
			Path: dest, Mkdirs: false,
		})
	}()

	var ack v2pb.FileWriteResponse
	if err := link.ReadFrame(client, &ack); err != nil {
		t.Fatalf("read ack: %v", err)
	}
	if ack.Error == "" {
		t.Fatal("expected ack.Error to be non-empty for missing parent dir")
	}
	<-done
}
