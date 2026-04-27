package agent

import (
	"bytes"
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/WangYihang/Platypus/internal/link"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// HandleFileReadStream reads the named file and streams its bytes
// back as a sequence of FileChunk frames. First frame is a
// FileReadResponse carrying size + mode (or an error).

func TestHandleFileReadStream_Happy(t *testing.T) {
	dir := t.TempDir()
	want := bytes.Repeat([]byte("platypus\n"), 1024) // ~9 KiB
	path := filepath.Join(dir, "src")
	if err := os.WriteFile(path, want, 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	client, server := pairedProcessStreams(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- HandleFileReadStream(ctx, server, &v2pb.FileReadRequest{Path: path})
	}()

	var hdr v2pb.FileReadResponse
	if err := link.ReadFrame(client, &hdr); err != nil {
		t.Fatalf("read header: %v", err)
	}
	if hdr.Error != "" {
		t.Fatalf("header error: %s", hdr.Error)
	}
	if hdr.Size != int64(len(want)) {
		t.Fatalf("size = %d; want %d", hdr.Size, len(want))
	}

	var got []byte
	for {
		var ch v2pb.FileChunk
		if err := link.ReadFrame(client, &ch); err != nil {
			t.Fatalf("read chunk: %v", err)
		}
		got = append(got, ch.Data...)
		if ch.Eof {
			if ch.Error != "" {
				t.Fatalf("final chunk error: %s", ch.Error)
			}
			break
		}
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("round-trip mismatch: got %d bytes, want %d", len(got), len(want))
	}
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("handler did not return")
	}
}

// Missing file → FileReadResponse.Error populated, no FileChunk.
func TestHandleFileReadStream_MissingFile(t *testing.T) {
	client, server := pairedProcessStreams(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- HandleFileReadStream(ctx, server, &v2pb.FileReadRequest{
			Path: "/platypus/does/not/exist",
		})
	}()

	var hdr v2pb.FileReadResponse
	if err := link.ReadFrame(client, &hdr); err != nil {
		t.Fatalf("read header: %v", err)
	}
	if hdr.Error == "" {
		t.Fatal("expected non-empty Error for missing file")
	}
	// After the error header the handler closes the stream; attempting
	// another ReadFrame should return EOF-shaped.
	var ch v2pb.FileChunk
	err := link.ReadFrame(client, &ch)
	if err == nil {
		// Pass &ch (not ch) so we don't copy the protoimpl.MessageState
		// inside FileChunk — it carries a sync.Mutex that go vet
		// rightly forbids us from copying. The %+v formatter walks
		// pointer fields the same way as a value, so the diagnostic
		// stays useful.
		t.Fatalf("expected error reading after close; got chunk %+v", &ch)
	}
	if err != io.EOF && err != io.ErrClosedPipe && err != io.ErrUnexpectedEOF {
		// Any EOF-shaped error is fine.
		if !isPipeClosed(err) {
			t.Fatalf("unexpected post-close error: %v", err)
		}
	}

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("handler did not return")
	}
}

// Offset + length bounds are honored.
func TestHandleFileReadStream_OffsetLength(t *testing.T) {
	dir := t.TempDir()
	all := []byte("0123456789abcdefghij")
	path := filepath.Join(dir, "src")
	if err := os.WriteFile(path, all, 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	client, server := pairedProcessStreams(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- HandleFileReadStream(ctx, server, &v2pb.FileReadRequest{
			Path:   path,
			Offset: 5,
			Length: 10,
		})
	}()

	var hdr v2pb.FileReadResponse
	if err := link.ReadFrame(client, &hdr); err != nil {
		t.Fatalf("read header: %v", err)
	}
	var got []byte
	for {
		var ch v2pb.FileChunk
		if err := link.ReadFrame(client, &ch); err != nil {
			t.Fatalf("read chunk: %v", err)
		}
		got = append(got, ch.Data...)
		if ch.Eof {
			break
		}
	}
	want := all[5:15]
	if !bytes.Equal(got, want) {
		t.Fatalf("got %q; want %q", got, want)
	}
	_ = net.Conn(nil) // silence unused-import if any
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("handler did not return")
	}
}

func isPipeClosed(err error) bool {
	if err == nil {
		return false
	}
	return err == io.EOF ||
		err == io.ErrClosedPipe ||
		err == io.ErrUnexpectedEOF ||
		contains(err.Error(), "closed") ||
		contains(err.Error(), "EOF")
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
