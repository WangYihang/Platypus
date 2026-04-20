package api

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gopkg.in/olahol/melody.v1"

	"github.com/WangYihang/Platypus/internal/core"
	"github.com/WangYihang/Platypus/internal/log"
	"github.com/WangYihang/Platypus/internal/utils/str"
	agentpb "github.com/WangYihang/Platypus/pkg/proto/agent/v1"
)

// RegisterWebSocketRoutes wires up /notify and /ws/:hash WebSocket endpoints.
// Auth on these is currently delegated to the originating client (token in URL
// or out-of-band); migrating to header-based auth is tracked in the modernization plan.
func RegisterWebSocketRoutes(engine *gin.Engine) {
	notify := newNotifyWebSocket()
	engine.GET("/notify", func(c *gin.Context) {
		notify.HandleRequest(c.Writer, c.Request)
	})
	core.Ctx.NotifyWebSocket = notify

	tty := newTTYWebSocket()
	engine.GET("/ws/:hash", func(c *gin.Context) {
		if !paramsExistOrAbort(c, []string{"hash"}) {
			return
		}
		client := core.FindTCPClientByHash(c.Param("hash"))
		termiteClient := core.FindTermiteClientByHash(c.Param("hash"))
		if client == nil && termiteClient == nil {
			abortWithLegacyError(c, 404, "client is not found")
			return
		}
		if client != nil {
			log.Success("Trying to poping up websocket shell for: %s", client.OnelineDesc())
		}
		if termiteClient != nil {
			log.Success("Trying to poping up encrypted websocket shell for: %s", termiteClient.OnelineDesc())
		}
		tty.HandleRequest(c.Writer, c.Request)
	})
}

func newNotifyWebSocket() *melody.Melody {
	m := melody.New()
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

	tty.HandleConnect(func(s *melody.Session) {
		hash := strings.Split(s.Request.URL.Path, "/")[2]

		// Reverse-shell client (raw TCP)
		if current := core.FindTCPClientByHash(hash); current != nil {
			handleTCPClientConnect(s, current)
			return
		}

		// Termite client (protobuf, encrypted)
		if currentTermite := core.FindTermiteClientByHash(hash); currentTermite != nil {
			handleTermiteClientConnect(s, currentTermite)
			return
		}
	})

	tty.HandleMessageBinary(func(s *melody.Session, msg []byte) {
		if value, exists := s.Get("client"); exists {
			handleTCPClientMessage(value.(*core.TCPClient), msg)
			return
		}
		if termiteValue, exists := s.Get("termiteClient"); exists {
			handleTermiteClientMessage(s, termiteValue.(*core.TermiteClient), msg)
		}
	})

	tty.HandleDisconnect(func(s *melody.Session) {
		if value, exists := s.Get("client"); exists {
			current := value.(*core.TCPClient)
			log.Success("Closing websocket shell for: %s", current.OnelineDesc())
			current.SetInteractive(false)
			current.GetInteractingLock().Unlock()
			return
		}
		if termiteValue, exists := s.Get("termiteClient"); exists {
			currentTermite := termiteValue.(*core.TermiteClient)
			if key, exists := s.Get("key"); exists {
				currentTermite.RequestTerminate(key.(string))
			} else {
				log.Error("missing process key on termite ws disconnect")
			}
		}
	})

	return tty
}

func handleTCPClientConnect(s *melody.Session, current *core.TCPClient) {
	s.Set("client", current)
	current.GetInteractingLock().Lock()
	current.SetInteractive(true)

	// Make sure PTY is up in case CLI is also interacting
	current.EstablishPTY()
	// SET_WINDOW_TITLE '1'
	s.WriteBinary([]byte("1" + current.GetShellPath() + " (ubuntu)"))
	// SET_PREFERENCES '2'
	s.WriteBinary([]byte("2" + "{ }"))

	// Trigger initial OUTPUT
	current.Write([]byte("\n"))

	go func() {
		for current != nil && !s.IsClosed() {
			current.GetConn().SetReadDeadline(time.Time{})
			msg := make([]byte, 0x100)
			n, err := current.ReadConnLock(msg)
			if err != nil {
				log.Error("Read from socket failed: %s", err)
				return
			}
			s.WriteBinary([]byte("0" + string(msg[0:n])))
		}
	}()
}

func handleTermiteClientConnect(s *melody.Session, currentTermite *core.TermiteClient) {
	log.Info("Encrypted websocket connected: %s", currentTermite.OnelineDesc())
	s.Set("termiteClient", currentTermite)

	// SET_WINDOW_TITLE '1'
	s.WriteBinary([]byte("1" + currentTermite.GetShellPath() + " (ubuntu)"))
	// SET_PREFERENCES '2'
	s.WriteBinary([]byte("2" + "{ }"))

	key := str.RandomString(0x10)
	s.Set("key", key)

	currentTermite.RequestStartProcess(currentTermite.GetShellPath(), 0, 0, key)

	process := core.Process{
		Pid:           -2,
		WindowColumns: 0,
		WindowRows:    0,
		State:         core.StartRequested,
		WebSocket:     s,
	}
	currentTermite.AddProcess(key, &process)
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

func handleTCPClientMessage(current *core.TCPClient, msg []byte) {
	if !current.GetInteractive() {
		return
	}
	switch classifyTTYOpcode(msg[0]) {
	case ttyActionInput:
		current.Write(msg[1:])
	case ttyActionResize, ttyActionIgnore:
		// Raw reverse shells don't support resize / pause / resume.
	default:
		fmt.Println("Invalid message: ", string(msg))
	}
}

func handleTermiteClientMessage(s *melody.Session, currentTermite *core.TermiteClient, msg []byte) {
	keyVal, exists := s.Get("key")
	if !exists {
		log.Error("Process has not been started")
		return
	}
	key := keyVal.(string)
	body := msg[1:]

	switch classifyTTYOpcode(msg[0]) {
	case ttyActionInput:
		err := currentTermite.Send(&agentpb.Envelope{
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
		err := currentTermite.Send(&agentpb.Envelope{
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
