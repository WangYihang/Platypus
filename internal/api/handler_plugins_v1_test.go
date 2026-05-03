package api

import (
	"encoding/base64"
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
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// pluginsTestAgent is the plugin-side analogue of upgradeTestAgent.
// Same layout: bidirectional in-memory link.Session pair, agent
// goroutine reads STREAM_TYPE_PLUGIN_MGMT openings and runs the
// per-test handler closure.
type pluginsTestAgent struct {
	svc     *core.AgentLinkService
	peer    *link.Session
	fixture *agentRouteFixture
}

func setupPluginsAgent(
	t *testing.T,
	agentID string,
	onMgmt func(*v2pb.PluginMgmtRequest, io.ReadWriteCloser),
) *pluginsTestAgent {
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
			if hdr.Type != v2pb.StreamType_STREAM_TYPE_PLUGIN_MGMT {
				_ = stream.Close()
				continue
			}
			var req v2pb.PluginMgmtRequest
			_ = proto.Unmarshal(hdr.Metadata, &req)
			go func() {
				defer stream.Close()
				onMgmt(&req, stream)
			}()
		}
	}()

	t.Cleanup(func() {
		agentSess.Close()
		peer.Close()
	})
	return &pluginsTestAgent{svc: svc, peer: peer, fixture: fixture}
}

// authed wraps an arbitrary HTTP method against the plugin endpoint
// suffix with the project's admin bearer token.
func (a *pluginsTestAgent) authed(t *testing.T, method, srvURL, suffix, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, srvURL+a.fixture.URL(suffix), strings.NewReader(body))
	if err != nil {
		t.Fatalf("http.NewRequest: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+a.fixture.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	return resp
}

func registerPluginsRoute(r *gin.Engine, a *pluginsTestAgent) {
	h := NewAgentPluginsHandler(a.svc)
	RegisterV1AgentPluginRoutes(r, h, a.fixture.RBAC)
}

// ---- happy-path tests ---------------------------------------------

func TestAgentPluginsV1_List(t *testing.T) {
	a := setupPluginsAgent(t, "agent-plug-list",
		func(req *v2pb.PluginMgmtRequest, stream io.ReadWriteCloser) {
			if req.GetList() == nil {
				t.Errorf("expected list op, got %T", req.GetOp())
			}
			_ = link.WriteFrame(stream, &v2pb.PluginMgmtResponse{
				Result: &v2pb.PluginMgmtResponse_List{
					List: &v2pb.PluginListResponse{Plugins: []*v2pb.PluginInfo{
						{Id: "com.example.foo", Name: "Foo", Version: "1.0.0", Enabled: true},
						{Id: "com.example.bar", Name: "Bar", Version: "2.0.0", Enabled: false},
					}},
				},
			})
		},
	)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	registerPluginsRoute(r, a)
	srv := httptest.NewServer(r)
	defer srv.Close()

	resp := a.authed(t, http.MethodGet, srv.URL, "/plugins", "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body = %s", resp.StatusCode, b)
	}
	var got struct {
		Plugins []pluginInfoJSON `json:"plugins"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Plugins) != 2 {
		t.Fatalf("plugins = %+v, want 2 entries", got.Plugins)
	}
	if got.Plugins[0].ID != "com.example.foo" || got.Plugins[1].Enabled {
		t.Errorf("unexpected plugin payload: %+v", got.Plugins)
	}
}

func TestAgentPluginsV1_Install(t *testing.T) {
	a := setupPluginsAgent(t, "agent-plug-install",
		func(req *v2pb.PluginMgmtRequest, stream io.ReadWriteCloser) {
			install := req.GetInstall()
			if install == nil {
				t.Errorf("expected install op")
				return
			}
			if install.GetPluginId() != "com.example.foo" {
				t.Errorf("plugin_id = %q", install.GetPluginId())
			}
			if got, want := install.GetGrantedCapabilities(), []string{"kv"}; len(got) != 1 || got[0] != want[0] {
				t.Errorf("granted = %v", got)
			}
			// Drain + discard the three inline chunks.
			for i := 0; i < 3; i++ {
				var c v2pb.PluginInstallChunk
				_ = link.ReadFrame(stream, &c)
			}
			// Emit the canonical happy-path progression.
			for _, ph := range []v2pb.PluginInstallProgress_Phase{
				v2pb.PluginInstallProgress_PHASE_RECEIVE,
				v2pb.PluginInstallProgress_PHASE_VERIFY_SHA,
				v2pb.PluginInstallProgress_PHASE_VERIFY_SIG,
				v2pb.PluginInstallProgress_PHASE_EXTRACT,
				v2pb.PluginInstallProgress_PHASE_LOAD,
				v2pb.PluginInstallProgress_PHASE_INSTALLED,
			} {
				_ = link.WriteFrame(stream, &v2pb.PluginInstallProgress{Phase: ph})
			}
		},
	)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	registerPluginsRoute(r, a)
	srv := httptest.NewServer(r)
	defer srv.Close()

	body := `{"plugin_id":"com.example.foo","version":"1.0.0",` +
		`"publisher_pubkey":"pk-bytes",` +
		`"manifest_b64":"` + b64("manifest") + `",` +
		`"wasm_b64":"` + b64("wasm") + `",` +
		`"signature_b64":"` + b64("sig") + `",` +
		`"granted_capabilities":["kv"]}`
	resp := a.authed(t, http.MethodPost, srv.URL, "/plugins", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body = %s", resp.StatusCode, b)
	}
	var got installResponse
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Status != "installed" {
		t.Errorf("status = %q, want installed; full = %+v", got.Status, got)
	}
	if len(got.Progress) < 6 {
		t.Errorf("expected at least 6 progress frames, got %d", len(got.Progress))
	}
}

func TestAgentPluginsV1_InstallAgentReportsFailure(t *testing.T) {
	a := setupPluginsAgent(t, "agent-plug-install-fail",
		func(req *v2pb.PluginMgmtRequest, stream io.ReadWriteCloser) {
			for i := 0; i < 3; i++ {
				var c v2pb.PluginInstallChunk
				_ = link.ReadFrame(stream, &c)
			}
			_ = link.WriteFrame(stream, &v2pb.PluginInstallProgress{
				Phase:        v2pb.PluginInstallProgress_PHASE_FAILED,
				ErrorCode:    "signature_mismatch",
				ErrorMessage: "did not verify",
			})
		},
	)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	registerPluginsRoute(r, a)
	srv := httptest.NewServer(r)
	defer srv.Close()

	body := `{"plugin_id":"com.example.foo","version":"1.0.0",` +
		`"publisher_pubkey":"pk",` +
		`"manifest_b64":"` + b64("m") + `",` +
		`"wasm_b64":"` + b64("w") + `",` +
		`"signature_b64":"` + b64("s") + `"}`
	resp := a.authed(t, http.MethodPost, srv.URL, "/plugins", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body = %s", resp.StatusCode, b)
	}
	var got installResponse
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Status != "failed" {
		t.Errorf("status = %q, want failed", got.Status)
	}
	last := got.Progress[len(got.Progress)-1]
	if last.ErrorCode != "signature_mismatch" {
		t.Errorf("error_code = %q", last.ErrorCode)
	}
}

func TestAgentPluginsV1_Enable(t *testing.T) {
	a := setupPluginsAgent(t, "agent-plug-enable",
		func(req *v2pb.PluginMgmtRequest, stream io.ReadWriteCloser) {
			en := req.GetEnable()
			if en == nil {
				t.Errorf("expected enable op")
				return
			}
			if en.GetPluginId() != "com.example.foo" || !en.GetEnabled() {
				t.Errorf("enable req = %+v", en)
			}
			_ = link.WriteFrame(stream, &v2pb.PluginMgmtResponse{
				Result: &v2pb.PluginMgmtResponse_Enable{Enable: &v2pb.PluginEnableResponse{}},
			})
		},
	)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	registerPluginsRoute(r, a)
	srv := httptest.NewServer(r)
	defer srv.Close()

	resp := a.authed(t, http.MethodPatch, srv.URL, "/plugins/com.example.foo",
		`{"enabled":true}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body = %s", resp.StatusCode, b)
	}
}

func TestAgentPluginsV1_Uninstall(t *testing.T) {
	a := setupPluginsAgent(t, "agent-plug-uninstall",
		func(req *v2pb.PluginMgmtRequest, stream io.ReadWriteCloser) {
			un := req.GetUninstall()
			if un == nil || un.GetPluginId() != "com.example.foo" {
				t.Errorf("expected uninstall com.example.foo, got %+v", un)
			}
			_ = link.WriteFrame(stream, &v2pb.PluginMgmtResponse{
				Result: &v2pb.PluginMgmtResponse_Uninstall{Uninstall: &v2pb.PluginUninstallResponse{}},
			})
		},
	)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	registerPluginsRoute(r, a)
	srv := httptest.NewServer(r)
	defer srv.Close()

	resp := a.authed(t, http.MethodDelete, srv.URL, "/plugins/com.example.foo", "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body = %s", resp.StatusCode, b)
	}
}

func TestAgentPluginsV1_Logs(t *testing.T) {
	a := setupPluginsAgent(t, "agent-plug-logs",
		func(req *v2pb.PluginMgmtRequest, stream io.ReadWriteCloser) {
			gl := req.GetGetLogs()
			if gl == nil || gl.GetPluginId() != "com.example.foo" {
				t.Errorf("expected get_logs com.example.foo, got %+v", gl)
			}
			_ = link.WriteFrame(stream, &v2pb.PluginMgmtResponse{
				Result: &v2pb.PluginMgmtResponse_GetLogs{
					GetLogs: &v2pb.PluginGetLogsResponse{
						Entries: []*v2pb.PluginLogEntry{
							{UnixNano: time.Now().UnixNano(), Level: "info", Message: "hello"},
						},
					},
				},
			})
		},
	)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	registerPluginsRoute(r, a)
	srv := httptest.NewServer(r)
	defer srv.Close()

	resp := a.authed(t, http.MethodGet, srv.URL, "/plugins/com.example.foo/logs?tail=10", "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body = %s", resp.StatusCode, b)
	}
	var got struct {
		Entries []pluginLogEntryJSON `json:"entries"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Entries) != 1 || got.Entries[0].Message != "hello" {
		t.Errorf("entries = %+v", got.Entries)
	}
}

func TestAgentPluginsV1_AgentNotConnected(t *testing.T) {
	fixture := newAgentRouteFixture(t, "agent-ghost-plug")
	svc := core.NewAgentLinkService() // empty
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterV1AgentPluginRoutes(r, NewAgentPluginsHandler(svc), fixture.RBAC)
	srv := httptest.NewServer(r)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL+fixture.URL("/plugins"), nil)
	req.Header.Set("Authorization", "Bearer "+fixture.Token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }
