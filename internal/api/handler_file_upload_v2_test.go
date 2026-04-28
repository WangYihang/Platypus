package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"google.golang.org/protobuf/proto"

	"github.com/WangYihang/Platypus/internal/core"
	"github.com/WangYihang/Platypus/internal/link"
	"github.com/WangYihang/Platypus/internal/storage"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// /api/v1/projects/:pid/agents/:agent_id/fs/upload streams a request
// body to the agent as a single STREAM_TYPE_FILE_WRITE, AND records
// the operation as a file_transfers row (direction=upload) so the UI
// shows it in the transfers tab.
//
// Wire shape mirrors the existing /fs/write but adds:
//   * X-Transfer-Id response header
//   * total_bytes query param so the server can size progress before
//     reading the body
//   * cancel registry registration so an in-flight upload can be
//     aborted via /transfers/:id/cancel

// uploadTestAgent registers a paired Session and answers
// STREAM_TYPE_FILE_WRITE streams via the supplied callback.
type uploadTestAgent struct {
	svc      *core.AgentLinkService
	peer     *link.Session
	fixture  *agentRouteFixture
	cancels  *TransferCancelRegistry
	recorder *FakeTransferRecorder
	hosts    *fakeHostLookup // nil unless a test seeds host rows
}

func setupUploadAgent(
	t *testing.T,
	agentID string,
	onWrite func(*v2pb.FileWriteRequest, io.ReadWriteCloser),
) *uploadTestAgent {
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
			if hdr.Type == v2pb.StreamType_STREAM_TYPE_FILE_WRITE {
				var req v2pb.FileWriteRequest
				_ = proto.Unmarshal(hdr.Metadata, &req)
				go onWrite(&req, stream)
			} else {
				_ = stream.Close()
			}
		}
	}()
	t.Cleanup(func() {
		agentSess.Close()
		peer.Close()
	})
	return &uploadTestAgent{
		svc:      svc,
		peer:     peer,
		fixture:  fixture,
		cancels:  NewTransferCancelRegistry(),
		recorder: &FakeTransferRecorder{},
	}
}

func (a *uploadTestAgent) registerRoutes(r *gin.Engine) {
	deps := FileArchiveDeps{
		Service:     a.svc,
		RBAC:        a.fixture.RBAC,
		Recorder:    a.recorder,
		Broadcaster: nil,
		Cancels:     a.cancels,
		IDGenerator: func() string { return "ft-up-1" },
	}
	if a.hosts != nil {
		deps.Hosts = a.hosts
	}
	RegisterV2FileArchiveRoutes(r, deps)
}

func (a *uploadTestAgent) authedPut(t *testing.T, srvURL, suffix string, body []byte) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPut, srvURL+a.fixture.URL(suffix), bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+a.fixture.Token)
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	return resp
}

func TestFileV2_UploadEndpointHappy(t *testing.T) {
	payload := bytes.Repeat([]byte("Q"), 2048)
	received := make(chan []byte, 1)

	a := setupUploadAgent(t, "agent-up-h",
		func(req *v2pb.FileWriteRequest, stream io.ReadWriteCloser) {
			defer stream.Close()
			_ = link.WriteFrame(stream, &v2pb.FileWriteResponse{})
			var got []byte
			for {
				var ch v2pb.FileChunk
				if err := link.ReadFrame(stream, &ch); err != nil {
					break
				}
				got = append(got, ch.Data...)
				if ch.Eof {
					break
				}
			}
			received <- got
			_ = link.WriteFrame(stream, &v2pb.FileWriteResult{
				BytesWritten: int64(len(got)),
			})
		},
	)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	a.registerRoutes(r)
	srv := httptest.NewServer(r)
	defer srv.Close()

	resp := a.authedPut(t, srv.URL,
		"/fs/upload?path=/dest/file.bin&total_bytes=2048&mkdirs=true", payload)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d; body=%s", resp.StatusCode, b)
	}

	if got := resp.Header.Get("X-Transfer-Id"); got != "ft-up-1" {
		t.Errorf("X-Transfer-Id = %q; want ft-up-1", got)
	}
	var body struct {
		BytesWritten int64  `json:"bytes_written"`
		TransferID   string `json:"transfer_id"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if body.BytesWritten != int64(len(payload)) {
		t.Errorf("BytesWritten = %d; want %d", body.BytesWritten, len(payload))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	select {
	case got := <-received:
		if !bytes.Equal(got, payload) {
			t.Fatalf("agent saw %d bytes; want %d", len(got), len(payload))
		}
	case <-ctx.Done():
		t.Fatal("agent handler timed out")
	}

	// Verify recorder calls: 1 Create (status=running), 1 Finish (done).
	if len(a.recorder.created) != 1 ||
		a.recorder.created[0].Direction != storage.TransferDirectionUpload ||
		a.recorder.created[0].Status != storage.TransferStatusRunning {
		t.Fatalf("created = %+v", a.recorder.created)
	}
	if len(a.recorder.finals) != 1 || a.recorder.finals[0].Status != storage.TransferStatusDone {
		t.Fatalf("finals = %+v", a.recorder.finals)
	}
	if a.recorder.created[0].TotalBytes != int64(len(payload)) {
		t.Errorf("TotalBytes on create = %d; want %d",
			a.recorder.created[0].TotalBytes, len(payload))
	}
}

func TestFileV2_UploadEndpointAgentError(t *testing.T) {
	a := setupUploadAgent(t, "agent-up-err",
		func(req *v2pb.FileWriteRequest, stream io.ReadWriteCloser) {
			defer stream.Close()
			_ = link.WriteFrame(stream, &v2pb.FileWriteResponse{
				Error: "permission denied",
			})
		},
	)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	a.registerRoutes(r)
	srv := httptest.NewServer(r)
	defer srv.Close()

	resp := a.authedPut(t, srv.URL,
		"/fs/upload?path=/restricted&total_bytes=4", []byte("data"))
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Fatalf("expected non-200")
	}
	if len(a.recorder.finals) != 1 ||
		a.recorder.finals[0].Status != storage.TransferStatusFailed {
		t.Fatalf("finals = %+v", a.recorder.finals)
	}
}

// Cancellation: registry's Cancel(transferID) tears down the in-flight
// upload and finalises the row as canceled.
func TestFileV2_UploadEndpointCancel(t *testing.T) {
	dataReady := make(chan struct{})
	a := setupUploadAgent(t, "agent-up-cx",
		func(req *v2pb.FileWriteRequest, stream io.ReadWriteCloser) {
			defer stream.Close()
			_ = link.WriteFrame(stream, &v2pb.FileWriteResponse{})
			// Drain whatever we can; close on error.
			gotFirst := false
			for {
				var ch v2pb.FileChunk
				if err := link.ReadFrame(stream, &ch); err != nil {
					return
				}
				if !gotFirst {
					close(dataReady)
					gotFirst = true
				}
				if ch.Eof {
					break
				}
			}
			_ = link.WriteFrame(stream, &v2pb.FileWriteResult{})
		},
	)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	a.registerRoutes(r)
	srv := httptest.NewServer(r)
	defer srv.Close()

	// Slow-feed body via an io.Pipe so we can keep the upload alive
	// until cancel fires.
	pr, pw := io.Pipe()
	defer pr.Close()
	go func() {
		defer pw.Close()
		// Write one chunk, then block forever (until pw closes).
		_, _ = pw.Write(bytes.Repeat([]byte("x"), 4096))
		<-time.After(10 * time.Second)
	}()
	respCh := make(chan *http.Response, 1)
	go func() {
		req, _ := http.NewRequest(http.MethodPut,
			srv.URL+a.fixture.URL("/fs/upload?path=/dst&total_bytes=999999"), pr)
		req.Header.Set("Authorization", "Bearer "+a.fixture.Token)
		req.Header.Set("Content-Type", "application/octet-stream")
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}
		respCh <- resp
	}()

	// Wait for first byte to land at the agent so we know the
	// upload is registered in the cancel registry.
	select {
	case <-dataReady:
	case <-time.After(3 * time.Second):
		t.Fatal("first chunk never arrived at agent")
	}
	if !a.cancels.Cancel("ft-up-1") {
		t.Fatal("Cancel returned false; transfer not registered")
	}
	select {
	case <-respCh:
	case <-time.After(5 * time.Second):
		t.Fatal("HTTP response did not return after cancel")
	}
	if len(a.recorder.finals) != 1 ||
		a.recorder.finals[0].Status != storage.TransferStatusCanceled {
		t.Fatalf("finals = %+v; want one canceled", a.recorder.finals)
	}
}

// Missing path query → 400, no transfer recorded.
// TestFileV2_UploadStoresRealHostID pins the F bug fix on the
// upload side: file_transfers.host_id must hold the host UUID, not
// the agent_id, so the per-host filter and host-name resolution on
// /transfers both work.
func TestFileV2_UploadStoresRealHostID(t *testing.T) {
	a := setupUploadAgent(t, "agent-up-host-id",
		func(_ *v2pb.FileWriteRequest, stream io.ReadWriteCloser) {
			defer stream.Close()
			_ = link.WriteFrame(stream, &v2pb.FileWriteResponse{})
			for {
				var ch v2pb.FileChunk
				if err := link.ReadFrame(stream, &ch); err != nil {
					break
				}
				if ch.Eof {
					break
				}
			}
			_ = link.WriteFrame(stream, &v2pb.FileWriteResult{BytesWritten: 4})
		},
	)
	a.hosts = &fakeHostLookup{
		byAgentID: map[string]*storage.Host{
			"agent-up-host-id": {ID: "host-uuid-upload"},
		},
	}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	a.registerRoutes(r)
	srv := httptest.NewServer(r)
	defer srv.Close()

	resp := a.authedPut(t, srv.URL, "/fs/upload?path=/dst", []byte("data"))
	defer resp.Body.Close()
	_, _ = io.ReadAll(resp.Body)

	a.recorder.mu.Lock()
	created := append([]*storage.FileTransfer(nil), a.recorder.created...)
	a.recorder.mu.Unlock()
	if len(created) != 1 {
		t.Fatalf("created = %d; want 1", len(created))
	}
	if created[0].HostID != "host-uuid-upload" {
		t.Errorf("host_id = %q; want %q", created[0].HostID, "host-uuid-upload")
	}
}

func TestFileV2_UploadEndpointMissingPath(t *testing.T) {
	a := setupUploadAgent(t, "agent-up-mp", func(_ *v2pb.FileWriteRequest, s io.ReadWriteCloser) {
		s.Close()
	})
	gin.SetMode(gin.TestMode)
	r := gin.New()
	a.registerRoutes(r)
	srv := httptest.NewServer(r)
	defer srv.Close()

	resp := a.authedPut(t, srv.URL, "/fs/upload?total_bytes=4", []byte("data"))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d; want 400; body=%s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	if len(a.recorder.created) != 0 {
		t.Fatalf("recorder.Create should not run on validation failure")
	}
}
