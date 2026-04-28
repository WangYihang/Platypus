package api

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/core"
	"github.com/WangYihang/Platypus/internal/link"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// v2 terminal handler at /api/v1/projects/:pid/agents/:agent_id/terminal/ws:
//   1. Look up the agent's *link.Session via AgentLinkService.
//   2. Read the first binary WS frame (must be resize opcode '1')
//      to learn cols/rows.
//   3. Open STREAM_TYPE_PROCESS_OPEN on the agent's session.
//   4. Read ProcessOpenResponse; if Error, close the WS.
//   5. Bidirectionally bridge: browser WS frames (opcode '0' input,
//      '1' resize) → ProcessFrame.stdin / .resize; agent
//      ProcessFrame.stdout → browser binary frame '0' + bytes.
//   6. On stream or WS EOF, close the other.

// terminalTestEnv pairs the live test server with the auth fixture so
// each test can build the WS URL + cookie auth (browsers can't set
// Bearer headers on WS Upgrade so the handler itself doesn't gate on
// the WS auth-ticket here — Bearer is allowed and is what real
// non-browser callers use).
type terminalTestEnv struct {
	srv     *httptest.Server
	fixture *agentRouteFixture
}

// setupTerminalV2Test stands up an httptest.Server running the v2
// terminal handler against a fake agent that implements
// HandleProcessStream. Returns the env + a paired-to-registered-
// agent teardown.
func setupTerminalV2Test(t *testing.T, agentID string) *terminalTestEnv {
	t.Helper()
	fixture := newAgentRouteFixture(t, agentID)

	// Build paired Sessions: one acts as the "agent", registered in
	// AgentLinkService under agentID; the other is what the handler
	// sees when it looks up the agent.
	svc := core.NewAgentLinkService()
	clientConn, serverConn := net.Pipe()

	// Register client side as the "agent's session" that the handler
	// will call sess.Open() on.
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
	t.Cleanup(func() {
		agentSess.Close()
		peer.Close()
	})

	// "Agent" side: accept streams, dispatch PROCESS_OPEN to a shell.
	go func() {
		for {
			hdr, stream, err := peer.Accept()
			if err != nil {
				return
			}
			if hdr.Type != v2pb.StreamType_STREAM_TYPE_PROCESS_OPEN {
				_ = stream.Close()
				continue
			}
			// Respond with a canned ProcessOpenResponse, then emit a
			// stdout frame and an exit frame, then close.
			go func(s io.ReadWriteCloser) {
				defer s.Close()
				_ = link.WriteFrame(s, &v2pb.ProcessOpenResponse{Pid: 1234})
				_ = link.WriteFrame(s, &v2pb.ProcessFrame{
					Payload: &v2pb.ProcessFrame_Stdout{Stdout: []byte("hello from v2\n")},
				})
				_ = link.WriteFrame(s, &v2pb.ProcessFrame{
					Payload: &v2pb.ProcessFrame_Exit{Exit: &v2pb.ExitInfo{Code: 0}},
				})
			}(stream)
		}
	}()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterV2TerminalRoute(r, svc, fixture.RBAC, nil)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return &terminalTestEnv{srv: srv, fixture: fixture}
}

func TestTerminalV2_StdoutBridge(t *testing.T) {
	env := setupTerminalV2Test(t, "agent-term-1")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	wsURL := strings.Replace(env.srv.URL, "http://", "ws://", 1) +
		env.fixture.URL("/terminal/ws")
	c, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader: http.Header{"Authorization": []string{"Bearer " + env.fixture.Token}},
	})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.CloseNow()

	// Send initial resize (opcode '1' + JSON cols/rows) — handler
	// uses this to open PROCESS_OPEN with real dimensions.
	if err := c.Write(ctx, websocket.MessageBinary,
		[]byte(`1{"columns":80,"rows":24}`)); err != nil {
		t.Fatalf("write resize: %v", err)
	}

	// Read frames until we see the "hello from v2" stdout. The
	// handler prefixes stdout with opcode '0'.
	deadline := time.Now().Add(2 * time.Second)
	var got string
	for time.Now().Before(deadline) {
		typ, data, err := c.Read(ctx)
		if err != nil {
			break
		}
		if typ != websocket.MessageBinary {
			continue
		}
		if len(data) > 0 && data[0] == '0' {
			got += string(data[1:])
			if strings.Contains(got, "hello from v2") {
				return
			}
		}
	}
	t.Fatalf("did not see expected stdout; got %q", got)
}

// Browser-close should immediately tear down the agent-side
// PROCESS_OPEN stream. Before the cross-cancel fix the WS-read pump
// closed only the in-handler `done` channel; the main pump remained
// blocked in link.ReadFrame waiting on the agent, leaking a
// goroutine + a yamux stream + (in production) the agent-side PTY
// process for as long as the agent was silent.
//
// Reproduce: a fake agent that holds the stream open without ever
// writing an Exit frame. The browser closes the WS. The agent-side
// stream MUST observe EOF/error within a tight bound; if it doesn't,
// the leak is back.
func TestTerminalV2_BrowserCloseTearsDownAgentStream(t *testing.T) {
	const agentID = "agent-term-leak"
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
	t.Cleanup(func() {
		agentSess.Close()
		peer.Close()
	})

	// Agent loop: accept the PROCESS_OPEN, ack it, then go silent and
	// only return when the stream itself observes an error (which is
	// what we want the handler to cause on browser disconnect).
	streamClosed := make(chan error, 1)
	go func() {
		hdr, stream, err := peer.Accept()
		if err != nil {
			streamClosed <- err
			return
		}
		if hdr.Type != v2pb.StreamType_STREAM_TYPE_PROCESS_OPEN {
			_ = stream.Close()
			streamClosed <- nil
			return
		}
		_ = link.WriteFrame(stream, &v2pb.ProcessOpenResponse{Pid: 4321})
		// Block on read. We expect the handler to close the stream
		// (cascade from browser-close) so this returns within a
		// reasonable bound.
		buf := make([]byte, 64)
		_, readErr := stream.Read(buf)
		_ = stream.Close()
		streamClosed <- readErr
	}()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterV2TerminalRoute(r, svc, fixture.RBAC, nil)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	dialCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	wsURL := strings.Replace(srv.URL, "http://", "ws://", 1) +
		fixture.URL("/terminal/ws")
	c, _, err := websocket.Dial(dialCtx, wsURL, &websocket.DialOptions{
		HTTPHeader: http.Header{"Authorization": []string{"Bearer " + fixture.Token}},
	})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	if err := c.Write(dialCtx, websocket.MessageBinary,
		[]byte(`1{"columns":80,"rows":24}`)); err != nil {
		t.Fatalf("write resize: %v", err)
	}

	// Browser close. After this the handler must propagate teardown
	// to the agent-side stream — that's the bug we're guarding.
	_ = c.Close(websocket.StatusNormalClosure, "bye")

	select {
	case err := <-streamClosed:
		if err == nil || err == io.EOF {
			return // expected: handler closed the stream
		}
		// Any error is acceptable evidence the stream was torn down.
		return
	case <-time.After(3 * time.Second):
		t.Fatal("agent-side stream still open 3s after browser close: handler did not propagate WS-close into stream teardown")
	}
}

// Unknown agent id (no live link.Session) → 404 on the HTTP Upgrade.
// The host row + project membership are pre-seeded by the fixture so
// the request makes it past the auth and project-scope gates and
// reaches the link-service lookup.
func TestTerminalV2_UnknownAgent404(t *testing.T) {
	fixture := newAgentRouteFixture(t, "ghost-agent")
	svc := core.NewAgentLinkService()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterV2TerminalRoute(r, svc, fixture.RBAC, nil)
	srv := httptest.NewServer(r)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL+fixture.URL("/terminal/ws"), nil)
	req.Header.Set("Authorization", "Bearer "+fixture.Token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d; want 404", resp.StatusCode)
	}
}
