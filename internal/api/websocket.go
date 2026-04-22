package api

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"gopkg.in/olahol/melody.v1"

	"github.com/WangYihang/Platypus/internal/core"
	"github.com/WangYihang/Platypus/internal/log"
	"github.com/WangYihang/Platypus/internal/utils/str"
	agentpb "github.com/WangYihang/Platypus/pkg/proto/agent/v1"
)

// RegisterWebSocketRoutes wires up /notify and /ws/:hash WebSocket endpoints
// behind Bearer-or-Ticket auth. Callers that can set HTTP headers (the Go
// desktop client) send Authorization: Bearer <token>; browsers — which can't
// set arbitrary WS upgrade headers — trade a Bearer token for a short-lived
// ticket at /api/v1/ws/ticket and pass it as ?ticket=<value> here.
func RegisterWebSocketRoutes(engine *gin.Engine, auth *Auth) {
	notify := newNotifyWebSocket()
	engine.GET("/notify", wsAuthMiddleware(auth), func(c *gin.Context) {
		notify.HandleRequest(c.Writer, c.Request)
	})
	core.Ctx.NotifyWebSocket = notify

	tty := newTTYWebSocket()
	engine.GET("/ws/:hash", wsAuthMiddleware(auth), func(c *gin.Context) {
		if !paramsExistOrAbort(c, []string{"hash"}) {
			return
		}
		agentClient := core.FindAgentClientByHash(c.Param("hash"))
		if agentClient == nil {
			abortWithError(c, 404, "client is not found")
			return
		}
		log.Success("Opening websocket shell for: %s", agentClient.OnelineDesc())
		tty.HandleRequest(c.Writer, c.Request)
	})
}

// wsAuthMiddleware gates a WebSocket route on either a Bearer header OR a
// valid one-shot ticket in the ?ticket= query. Both paths 401 on failure so
// a WS upgrade is never attempted for unauthenticated clients.
func wsAuthMiddleware(auth *Auth) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Bearer header path (native clients that can set headers).
		if h := c.GetHeader("Authorization"); h != "" {
			if parts := strings.SplitN(h, " ", 2); len(parts) == 2 &&
				strings.EqualFold(parts[0], "bearer") &&
				auth.ValidateToken(parts[1]) {
				c.Next()
				return
			}
		}
		// Ticket path (browsers).
		if tk := c.Query("ticket"); tk != "" && auth.ConsumeWSTicket(tk) {
			c.Next()
			return
		}
		c.AbortWithStatusJSON(401, gin.H{"error": "websocket auth required (Bearer header or ?ticket=)"})
	}
}

func newNotifyWebSocket() *melody.Melody {
	m := melody.New()
	// Events are small JSON blobs, but pin the cap explicitly so we don't
	// silently sit on melody's 512-byte default. Ping/pong defaults (54s/60s)
	// are kept so dead connections behind firewalls get reaped.
	m.Config.MaxMessageSize = 64 * 1024
	m.HandleConnect(func(s *melody.Session) {
		log.Info("Notify client connected from: %s", s.Request.RemoteAddr)
	})
	m.HandleMessage(func(s *melody.Session, msg []byte) {
		// no-op: notify is one-way (server → client)
	})
	m.HandleDisconnect(func(s *melody.Session) {
		log.Info("Notify client disconnected from: %s", s.Request.RemoteAddr)
	})
	return m
}

func newTTYWebSocket() *melody.Melody {
	tty := melody.New()
	tty.Upgrader.Subprotocols = []string{"tty"}
	// melody's 512-byte default would sever a terminal session the moment a
	// user pasted a modest line. 1 MiB matches what xterm.js sends in one
	// frame for typical paste sizes; ping/pong defaults handle idle reaping.
	tty.Config.MaxMessageSize = 1 << 20

	tty.HandleConnect(func(s *melody.Session) {
		hash := strings.Split(s.Request.URL.Path, "/")[2]
		if currentAgent := core.FindAgentClientByHash(hash); currentAgent != nil {
			handleAgentClientConnect(s, currentAgent)
			return
		}
	})

	tty.HandleMessageBinary(func(s *melody.Session, msg []byte) {
		if agentValue, exists := s.Get("agentClient"); exists {
			handleAgentClientMessage(s, agentValue.(*core.AgentClient), msg)
		}
	})

	tty.HandleDisconnect(func(s *melody.Session) {
		if agentValue, exists := s.Get("agentClient"); exists {
			currentAgent := agentValue.(*core.AgentClient)
			if key, exists := s.Get("key"); exists {
				currentAgent.RequestTerminate(key.(string))
			} else {
				log.Error("missing process key on agent ws disconnect")
			}
		}
	})

	return tty
}

func handleAgentClientConnect(s *melody.Session, currentAgent *core.AgentClient) {
	log.Info("Agent websocket connected: %s", currentAgent.OnelineDesc())
	s.Set("agentClient", currentAgent)

	// SET_WINDOW_TITLE '1'
	s.WriteBinary([]byte("1" + currentAgent.GetShellPath() + " (ubuntu)"))
	// SET_PREFERENCES '2'
	s.WriteBinary([]byte("2" + "{ }"))

	key := str.RandomString(0x10)
	s.Set("key", key)

	// Don't start the process yet — wait for the first resize message from
	// xterm.js so we can pass real cols/rows to the PTY. Starting with 0×0
	// breaks ncurses apps (tmux, vim) until the first SIGWINCH arrives.
	process := core.Process{
		Pid:           -2,
		WindowColumns: 0,
		WindowRows:    0,
		State:         core.StartRequested,
		WebSocket:     s,
	}
	currentAgent.AddProcess(key, &process)
}

// TTY opcodes (mirrors ttyd protocol)
const (
	opcodeInput          = '0'
	opcodeResizeTerminal = '1'
	opcodePause          = '2'
	opcodeResume         = '3'
)

// ttyAction is the high-level intent of a TTY WebSocket frame.
type ttyAction int

const (
	ttyActionUnknown ttyAction = iota
	ttyActionInput
	ttyActionResize
	ttyActionIgnore // recognized but intentionally dropped (pause/resume)
)

// classifyTTYOpcode maps the leading opcode byte to its action. The '{'
// opcode used by older ttyd clients is deliberately not handled: it collides
// with JSON written to stdin by shell pipelines, which caused silent byte
// loss. Resize requests must use opcode '1' with a JSON body.
func classifyTTYOpcode(b byte) ttyAction {
	switch b {
	case opcodeInput:
		return ttyActionInput
	case opcodeResizeTerminal:
		return ttyActionResize
	case opcodePause, opcodeResume:
		return ttyActionIgnore
	}
	return ttyActionUnknown
}

func handleAgentClientMessage(s *melody.Session, currentAgent *core.AgentClient, msg []byte) {
	keyVal, exists := s.Get("key")
	if !exists {
		log.Error("Process has not been started")
		return
	}
	key := keyVal.(string)
	body := msg[1:]

	switch classifyTTYOpcode(msg[0]) {
	case ttyActionInput:
		err := currentAgent.Send(&agentpb.Envelope{
			Payload: &agentpb.Envelope_Stdio{
				Stdio: &agentpb.StdioData{Key: key, Data: body},
			},
		})
		if err != nil {
			log.Error("Network error: %s", err)
		}
	case ttyActionResize:
		var ws core.WindowSize
		json.Unmarshal(body, &ws)

		// If the process hasn't been started yet (waiting for initial
		// dimensions from xterm.js), use this resize to kick it off with
		// real cols/rows so ncurses apps work immediately.
		if proc := currentAgent.GetProcess(key); proc != nil && proc.State == core.StartRequested {
			currentAgent.RequestStartProcess(currentAgent.GetShellPath(), ws.Columns, ws.Rows, key)
			return
		}

		err := currentAgent.Send(&agentpb.Envelope{
			Payload: &agentpb.Envelope_WindowSize{
				WindowSize: &agentpb.WindowSizeUpdate{Key: key, Columns: int32(ws.Columns), Rows: int32(ws.Rows)},
			},
		})
		if err != nil {
			log.Error("Network error: %s", err)
		}
	case ttyActionIgnore:
		// TODO: support zmodem pause/resume
	default:
		fmt.Println("Invalid message: ", string(msg))
	}
}
