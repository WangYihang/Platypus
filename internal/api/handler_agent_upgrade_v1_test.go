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
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// upgradeTestAgent stands in for a real connected platypus-agent.
// It registers a link.Session under svc and runs onUpgrade for every
// STREAM_TYPE_AGENT_UPGRADE the server opens. Tests pass an
// onUpgrade closure that emits the canned UpgradeProgress sequence
// the test wants to assert on.
type upgradeTestAgent struct {
	svc     *core.AgentLinkService
	peer    *link.Session
	fixture *agentRouteFixture
}

func setupUpgradeAgent(
	t *testing.T,
	agentID string,
	onUpgrade func(*v2pb.AgentUpgradeRequest, io.ReadWriteCloser),
) *upgradeTestAgent {
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
			if hdr.Type != v2pb.StreamType_STREAM_TYPE_AGENT_UPGRADE {
				_ = stream.Close()
				continue
			}
			var req v2pb.AgentUpgradeRequest
			_ = proto.Unmarshal(hdr.Metadata, &req)
			go func() {
				defer stream.Close()
				onUpgrade(&req, stream)
			}()
		}
	}()

	t.Cleanup(func() {
		agentSess.Close()
		peer.Close()
	})
	return &upgradeTestAgent{svc: svc, peer: peer, fixture: fixture}
}

// authedPost fires an admin-authed POST against the upgrade endpoint.
func (a *upgradeTestAgent) authedPost(t *testing.T, srvURL, suffix, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, srvURL+a.fixture.URL(suffix),
		strings.NewReader(body))
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

func registerUpgradeRoute(r *gin.Engine, a *upgradeTestAgent) {
	h := NewAgentUpgradeHandler(a.svc)
	RegisterV1AgentUpgradeRoutes(r, h, a.fixture.RBAC)
}

func TestAgentUpgradeV1_Happy(t *testing.T) {
	a := setupUpgradeAgent(t, "agent-upg-ok",
		func(req *v2pb.AgentUpgradeRequest, stream io.ReadWriteCloser) {
			// Mirror what the real agent emits on a happy upgrade —
			// the handler just needs to see PHASE_EXITING in the
			// stream and trust the agent's PHASE_INSTALL succeeded.
			_ = link.WriteFrame(stream, &v2pb.UpgradeProgress{
				Phase: v2pb.UpgradeProgress_PHASE_FETCH_MANIFEST,
			})
			_ = link.WriteFrame(stream, &v2pb.UpgradeProgress{
				Phase:           v2pb.UpgradeProgress_PHASE_VERIFY_SIG,
				ResolvedVersion: "1.6.0",
			})
			_ = link.WriteFrame(stream, &v2pb.UpgradeProgress{
				Phase:           v2pb.UpgradeProgress_PHASE_EXITING,
				ResolvedVersion: "1.6.0",
			})
		},
	)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	registerUpgradeRoute(r, a)
	srv := httptest.NewServer(r)
	defer srv.Close()

	resp := a.authedPost(t, srv.URL, "/upgrade",
		`{"target_version":"1.6.0","channel":"stable"}`)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d; want 200; body=%s", resp.StatusCode, b)
	}
	var got upgradeResponse
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Status != "exited" {
		t.Errorf("Status = %q; want exited", got.Status)
	}
	if got.ResolvedVer != "1.6.0" {
		t.Errorf("ResolvedVer = %q; want 1.6.0", got.ResolvedVer)
	}
}

func TestAgentUpgradeV1_AgentReportsFailure(t *testing.T) {
	a := setupUpgradeAgent(t, "agent-upg-fail",
		func(req *v2pb.AgentUpgradeRequest, stream io.ReadWriteCloser) {
			_ = link.WriteFrame(stream, &v2pb.UpgradeProgress{
				Phase:        v2pb.UpgradeProgress_PHASE_FAILED,
				ErrorCode:    "signature_mismatch",
				ErrorMessage: "manifest sig did not verify",
			})
		},
	)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	registerUpgradeRoute(r, a)
	srv := httptest.NewServer(r)
	defer srv.Close()

	resp := a.authedPost(t, srv.URL, "/upgrade", `{}`)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadGateway {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d; want 502; body=%s", resp.StatusCode, b)
	}
	var got upgradeResponse
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Status != "failed" {
		t.Errorf("Status = %q; want failed", got.Status)
	}
	if got.ErrorCode != "signature_mismatch" {
		t.Errorf("ErrorCode = %q; want signature_mismatch", got.ErrorCode)
	}
}

func TestAgentUpgradeV1_AgentNotConnected(t *testing.T) {
	// No setup of the link — just bind the route and POST.
	fixture := newAgentRouteFixture(t, "agent-ghost")
	svc := core.NewAgentLinkService() // empty, nothing registered

	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewAgentUpgradeHandler(svc)
	RegisterV1AgentUpgradeRoutes(r, h, fixture.RBAC)
	srv := httptest.NewServer(r)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodPost, srv.URL+fixture.URL("/upgrade"), bytes.NewReader([]byte("{}")))
	req.Header.Set("Authorization", "Bearer "+fixture.Token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d; want 404; body=%s", resp.StatusCode, b)
	}
}

// TestAgentUpgradeV1_DefaultsChannel ensures an empty body still
// works — the agent should see channel="stable" because the server
// fills in the default.
func TestAgentUpgradeV1_DefaultsChannel(t *testing.T) {
	gotChan := make(chan string, 1)
	a := setupUpgradeAgent(t, "agent-upg-defaults",
		func(req *v2pb.AgentUpgradeRequest, stream io.ReadWriteCloser) {
			gotChan <- req.GetChannel()
			_ = link.WriteFrame(stream, &v2pb.UpgradeProgress{
				Phase:           v2pb.UpgradeProgress_PHASE_EXITING,
				ResolvedVersion: "1.6.0",
			})
		},
	)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	registerUpgradeRoute(r, a)
	srv := httptest.NewServer(r)
	defer srv.Close()

	resp := a.authedPost(t, srv.URL, "/upgrade", `{}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; want 200", resp.StatusCode)
	}

	select {
	case c := <-gotChan:
		if c != "stable" {
			t.Errorf("channel forwarded = %q; want stable", c)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("agent never observed the upgrade request")
	}
}

// _ = context to keep the imports list stable when a future test
// adds context-aware assertions.
var _ = context.Background
