package restful

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"
	"time"

	"github.com/WangYihang/Platypus/internal/context"
	client_model "github.com/WangYihang/Platypus/internal/model/client"
	server_model "github.com/WangYihang/Platypus/internal/model/server"
	"github.com/WangYihang/Platypus/internal/util/fs"
	"github.com/WangYihang/Platypus/internal/util/log"
	"github.com/WangYihang/Platypus/internal/util/message"
	"github.com/WangYihang/Platypus/internal/util/str"
	"github.com/WangYihang/Platypus/internal/util/ui"
	"github.com/WangYihang/Platypus/internal/util/validator"
	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"
	"gopkg.in/olahol/melody.v1"
)

func CreateRESTfulAPIServer() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	gin.DefaultWriter = ioutil.Discard
	endpoint := gin.Default()

	endpoint.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "DELETE", "PUT", "PATCH"},
		AllowHeaders:     []string{"Origin"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	// Notify client online event
	notifyWebSocket := melody.New()
	endpoint.GET("/notify", func(c *gin.Context) {
		notifyWebSocket.HandleRequest(c.Writer, c.Request)
	})

	notifyWebSocket.HandleConnect(func(s *melody.Session) {
		log.Info("Notify client conencted from: %s", s.Request.RemoteAddr)
	})

	notifyWebSocket.HandleMessage(func(s *melody.Session, msg []byte) {
		// Nothing to do
	})

	notifyWebSocket.HandleDisconnect(func(s *melody.Session) {
		log.Info("Notify client disconencted from: %s", s.Request.RemoteAddr)
	})
	context.Ctx.NotifyWebSocket = notifyWebSocket

	// Websocket
	ttyWebSocket := melody.New()
	ttyWebSocket.Upgrader.Subprotocols = []string{"tty"}
	endpoint.GET("/ws/:hash", func(c *gin.Context) {
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
	})

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

	// Static files
	endpoint.Use(static.Serve("/", fs.BinaryFileSystem("./html/frontend/build")))
	// WebSocket TTYd
	endpoint.Use(static.Serve("/shell/", fs.BinaryFileSystem("./html/ttyd/dist")))

	// TODO: Websocket UI Auth (to be implemented)
	endpoint.GET("/token", func(c *gin.Context) {
		c.String(200, "")
	})

	// Server related
	// Simple group: v1
	RESTfulAPIGroup := endpoint.Group("/api")
	{
		serverAPIGroup := RESTfulAPIGroup.Group("/server")
		{
			serverAPIGroup.GET("", server_model.ListServers)
			serverAPIGroup.GET("/:hash", server_model.GetServerInfo)
			serverAPIGroup.GET("/:hash/client", server_model.GetServerClients)
			serverAPIGroup.POST("", server_model.CreateServer)
			serverAPIGroup.DELETE("/:hash", server_model.DeleteServer)
		}
		clientAPIGroup := RESTfulAPIGroup.Group("/client")
		{

			// Client related
			clientAPIGroup.GET("", client_model.ListAllClients)
			clientAPIGroup.GET("/:hash", client_model.GetClientInfo)
			// Upgrade reverse shell client to termite client
			clientAPIGroup.GET("/:hash/upgrade/:target", client_model.UpgradeClient)
			clientAPIGroup.DELETE("/:hash", client_model.DeleteClient)
			clientAPIGroup.POST("/:hash", client_model.ExecuteCommand)
		}
	}
	return endpoint
}
