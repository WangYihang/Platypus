package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/core"
	"github.com/WangYihang/Platypus/internal/link"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// v2 one-shot RPCs routed through CallAgentRPC:
//   GET    /api/v1/agents/:agent_id/fs/list?path=...
//   GET    /api/v1/agents/:agent_id/fs/stat?path=...
//   DELETE /api/v1/agents/:agent_id/fs/remove?path=...&recursive=true
//   POST   /api/v1/agents/:agent_id/fs/rename?from=...&to=...
//   POST   /api/v1/agents/:agent_id/fs/mkdir?path=...&mkdirs=true
//   PATCH  /api/v1/agents/:agent_id/fs/mode?path=...&mode=...
//   GET    /api/v1/agents/:agent_id/sys
//   POST   /api/v1/agents/:agent_id/exec  (JSON body)

// setupRPCAgent registers a stub agent that echoes whatever RPC
// payload it receives back with a canned response. Returns the
// gin engine mounted with the v2 RPC routes.
func setupRPCAgent(t *testing.T, agentID string, handler func(*v2pb.RpcRequest) *v2pb.RpcResponse) *httptest.Server {
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
			if hdr.Type != v2pb.StreamType_STREAM_TYPE_RPC {
				_ = stream.Close()
				continue
			}
			go func(s io.ReadWriteCloser) {
				defer s.Close()
				var req v2pb.RpcRequest
				if err := link.ReadFrame(s, &req); err != nil {
					return
				}
				resp := handler(&req)
				_ = link.WriteFrame(s, resp)
			}(stream)
		}
	}()
	t.Cleanup(func() {
		agentSess.Close()
		peer.Close()
	})

	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterV2AgentRPCRoutes(r, svc)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv
}

func TestRPCv2_ListDir(t *testing.T) {
	srv := setupRPCAgent(t, "a1", func(req *v2pb.RpcRequest) *v2pb.RpcResponse {
		if req.GetListDir() == nil {
			return &v2pb.RpcResponse{Error: "expected list_dir"}
		}
		return &v2pb.RpcResponse{Payload: &v2pb.RpcResponse_ListDir{
			ListDir: &v2pb.ListDirResponse{Entries: []*v2pb.FileEntry{
				{Name: "a", Size: 1}, {Name: "b", Size: 2},
			}},
		}}
	})

	resp, _ := http.Get(srv.URL + "/api/v1/agents/a1/fs/list?path=/tmp")
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status %d body %s", resp.StatusCode, b)
	}
	var body struct {
		Entries []struct {
			Name string `json:"name"`
			Size int64  `json:"size"`
		} `json:"entries"`
	}
	json.NewDecoder(resp.Body).Decode(&body)
	if len(body.Entries) != 2 || body.Entries[0].Name != "a" {
		t.Fatalf("entries = %+v", body.Entries)
	}
}

func TestRPCv2_Stat(t *testing.T) {
	srv := setupRPCAgent(t, "a1", func(req *v2pb.RpcRequest) *v2pb.RpcResponse {
		return &v2pb.RpcResponse{Payload: &v2pb.RpcResponse_Stat{
			Stat: &v2pb.StatResponse{Entry: &v2pb.FileEntry{Name: "f", Size: 99}},
		}}
	})
	resp, _ := http.Get(srv.URL + "/api/v1/agents/a1/fs/stat?path=/f")
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var body struct {
		Entry struct {
			Name string `json:"name"`
			Size int64  `json:"size"`
		} `json:"entry"`
	}
	json.NewDecoder(resp.Body).Decode(&body)
	if body.Entry.Size != 99 {
		t.Fatalf("stat size = %d; want 99", body.Entry.Size)
	}
}

func TestRPCv2_Remove(t *testing.T) {
	var got *v2pb.DeleteRequest
	srv := setupRPCAgent(t, "a1", func(req *v2pb.RpcRequest) *v2pb.RpcResponse {
		got = req.GetDelete()
		return &v2pb.RpcResponse{Payload: &v2pb.RpcResponse_Delete{
			Delete: &v2pb.DeleteResponse{},
		}}
	})
	req, _ := http.NewRequest(http.MethodDelete,
		srv.URL+"/api/v1/agents/a1/fs/remove?path=/tree&recursive=true", nil)
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if got == nil || got.Path != "/tree" || !got.Recursive {
		t.Fatalf("delete req = %+v", got)
	}
}

func TestRPCv2_Rename(t *testing.T) {
	var got *v2pb.RenameRequest
	srv := setupRPCAgent(t, "a1", func(req *v2pb.RpcRequest) *v2pb.RpcResponse {
		got = req.GetRename()
		return &v2pb.RpcResponse{Payload: &v2pb.RpcResponse_Rename{
			Rename: &v2pb.RenameResponse{},
		}}
	})
	resp, _ := http.Post(srv.URL+"/api/v1/agents/a1/fs/rename?from=/a&to=/b",
		"", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if got.From != "/a" || got.To != "/b" {
		t.Fatalf("rename = %+v", got)
	}
}

func TestRPCv2_Mkdir(t *testing.T) {
	srv := setupRPCAgent(t, "a1", func(req *v2pb.RpcRequest) *v2pb.RpcResponse {
		return &v2pb.RpcResponse{Payload: &v2pb.RpcResponse_Mkdir{
			Mkdir: &v2pb.MkdirResponse{},
		}}
	})
	resp, _ := http.Post(srv.URL+"/api/v1/agents/a1/fs/mkdir?path=/d&mkdirs=true&mode=493",
		"", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestRPCv2_Chmod(t *testing.T) {
	var got *v2pb.ChmodRequest
	srv := setupRPCAgent(t, "a1", func(req *v2pb.RpcRequest) *v2pb.RpcResponse {
		got = req.GetChmod()
		return &v2pb.RpcResponse{Payload: &v2pb.RpcResponse_Chmod{
			Chmod: &v2pb.ChmodResponse{},
		}}
	})
	req, _ := http.NewRequest(http.MethodPatch,
		srv.URL+"/api/v1/agents/a1/fs/mode?path=/f&mode=384", nil) // 0600
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if got.Mode != 384 {
		t.Fatalf("mode = %d; want 384", got.Mode)
	}
}

func TestRPCv2_SysInfo(t *testing.T) {
	srv := setupRPCAgent(t, "a1", func(req *v2pb.RpcRequest) *v2pb.RpcResponse {
		return &v2pb.RpcResponse{Payload: &v2pb.RpcResponse_SysInfo{
			SysInfo: &v2pb.SysInfoResponse{Os: "linux", Arch: "amd64", Hostname: "h"},
		}}
	})
	resp, _ := http.Get(srv.URL + "/api/v1/agents/a1/sys")
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var body struct{ Os, Arch, Hostname string }
	json.NewDecoder(resp.Body).Decode(&body)
	if body.Os != "linux" || body.Hostname != "h" {
		t.Fatalf("sysinfo = %+v", body)
	}
}

func TestRPCv2_Exec(t *testing.T) {
	var got *v2pb.ExecRequest
	srv := setupRPCAgent(t, "a1", func(req *v2pb.RpcRequest) *v2pb.RpcResponse {
		got = req.GetExec()
		return &v2pb.RpcResponse{Payload: &v2pb.RpcResponse_Exec{
			Exec: &v2pb.ExecResponse{Stdout: []byte("hi"), ExitCode: 0},
		}}
	})
	body := bytes.NewReader([]byte(`{"command":"uname","args":["-a"]}`))
	resp, _ := http.Post(srv.URL+"/api/v1/agents/a1/exec",
		"application/json", body)
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d body=%s", resp.StatusCode, b)
	}
	if got.Command != "uname" || len(got.Args) != 1 || got.Args[0] != "-a" {
		t.Fatalf("exec req = %+v", got)
	}
	var res struct {
		Stdout   string `json:"stdout"`
		ExitCode int    `json:"exit_code"`
	}
	json.NewDecoder(resp.Body).Decode(&res)
	if res.Stdout != "hi" {
		t.Fatalf("stdout = %q", res.Stdout)
	}
}

func TestRPCv2_AgentNotConnected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterV2AgentRPCRoutes(r, core.NewAgentLinkService())
	srv := httptest.NewServer(r)
	defer srv.Close()
	resp, _ := http.Get(srv.URL + "/api/v1/agents/nope/sys")
	if resp.StatusCode != 404 {
		t.Fatalf("status = %d; want 404", resp.StatusCode)
	}
	_ = time.Second
}
