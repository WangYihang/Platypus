package websocket

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/WangYihang/Platypus/internal/context"
	"github.com/WangYihang/Platypus/internal/util/log"
	"github.com/WangYihang/Platypus/internal/util/message"
	"github.com/WangYihang/Platypus/internal/util/str"
	"github.com/WangYihang/Platypus/internal/util/ui"
	"github.com/WangYihang/Platypus/internal/util/validator"
	"github.com/gin-gonic/gin"
	"gopkg.in/olahol/melody.v1"
)

func CreateTTYWebSocketServer() *melody.Melody {
	// Websocket
	ttyWebSocket := melody.New()
	ttyWebSocket.Upgrader.Subprotocols = []string{"tty"}

	ttyWebSocket.HandleConnect(func(s *melody.Session) {
		// Get client hash
		hash := strings.Split(s.Request.URL.Path, "/")[2]

		// Handle TCPClient
		current := context.Ctx.FindTCPClientByHash(hash)
		if current != nil {
			s.Set("client", current)
			// Lock
			current.GetInteractingLock().Lock()
			current.SetInteractive(true)

			// Incase somebody is interacting via cli
			current.EstablishPTY()
			// SET_WINDOW_TITLE '1'
			s.WriteBinary([]byte("1" + "/bin/bash (ubuntu)"))
			// SET_PREFERENCES '2'
			s.WriteBinary([]byte("2" + "{ }"))

			// OUTPUT '0'
			current.Write([]byte("\n"))
			go func(s *melody.Session) {
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
			}(s)
			return
		}

		// Handle TermiteClient
		currentTermite := context.Ctx.FindTermiteClientByHash(hash)
		if currentTermite != nil {
			log.Info("Encrypted websocket connected: %s", currentTermite.OnelineDesc())
			// Start process /bin/bash
			s.Set("termiteClient", currentTermite)

			// SET_WINDOW_TITLE '1'
			s.WriteBinary([]byte("1" + "/bin/bash (ubuntu)"))
			// SET_PREFERENCES '2'
			s.WriteBinary([]byte("2" + "{ }"))
			// OUTPUT '0'
			key := str.RandomString(0x10)
			s.Set("key", key)

			currentTermite.RequestStartProcess("/bin/bash", 0, 0, key)

			// Create Process Object
			process := context.Process{
				Pid:           -2,
				WindowColumns: 0,
				WindowRows:    0,
				State:         context.StartRequested,
				WebSocket:     s,
			}
			currentTermite.Processes[key] = &process
			return
		}
	})

	// User input from websocket -> process
	ttyWebSocket.HandleMessageBinary(func(s *melody.Session, msg []byte) {
		// Handle TCPClient
		value, exists := s.Get("client")
		if exists {
			current := value.(*context.TCPClient)
			if current.GetInteractive() {
				opcode := msg[0]
				body := msg[1:]
				switch opcode {
				case '0': // INPUT '0'
					current.Write(body)
				case '1': // RESIZE_TERMINAL '1'
					// Raw reverse shell does not support resize terminal size when
					// in interactive foreground program, eg: vim
					// var ws ui.WindowSize
					// json.Unmarshal(body, &ws)
					// current.SetWindowSize(&ws)
				case '2': // PAUSE '2'
					// TODO: Pause, support for zmodem
				case '3': // RESUME '3'
					// TODO: Pause, support for zmodem
				case '{': // JSON_DATA '{'
					// Raw reverse shell does not support resize terminal size when
					// in interactive foreground program, eg: vim
					// var ws ui.WindowSize
					// json.Unmarshal([]byte("{"+string(body)), &ws)
					// current.SetWindowSize(&ws)
				default:
					fmt.Println("Invalid message: ", string(msg))
				}
			}
			return
		}

		// Handle TermiteClient
		if termiteValue, exists := s.Get("termiteClient"); exists {
			currentTermite := termiteValue.(*context.TermiteClient)
			if key, exists := s.Get("key"); exists {
				opcode := msg[0]
				body := msg[1:]
				switch opcode {
				case '0': // INPUT '0'
					currentTermite.EncoderLock.Lock()
					err := currentTermite.Encoder.Encode(message.Message{
						Type: message.STDIO,
						Body: message.BodyStdio{
							Key:  key.(string),
							Data: body,
						},
					})
					currentTermite.EncoderLock.Unlock()

					if err != nil {
						// Network
						log.Error("Network error: %s", err)
						return
					}
				case '1': // RESIZE_TERMINAL '1'
					var ws ui.WindowSize
					json.Unmarshal(body, &ws)

					currentTermite.EncoderLock.Lock()
					err := currentTermite.Encoder.Encode(message.Message{
						Type: message.WINDOW_SIZE,
						Body: message.BodyWindowSize{
							Key:     key.(string),
							Columns: ws.Columns,
							Rows:    ws.Rows,
						},
					})
					currentTermite.EncoderLock.Unlock()

					if err != nil {
						// Network
						log.Error("Network error: %s", err)
						return
					}
				case '2': // PAUSE '2'
					// TODO: Pause, support for zmodem
				case '3': // RESUME '3'
					// TODO: Pause, support for zmodem
				case '{': // JSON_DATA '{'
					var ws ui.WindowSize
					json.Unmarshal([]byte(msg), &ws)

					currentTermite.EncoderLock.Lock()
					err := currentTermite.Encoder.Encode(message.Message{
						Type: message.WINDOW_SIZE,
						Body: message.BodyWindowSize{
							Key:     key.(string),
							Columns: ws.Columns,
							Rows:    ws.Rows,
						},
					})
					currentTermite.EncoderLock.Unlock()

					if err != nil {
						// Network
						log.Error("Network error: %s", err)
						return
					}
				default:
					fmt.Println("Invalid message: ", string(msg))
				}
			} else {
				log.Error("Process has not been started")
			}
		}
	})

	ttyWebSocket.HandleDisconnect(func(s *melody.Session) {
		// Handle TCPClient
		value, exists := s.Get("client")
		if exists {
			current := value.(*context.TCPClient)
			log.Success("Closing websocket shell for: %s", current.OnelineDesc())
			current.SetInteractive(false)
			current.GetInteractingLock().Unlock()
			return
		}

		// Handle TermiteClient
		termiteValue, exists := s.Get("termiteClient")
		if exists {
			currentTermite := termiteValue.(*context.TermiteClient)
			if key, exists := s.Get("key"); exists {
				currentTermite.RequestTerminate(key.(string))
			} else {
				log.Error("No such key: %d", key)
				return
			}
		}
	})
	return ttyWebSocket
}

func EstablishTTY(c *gin.Context) {
	ttyWebSocket := CreateTTYWebSocketServer()
	if !validator.ParamsExistOrAbort(c, []string{"hash"}) {
		return
	}
	client := context.Ctx.FindTCPClientByHash(c.Param("hash"))
	termiteClient := context.Ctx.FindTermiteClientByHash(c.Param("hash"))
	if client == nil && termiteClient == nil {
		validator.PanicRESTfully(c, "client is not found")
		return
	}
	if client != nil {
		log.Success("Trying to poping up websocket shell for: %s", client.OnelineDesc())
	}
	if termiteClient != nil {
		log.Success("Trying to poping up encrypted websocket shell for: %s", termiteClient.OnelineDesc())
	}
	ttyWebSocket.HandleRequest(c.Writer, c.Request)
}
