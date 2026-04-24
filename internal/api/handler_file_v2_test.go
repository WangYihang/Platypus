package api

import (
	"bytes"
	"context"
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
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// v2 file endpoints:
//   GET  /api/v1/agents/:agent_id/fs/read?path=...   — download
//   PUT  /api/v1/agents/:agent_id/fs/write?path=...  — upload
//
// Both look up the agent in AgentLinkService and open the matching
// STREAM_TYPE_FILE_* stream on its link.Session.

// fileTestAgent registers a Session under agentID that spawns a
// handler per stream type. onRead / onWrite get the stream + parsed
// request from the header; they're responsible for writing the
// response frames.
type fileTestAgent struct {
	svc  *core.AgentLinkService
	peer *link.Session
}

func setupFileAgent(t *testing.T, agentID string,
	onRead func(*v2pb.FileReadRequest, io.ReadWriteCloser),
	onWrite func(*v2pb.FileWriteRequest, io.ReadWriteCloser),
) *fileTestAgent {
	t.Helper()
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
			case v2pb.StreamType_STREAM_TYPE_FILE_READ:
				var req v2pb.FileReadRequest
				_ = proto.Unmarshal(hdr.Metadata, &req)
				go onRead(&req, stream)
			case v2pb.StreamType_STREAM_TYPE_FILE_WRITE:
				var req v2pb.FileWriteRequest
				_ = proto.Unmarshal(hdr.Metadata, &req)
				go onWrite(&req, stream)
			default:
				_ = stream.Close()
			}
		}
	}()

	t.Cleanup(func() {
		agentSess.Close()
		peer.Close()
	})
	return &fileTestAgent{svc: svc, peer: peer}
}

func TestFileV2_DownloadHappy(t *testing.T) {
	want := bytes.Repeat([]byte("AB"), 400) // 800 bytes
	a := setupFileAgent(t, "agent-dl",
		func(req *v2pb.FileReadRequest, stream io.ReadWriteCloser) {
			defer stream.Close()
			_ = link.WriteFrame(stream, &v2pb.FileReadResponse{
				Size: int64(len(want)), Mode: 0o644,
			})
			_ = link.WriteFrame(stream, &v2pb.FileChunk{Data: want, Eof: true})
		},
		nil,
	)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterV2FileRoutes(r, a.svc)
	srv := httptest.NewServer(r)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/agents/agent-dl/fs/read?path=/whatever")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; want 200", resp.StatusCode)
	}
	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("body mismatch: got %d bytes, want %d", len(got), len(want))
	}
}

func TestFileV2_DownloadAgentError(t *testing.T) {
	a := setupFileAgent(t, "agent-missing",
		func(req *v2pb.FileReadRequest, stream io.ReadWriteCloser) {
			defer stream.Close()
			_ = link.WriteFrame(stream, &v2pb.FileReadResponse{
				Error: "not found",
			})
		},
		nil,
	)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterV2FileRoutes(r, a.svc)
	srv := httptest.NewServer(r)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/agents/agent-missing/fs/read?path=/nope")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer resp.Body.Close()
	// Server should surface a 502-ish status since the agent said no.
	if resp.StatusCode == http.StatusOK {
		t.Fatalf("expected non-2xx, got 200")
	}
	b, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(b), "not found") {
		t.Fatalf("body = %q; want to mention agent error", b)
	}
}

func TestFileV2_UploadHappy(t *testing.T) {
	payload := bytes.Repeat([]byte("X"), 1500)
	received := make(chan []byte, 1)

	a := setupFileAgent(t, "agent-up", nil,
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
	RegisterV2FileRoutes(r, a.svc)
	srv := httptest.NewServer(r)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodPut,
		srv.URL+"/api/v1/agents/agent-up/fs/write?path=/dest", bytes.NewReader(payload))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d; body=%s", resp.StatusCode, b)
	}

	select {
	case got := <-received:
		if !bytes.Equal(got, payload) {
			t.Fatalf("agent saw %d bytes; want %d", len(got), len(payload))
		}
	case <-ctx.Done():
		t.Fatal("agent handler timed out")
	}
}

func TestFileV2_UnknownAgent404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterV2FileRoutes(r, core.NewAgentLinkService())
	srv := httptest.NewServer(r)
	defer srv.Close()

	resp, _ := http.Get(srv.URL + "/api/v1/agents/missing/fs/read?path=/x")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d; want 404", resp.StatusCode)
	}
}
