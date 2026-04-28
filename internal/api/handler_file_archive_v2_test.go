package api

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"google.golang.org/protobuf/proto"

	"github.com/WangYihang/Platypus/internal/core"
	"github.com/WangYihang/Platypus/internal/link"
	"github.com/WangYihang/Platypus/internal/storage"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// archiveTestAgent registers a Session under agentID and answers
// STREAM_TYPE_FILE_SCAN + STREAM_TYPE_FILE_ARCHIVE streams via the
// supplied callbacks. Mirrors fileTestAgent but for the new
// archive-flavored streams.
type archiveTestAgent struct {
	svc      *core.AgentLinkService
	peer     *link.Session
	fixture  *agentRouteFixture
	cancels  *TransferCancelRegistry
	recorder *FakeTransferRecorder
}

// FakeTransferRecorder swaps in for the production recorder during
// tests, capturing all transfer state transitions in memory so
// assertions can inspect the timeline directly.
type FakeTransferRecorder struct {
	mu      sync.Mutex
	created []*storage.FileTransfer
	updates []TransferProgressUpdate
	finals  []TransferFinalUpdate
}

type TransferProgressUpdate struct {
	ID    string
	Bytes int64
	Total int64
}

type TransferFinalUpdate struct {
	ID     string
	Status string
	Bytes  int64
	Error  string
}

func (f *FakeTransferRecorder) Create(_ context.Context, ft *storage.FileTransfer) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	// Snapshot the row at creation time so test assertions about
	// initial state aren't fooled by later mutations on the same
	// pointer (the production handler reuses the struct as the
	// "live" transfer state).
	cp := *ft
	f.created = append(f.created, &cp)
	return nil
}

func (f *FakeTransferRecorder) UpdateProgress(_ context.Context, id string, bytes, total int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.updates = append(f.updates, TransferProgressUpdate{ID: id, Bytes: bytes, Total: total})
	return nil
}

func (f *FakeTransferRecorder) Finish(_ context.Context, id, status string, bytes int64, errMsg string, _ time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.finals = append(f.finals, TransferFinalUpdate{ID: id, Status: status, Bytes: bytes, Error: errMsg})
	return nil
}

func setupArchiveAgent(t *testing.T, agentID string,
	onScan func(*v2pb.FileScanRequest, io.ReadWriteCloser),
	onArchive func(*v2pb.FileArchiveRequest, io.ReadWriteCloser),
) *archiveTestAgent {
	t.Helper()
	fixture := newAgentRouteFixture(t, agentID)

	svc := core.NewAgentLinkService()
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
	agentSess, err := link.NewClientSession(clientConn)
	if err != nil {
		t.Fatalf("client session: %v", err)
	}
	peer := <-serverCh
	svc.Register(agentID, agentSess)

	go func() {
		for {
			hdr, stream, err := peer.Accept()
			if err != nil {
				return
			}
			switch hdr.Type {
			case v2pb.StreamType_STREAM_TYPE_FILE_SCAN:
				if onScan == nil {
					_ = stream.Close()
					continue
				}
				var req v2pb.FileScanRequest
				_ = proto.Unmarshal(hdr.Metadata, &req)
				go onScan(&req, stream)
			case v2pb.StreamType_STREAM_TYPE_FILE_ARCHIVE:
				if onArchive == nil {
					_ = stream.Close()
					continue
				}
				var req v2pb.FileArchiveRequest
				_ = proto.Unmarshal(hdr.Metadata, &req)
				go onArchive(&req, stream)
			default:
				_ = stream.Close()
			}
		}
	}()

	t.Cleanup(func() {
		agentSess.Close()
		peer.Close()
	})

	return &archiveTestAgent{
		svc:      svc,
		peer:     peer,
		fixture:  fixture,
		cancels:  NewTransferCancelRegistry(),
		recorder: &FakeTransferRecorder{},
	}
}

func (a *archiveTestAgent) registerArchiveRoutes(r *gin.Engine) {
	RegisterV2FileArchiveRoutes(r, FileArchiveDeps{
		Service:    a.svc,
		RBAC:       a.fixture.RBAC,
		Recorder:   a.recorder,
		Broadcaster: nil,
		Cancels:    a.cancels,
		IDGenerator: func() string { return "ft-test-1" },
	})
}

func (a *archiveTestAgent) authedPost(t *testing.T, srvURL, suffix string, body any) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req, err := http.NewRequest(http.MethodPost, srvURL+a.fixture.URL(suffix), &buf)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.fixture.Token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	return resp
}

// Scan endpoint round-trips and returns counts.
func TestFileV2_ScanHappy(t *testing.T) {
	a := setupArchiveAgent(t, "agent-scan",
		func(req *v2pb.FileScanRequest, stream io.ReadWriteCloser) {
			defer stream.Close()
			_ = link.WriteFrame(stream, &v2pb.FileScanResponse{
				FileCount:  3,
				DirCount:   2,
				TotalBytes: 9001,
			})
		},
		nil,
	)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	a.registerArchiveRoutes(r)
	srv := httptest.NewServer(r)
	defer srv.Close()

	resp := a.authedPost(t, srv.URL, "/fs/scan", map[string]any{
		"paths": []string{"/etc"},
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d; want 200; body=%s", resp.StatusCode, b)
	}
	var got struct {
		FileCount  int64 `json:"file_count"`
		DirCount   int64 `json:"dir_count"`
		TotalBytes int64 `json:"total_bytes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.FileCount != 3 || got.DirCount != 2 || got.TotalBytes != 9001 {
		t.Errorf("got %+v", got)
	}
}

// Archive endpoint streams agent-produced bytes back to the client
// AND records a file_transfers row.
func TestFileV2_ArchiveHappy(t *testing.T) {
	// Build a tiny tar payload so we can verify it round-trips
	// through the server.
	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)
	hdr := &tar.Header{Name: "x.txt", Size: 5, Mode: 0o644}
	_ = tw.WriteHeader(hdr)
	_, _ = tw.Write([]byte("hello"))
	_ = tw.Close()
	want := tarBuf.Bytes()

	a := setupArchiveAgent(t, "agent-arch",
		func(req *v2pb.FileScanRequest, stream io.ReadWriteCloser) {
			defer stream.Close()
			_ = link.WriteFrame(stream, &v2pb.FileScanResponse{
				FileCount: 1, TotalBytes: int64(len(want)),
			})
		},
		func(req *v2pb.FileArchiveRequest, stream io.ReadWriteCloser) {
			defer stream.Close()
			_ = link.WriteFrame(stream, &v2pb.FileArchiveResponse{})
			// Send in two chunks to exercise the loop.
			mid := len(want) / 2
			_ = link.WriteFrame(stream, &v2pb.FileChunk{Data: want[:mid]})
			_ = link.WriteFrame(stream, &v2pb.FileChunk{Data: want[mid:], Eof: true})
		},
	)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	a.registerArchiveRoutes(r)
	srv := httptest.NewServer(r)
	defer srv.Close()

	resp := a.authedPost(t, srv.URL, "/fs/archive", map[string]any{
		"paths":  []string{"/etc"},
		"format": "tar",
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d; want 200; body=%s", resp.StatusCode, b)
	}
	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("body mismatch: got %d bytes, want %d", len(got), len(want))
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "application/x-tar") {
		t.Errorf("Content-Type = %q; want application/x-tar", ct)
	}
	if disp := resp.Header.Get("Content-Disposition"); !strings.Contains(disp, ".tar") {
		t.Errorf("Content-Disposition = %q; want filename ending .tar", disp)
	}
	if id := resp.Header.Get("X-Transfer-Id"); id != "ft-test-1" {
		t.Errorf("X-Transfer-Id = %q; want ft-test-1", id)
	}
	if total := resp.Header.Get("X-Total-Bytes"); total == "" {
		t.Errorf("X-Total-Bytes header missing")
	}

	// Verify recorder calls happened: 1 Create, ≥1 progress, 1 Finish.
	if len(a.recorder.created) != 1 {
		t.Fatalf("recorder.Create called %d times; want 1", len(a.recorder.created))
	}
	if a.recorder.created[0].Status != storage.TransferStatusRunning {
		t.Errorf("created status = %q; want running", a.recorder.created[0].Status)
	}
	if len(a.recorder.finals) != 1 {
		t.Fatalf("recorder.Finish called %d times; want 1", len(a.recorder.finals))
	}
	if a.recorder.finals[0].Status != storage.TransferStatusDone {
		t.Errorf("final status = %q; want done", a.recorder.finals[0].Status)
	}
	if a.recorder.finals[0].Bytes != int64(len(want)) {
		t.Errorf("final bytes = %d; want %d", a.recorder.finals[0].Bytes, len(want))
	}
}

// Agent error during archive build → final transfer status=failed.
func TestFileV2_ArchiveAgentError(t *testing.T) {
	a := setupArchiveAgent(t, "agent-arch-err",
		func(req *v2pb.FileScanRequest, stream io.ReadWriteCloser) {
			defer stream.Close()
			_ = link.WriteFrame(stream, &v2pb.FileScanResponse{
				FileCount: 0, TotalBytes: 0,
			})
		},
		func(req *v2pb.FileArchiveRequest, stream io.ReadWriteCloser) {
			defer stream.Close()
			_ = link.WriteFrame(stream, &v2pb.FileArchiveResponse{Error: "permission denied"})
		},
	)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	a.registerArchiveRoutes(r)
	srv := httptest.NewServer(r)
	defer srv.Close()

	resp := a.authedPost(t, srv.URL, "/fs/archive", map[string]any{
		"paths":  []string{"/root/secret"},
		"format": "tar",
	})
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Fatalf("expected non-200 on agent error")
	}
	if len(a.recorder.finals) != 1 ||
		a.recorder.finals[0].Status != storage.TransferStatusFailed {
		t.Fatalf("finals = %+v", a.recorder.finals)
	}
}

// Cancellation: the cancel registry's Cancel(transferID) closes the
// in-flight HTTP request and finalizes the transfer as canceled.
func TestFileV2_ArchiveCancel(t *testing.T) {
	dataReady := make(chan struct{})
	a := setupArchiveAgent(t, "agent-cancel",
		func(req *v2pb.FileScanRequest, stream io.ReadWriteCloser) {
			defer stream.Close()
			_ = link.WriteFrame(stream, &v2pb.FileScanResponse{
				FileCount: 1, TotalBytes: 1024 * 1024,
			})
		},
		func(req *v2pb.FileArchiveRequest, stream io.ReadWriteCloser) {
			defer stream.Close()
			_ = link.WriteFrame(stream, &v2pb.FileArchiveResponse{})
			data := bytes.Repeat([]byte("X"), 4096)
			// Send chunks until WriteFrame errors (server tore the
			// stream down) or the agent runs out of data.
			for i := 0; i < 500; i++ {
				if err := link.WriteFrame(stream, &v2pb.FileChunk{Data: data}); err != nil {
					return
				}
				if i == 0 {
					close(dataReady)
				}
				time.Sleep(10 * time.Millisecond)
			}
			_ = link.WriteFrame(stream, &v2pb.FileChunk{Eof: true})
		},
	)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	a.registerArchiveRoutes(r)
	srv := httptest.NewServer(r)
	defer srv.Close()

	respCh := make(chan struct{})
	go func() {
		defer close(respCh)
		var buf bytes.Buffer
		_ = json.NewEncoder(&buf).Encode(map[string]any{
			"paths": []string{"/big"}, "format": "tar",
		})
		req, _ := http.NewRequest(http.MethodPost,
			srv.URL+a.fixture.URL("/fs/archive"), &buf)
		req.Header.Set("Authorization", "Bearer "+a.fixture.Token)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	// Wait for the agent to start writing data so we know the
	// transfer was registered in the cancel registry.
	select {
	case <-dataReady:
	case <-time.After(3 * time.Second):
		t.Fatal("agent did not start writing data")
	}
	if !a.cancels.Cancel("ft-test-1") {
		t.Fatal("Cancel returned false; transfer not registered")
	}

	select {
	case <-respCh:
	case <-time.After(5 * time.Second):
		t.Fatal("HTTP response did not return after cancellation")
	}
	// Final status should be canceled.
	a.recorder.mu.Lock()
	finals := append([]TransferFinalUpdate(nil), a.recorder.finals...)
	a.recorder.mu.Unlock()
	if len(finals) != 1 || finals[0].Status != storage.TransferStatusCanceled {
		t.Fatalf("finals = %+v; want one canceled entry", finals)
	}
}

