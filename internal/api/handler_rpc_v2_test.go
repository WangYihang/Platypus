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

	"github.com/WangYihang/Platypus/internal/activity"
	"github.com/WangYihang/Platypus/internal/core"
	"github.com/WangYihang/Platypus/internal/link"
	"github.com/WangYihang/Platypus/internal/storage"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// v2 one-shot RPCs routed through CallAgentRPC (project-scoped):
//   GET    /api/v1/projects/:pid/agents/:agent_id/fs/list?path=...
//   GET    /api/v1/projects/:pid/agents/:agent_id/fs/stat?path=...
//   DELETE /api/v1/projects/:pid/agents/:agent_id/fs/remove?path=...&recursive=true
//   POST   /api/v1/projects/:pid/agents/:agent_id/fs/rename?from=...&to=...
//   POST   /api/v1/projects/:pid/agents/:agent_id/fs/mkdir?path=...&mkdirs=true
//   PATCH  /api/v1/projects/:pid/agents/:agent_id/fs/mode?path=...&mode=...
//   GET    /api/v1/projects/:pid/agents/:agent_id/sys
//   POST   /api/v1/projects/:pid/agents/:agent_id/exec  (JSON body)

// rpcTestEnv bundles the live test server + the auth fixture so tests
// can build authorized requests against the project-scoped routes
// without re-stating the URL prefix and Bearer token in every block.
type rpcTestEnv struct {
	srv     *httptest.Server
	fixture *agentRouteFixture
}

// setupRPCAgent registers a stub agent that echoes whatever RPC
// payload it receives back with a canned response, then mounts the
// project-scoped agent RPC routes on a fresh gin engine. The returned
// env carries the URL prefix + admin Bearer token so each test can
// build requests with newRPCRequest.
func setupRPCAgent(t *testing.T, agentID string, handler func(*v2pb.RpcRequest) *v2pb.RpcResponse) *rpcTestEnv {
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
	RegisterV2AgentRPCRoutes(r, svc, fixture.RBAC)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return &rpcTestEnv{srv: srv, fixture: fixture}
}

// newRPCRequest builds an authorized request against env.srv at the
// project-scoped path env.fixture.URL(suffix).
func (e *rpcTestEnv) newRPCRequest(t *testing.T, method, suffix string, body io.Reader) *http.Request {
	t.Helper()
	req, err := http.NewRequest(method, e.srv.URL+e.fixture.URL(suffix), body)
	if err != nil {
		t.Fatalf("http.NewRequest: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+e.fixture.Token)
	return req
}

// do is a convenience: build the request and run it on http.DefaultClient.
func (e *rpcTestEnv) do(t *testing.T, method, suffix string, body io.Reader) *http.Response {
	t.Helper()
	req := e.newRPCRequest(t, method, suffix, body)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	return resp
}

func TestRPCv2_ListDir(t *testing.T) {
	env := setupRPCAgent(t, "a1", func(req *v2pb.RpcRequest) *v2pb.RpcResponse {
		if req.GetListDir() == nil {
			return &v2pb.RpcResponse{Error: "expected list_dir"}
		}
		return &v2pb.RpcResponse{Payload: &v2pb.RpcResponse_ListDir{
			ListDir: &v2pb.ListDirResponse{Entries: []*v2pb.FileEntry{
				{Name: "a", Size: 1}, {Name: "b", Size: 2},
			}},
		}}
	})

	resp := env.do(t, http.MethodGet, "/fs/list?path=/tmp", nil)
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
	env := setupRPCAgent(t, "a1", func(req *v2pb.RpcRequest) *v2pb.RpcResponse {
		return &v2pb.RpcResponse{Payload: &v2pb.RpcResponse_Stat{
			Stat: &v2pb.StatResponse{Entry: &v2pb.FileEntry{Name: "f", Size: 99}},
		}}
	})
	resp := env.do(t, http.MethodGet, "/fs/stat?path=/f", nil)
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
	env := setupRPCAgent(t, "a1", func(req *v2pb.RpcRequest) *v2pb.RpcResponse {
		got = req.GetDelete()
		return &v2pb.RpcResponse{Payload: &v2pb.RpcResponse_Delete{
			Delete: &v2pb.DeleteResponse{},
		}}
	})
	resp := env.do(t, http.MethodDelete, "/fs/remove?path=/tree&recursive=true", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if got == nil || got.Path != "/tree" || !got.Recursive {
		t.Fatalf("delete req = %+v", got)
	}
}

func TestRPCv2_Rename(t *testing.T) {
	var got *v2pb.RenameRequest
	env := setupRPCAgent(t, "a1", func(req *v2pb.RpcRequest) *v2pb.RpcResponse {
		got = req.GetRename()
		return &v2pb.RpcResponse{Payload: &v2pb.RpcResponse_Rename{
			Rename: &v2pb.RenameResponse{},
		}}
	})
	resp := env.do(t, http.MethodPost, "/fs/rename?from=/a&to=/b", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if got.From != "/a" || got.To != "/b" {
		t.Fatalf("rename = %+v", got)
	}
}

func TestRPCv2_Mkdir(t *testing.T) {
	env := setupRPCAgent(t, "a1", func(req *v2pb.RpcRequest) *v2pb.RpcResponse {
		return &v2pb.RpcResponse{Payload: &v2pb.RpcResponse_Mkdir{
			Mkdir: &v2pb.MkdirResponse{},
		}}
	})
	resp := env.do(t, http.MethodPost, "/fs/mkdir?path=/d&mkdirs=true&mode=493", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestRPCv2_Chmod(t *testing.T) {
	var got *v2pb.ChmodRequest
	env := setupRPCAgent(t, "a1", func(req *v2pb.RpcRequest) *v2pb.RpcResponse {
		got = req.GetChmod()
		return &v2pb.RpcResponse{Payload: &v2pb.RpcResponse_Chmod{
			Chmod: &v2pb.ChmodResponse{},
		}}
	})
	resp := env.do(t, http.MethodPatch, "/fs/mode?path=/f&mode=384", nil) // 0600
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if got.Mode != 384 {
		t.Fatalf("mode = %d; want 384", got.Mode)
	}
}

func TestRPCv2_SysInfo(t *testing.T) {
	env := setupRPCAgent(t, "a1", func(req *v2pb.RpcRequest) *v2pb.RpcResponse {
		return &v2pb.RpcResponse{Payload: &v2pb.RpcResponse_SysInfo{
			SysInfo: &v2pb.SysInfoResponse{Os: "linux", Arch: "amd64", Hostname: "h"},
		}}
	})
	resp := env.do(t, http.MethodGet, "/sys", nil)
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
	env := setupRPCAgent(t, "a1", func(req *v2pb.RpcRequest) *v2pb.RpcResponse {
		got = req.GetExec()
		return &v2pb.RpcResponse{Payload: &v2pb.RpcResponse_Exec{
			Exec: &v2pb.ExecResponse{Stdout: []byte("hi"), ExitCode: 0},
		}}
	})
	body := bytes.NewReader([]byte(`{"command":"uname","args":["-a"]}`))
	resp := env.do(t, http.MethodPost, "/exec", body)
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

// TestRPCv2_AuditRowsRecorded covers the audit side-effect of the
// RPC handlers: every successful call lands one activity row with
// the expected category/action and a meta payload that mirrors the
// request parameters. Guards against drift where someone adds a new
// RPC method but forgets to register it in rpcAuditTable.
func TestRPCv2_AuditRowsRecorded(t *testing.T) {
	env := setupRPCAgent(t, "audit-agent", func(req *v2pb.RpcRequest) *v2pb.RpcResponse {
		switch {
		case req.GetDelete() != nil:
			return &v2pb.RpcResponse{Payload: &v2pb.RpcResponse_Delete{Delete: &v2pb.DeleteResponse{}}}
		case req.GetExec() != nil:
			return &v2pb.RpcResponse{Payload: &v2pb.RpcResponse_Exec{Exec: &v2pb.ExecResponse{ExitCode: 0}}}
		}
		return &v2pb.RpcResponse{Error: "unexpected"}
	})

	rec := activity.New(env.fixture.DB)
	activity.SetRecorder(rec)
	t.Cleanup(func() {
		rec.Close()
		activity.SetRecorder(nil)
	})

	resp := env.do(t, http.MethodDelete, "/fs/remove?path=/etc/hosts&recursive=true", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete status = %d", resp.StatusCode)
	}
	resp = env.do(t, http.MethodPost, "/exec", bytes.NewReader([]byte(`{"command":"id"}`)))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("exec status = %d", resp.StatusCode)
	}

	rows := waitForActivities(t, env.fixture.DB, 2)

	var sawDelete, sawExec bool
	for _, a := range rows {
		switch a.Action {
		case "file.delete":
			sawDelete = true
			if a.TargetID != "audit-agent" || a.Category != storage.CategoryFile {
				t.Errorf("delete row: %+v", a)
			}
			if !strings.Contains(a.Meta, `"path":"/etc/hosts"`) || !strings.Contains(a.Meta, `"recursive":true`) {
				t.Errorf("delete meta missing fields: %s", a.Meta)
			}
		case "command.exec":
			sawExec = true
			if a.Category != storage.CategoryCommand {
				t.Errorf("exec category = %s", a.Category)
			}
			if !strings.Contains(a.Meta, `"command":"id"`) {
				t.Errorf("exec meta missing command: %s", a.Meta)
			}
			if !strings.Contains(a.Meta, `"exit_code":0`) {
				t.Errorf("exec meta missing exit_code: %s", a.Meta)
			}
		}
	}
	if !sawDelete || !sawExec {
		t.Fatalf("missing audit rows: delete=%v exec=%v rows=%v", sawDelete, sawExec, rows)
	}
}

// waitForActivities polls db.Activities().List until at least n rows
// are present, or the deadline expires. Mirrors the helper used by
// other audit-aware tests in this package.
func waitForActivities(t *testing.T, db *storage.DB, n int) []*storage.Activity {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		rows, _, err := db.Activities().List(context.Background(), storage.ActivityFilter{})
		if err != nil {
			t.Fatalf("Activities.List: %v", err)
		}
		if len(rows) >= n {
			return rows
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %d activity rows; have %d", n, len(rows))
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// AgentNotConnected exercises the path where the agent_id passes the
// host-row check (RequireAgentInProject finds it) but the
// AgentLinkService doesn't have a live session for it. Expect 404
// from callOrAbort, not 403/401.
func TestRPCv2_AgentNotConnected(t *testing.T) {
	fixture := newAgentRouteFixture(t, "nope")
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterV2AgentRPCRoutes(r, core.NewAgentLinkService(), fixture.RBAC)
	srv := httptest.NewServer(r)
	defer srv.Close()
	req, _ := http.NewRequest(http.MethodGet, srv.URL+fixture.URL("/sys"), nil)
	req.Header.Set("Authorization", "Bearer "+fixture.Token)
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != 404 {
		t.Fatalf("status = %d; want 404", resp.StatusCode)
	}
	_ = time.Second
}
