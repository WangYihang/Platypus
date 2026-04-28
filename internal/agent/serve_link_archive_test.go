package agent

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/WangYihang/Platypus/internal/link"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

func protoMarshal(t *testing.T, m proto.Message) ([]byte, error) {
	t.Helper()
	b, err := proto.Marshal(m)
	if err != nil {
		t.Fatalf("proto.Marshal: %v", err)
	}
	return b, nil
}

// Dispatcher test: opening a STREAM_TYPE_FILE_SCAN stream routes
// to deps.FileScan and the response round-trips to the client.
func TestServeLink_DispatchesFileScan(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a"), []byte("xxxx"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	clientSess, agentSess := pairedAgentSessions(t)
	deps := AgentHandlerDeps{
		FileScan: HandleFileScanStream,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = ServeLink(ctx, agentSess, deps) }()

	meta, _ := protoMarshal(t, &v2pb.FileScanRequest{Paths: []string{dir}})
	stream, err := clientSess.Open(v2pb.StreamType_STREAM_TYPE_FILE_SCAN, meta, "test-scan")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer stream.Close()

	deadline, dcancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer dcancel()
	respCh := make(chan *v2pb.FileScanResponse, 1)
	errCh := make(chan error, 1)
	go func() {
		var resp v2pb.FileScanResponse
		if err := link.ReadFrame(stream, &resp); err != nil {
			errCh <- err
			return
		}
		respCh <- &resp
	}()
	select {
	case resp := <-respCh:
		if resp.FileCount != 1 || resp.TotalBytes != 4 {
			t.Errorf("got %+v; want FileCount=1 TotalBytes=4", resp)
		}
	case err := <-errCh:
		t.Fatalf("read response: %v", err)
	case <-deadline.Done():
		t.Fatal("timeout waiting for FileScanResponse")
	}
}

// Dispatcher test: opening STREAM_TYPE_FILE_SCAN with no handler
// registered returns a StreamReject.
func TestServeLink_RejectsFileScanWhenUnregistered(t *testing.T) {
	clientSess, agentSess := pairedAgentSessions(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = ServeLink(ctx, agentSess, AgentHandlerDeps{}) }()

	meta, _ := protoMarshal(t, &v2pb.FileScanRequest{Paths: []string{"/tmp"}})
	stream, err := clientSess.Open(v2pb.StreamType_STREAM_TYPE_FILE_SCAN, meta, "test-scan-unreg")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer stream.Close()

	var rej v2pb.StreamReject
	if err := link.ReadFrame(stream, &rej); err != nil {
		t.Fatalf("expected reject; got error %v", err)
	}
	if rej.Code != "unsupported_type" {
		t.Errorf("Code = %q; want unsupported_type", rej.Code)
	}
}

// Dispatcher test: STREAM_TYPE_FILE_ARCHIVE routes to deps.FileArchive.
func TestServeLink_DispatchesFileArchive(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	clientSess, agentSess := pairedAgentSessions(t)
	deps := AgentHandlerDeps{
		FileArchive: HandleFileArchiveStream,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = ServeLink(ctx, agentSess, deps) }()

	meta, _ := protoMarshal(t, &v2pb.FileArchiveRequest{
		Paths:  []string{dir},
		Format: v2pb.ArchiveFormat_ARCHIVE_FORMAT_TAR,
	})
	stream, err := clientSess.Open(v2pb.StreamType_STREAM_TYPE_FILE_ARCHIVE, meta, "test-arch")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer stream.Close()

	var hdr v2pb.FileArchiveResponse
	if err := link.ReadFrame(stream, &hdr); err != nil {
		t.Fatalf("read header: %v", err)
	}
	if hdr.Error != "" {
		t.Fatalf("header error: %s", hdr.Error)
	}
	// Drain until eof.
	for {
		var ch v2pb.FileChunk
		if err := link.ReadFrame(stream, &ch); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrClosedPipe) {
				break
			}
			t.Fatalf("read chunk: %v", err)
		}
		if ch.Eof {
			if ch.Error != "" {
				t.Fatalf("final chunk error: %s", ch.Error)
			}
			break
		}
	}
}

// Dispatcher test: STREAM_TYPE_FILE_ARCHIVE with no handler →
// StreamReject.
func TestServeLink_RejectsFileArchiveWhenUnregistered(t *testing.T) {
	clientSess, agentSess := pairedAgentSessions(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = ServeLink(ctx, agentSess, AgentHandlerDeps{}) }()

	meta, _ := protoMarshal(t, &v2pb.FileArchiveRequest{Paths: []string{"/tmp"}})
	stream, err := clientSess.Open(v2pb.StreamType_STREAM_TYPE_FILE_ARCHIVE, meta, "test-arch-unreg")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer stream.Close()

	var rej v2pb.StreamReject
	if err := link.ReadFrame(stream, &rej); err != nil {
		t.Fatalf("expected reject; got error %v", err)
	}
	if rej.Code != "unsupported_type" {
		t.Errorf("Code = %q; want unsupported_type", rej.Code)
	}
}
