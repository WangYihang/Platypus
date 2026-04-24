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

// v2 terminal handler at /api/v1/terminal/:agent_id/ws:
//   1. Look up the agent's *link.Session via AgentLinkService.
//   2. Read the first binary WS frame (must be resize opcode '1')
//      to learn cols/rows.
//   3. Open STREAM_TYPE_PROCESS_OPEN on the agent's session.
//   4. Read ProcessOpenResponse; if Error, close the WS.
//   5. Bidirectionally bridge: browser WS frames (opcode '0' input,
//      '1' resize) → ProcessFrame.stdin / .resize; agent
//      ProcessFrame.stdout → browser binary frame '0' + bytes.
//   6. On stream or WS EOF, close the other.

// setupTerminalV2Test stands up an httptest.Server running the v2
// terminal handler against a fake agent that implements
// HandleProcessStream. Returns the server + a paired-to-registered-
// agent teardown.
func setupTerminalV2Test(t *testing.T, agentID string) *httptest.Server {
	t.Helper()

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
	RegisterV2TerminalRoute(r, svc)
	return httptest.NewServer(r)
}

func TestTerminalV2_StdoutBridge(t *testing.T) {
	srv := setupTerminalV2Test(t, "agent-term-1")
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	wsURL := strings.Replace(srv.URL, "http://", "ws://", 1) +
		"/api/v1/terminal/agent-term-1/ws"
	c, _, err := websocket.Dial(ctx, wsURL, nil)
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

// Unknown agent id → 404 on the HTTP Upgrade.
func TestTerminalV2_UnknownAgent404(t *testing.T) {
	svc := core.NewAgentLinkService()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterV2TerminalRoute(r, svc)
	srv := httptest.NewServer(r)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/terminal/missing/ws")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d; want 404", resp.StatusCode)
	}
}
