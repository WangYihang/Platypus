package context

import (
	"fmt"
	"io/ioutil"
	"strconv"
	"strings"
	"time"

	"github.com/WangYihang/Platypus/lib/util/fs"
	"github.com/WangYihang/Platypus/lib/util/log"
	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"
	"gopkg.in/olahol/melody.v1"
)

type WindowSize struct {
	Columns int
	Rows    int
}

func formExistOrAbort(c *gin.Context, params []string) bool {
	for _, param := range params {
		if c.PostForm(param) == "" {
			return panicRESTfully(c, fmt.Sprintf("%s is required", param))
		}
	}
	return true
}

func paramsExistOrAbort(c *gin.Context, params []string) bool {
	for _, param := range params {
		if c.Param(param) == "" {
			return panicRESTfully(c, fmt.Sprintf("%s is required", param))
		}
	}
	return true
}

func panicRESTfully(c *gin.Context, msg string) bool {
	c.JSON(200, gin.H{
		"status": false,
		"msg":    msg,
	})
	c.Abort()
	return false
}

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
	Ctx.NotifyWebSocket = notifyWebSocket

	// Websocket
	ttyWebSocket := melody.New()
	ttyWebSocket.Upgrader.Subprotocols = []string{"tty"}
	endpoint.GET("/ws/:hash", func(c *gin.Context) {
		if !paramsExistOrAbort(c, []string{"hash"}) {
			return
		}
		client := Ctx.FindTCPClientByHash(c.Param("hash"))
		if client == nil {
			panicRESTfully(c, fmt.Sprintf("client is not found"))
			return
		}
		log.Success("Poping up websocket shell for: %s", client.OnelineDesc())
		ttyWebSocket.HandleRequest(c.Writer, c.Request)
	})

	ttyWebSocket.HandleConnect(func(s *melody.Session) {
		// Get client hash
		hash := strings.Split(s.Request.URL.Path, "/")[2]
		current := Ctx.FindTCPClientByHash(hash)
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
	})

	ttyWebSocket.HandleMessageBinary(func(s *melody.Session, msg []byte) {
		// Get client hash
		value, _ := s.Get("client")
		current := value.(*TCPClient)
		if current.GetInteractive() {
			opcode := msg[0]
			body := msg[1:]
			switch opcode {
			case '0': // INPUT '0'
				current.Write(body)
			case '1': // RESIZE_TERMINAL '1'
				// Raw reverse shell does not support resize terminal size when
				// in interactive foreground program, eg: vim
				// var ws WindowSize
				// json.Unmarshal(body, &ws)
				// current.SetWindowSize(&ws)
			case '2': // PAUSE '2'
				// TODO: Pause, support for zmodem
			case '3': // RESUME '3'
				// TODO: Pause, support for zmodem
			case '{': // JSON_DATA '{'
				// Raw reverse shell does not support resize terminal size when
				// in interactive foreground program, eg: vim
				// var ws WindowSize
				// json.Unmarshal([]byte("{"+string(body)), &ws)
				// current.SetWindowSize(&ws)
			default:
				fmt.Println("Invalid message: ", string(msg))
			}
		}
	})

	ttyWebSocket.HandleDisconnect(func(s *melody.Session) {
		// Get client hash
		value, _ := s.Get("client")
		current := value.(*TCPClient)
		log.Success("Closing websocket shell for: %s", current.OnelineDesc())
		current.SetInteractive(false)
		current.GetInteractingLock().Unlock()
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
			serverAPIGroup.GET("", func(c *gin.Context) {
				c.JSON(200, gin.H{
					"status": true,
					"msg":    Ctx.Servers,
				})
				c.Abort()
				return
			})
			serverAPIGroup.GET("/:hash", func(c *gin.Context) {
				if !paramsExistOrAbort(c, []string{"hash"}) {
					return
				}
				hash := c.Param("hash")
				for _, server := range Ctx.Servers {
					if server.Hash() == hash {
						c.JSON(200, gin.H{
							"status": true,
							"msg":    server,
						})
						c.Abort()
						return
					}
				}
				panicRESTfully(c, "No such server")
				return
			})
			serverAPIGroup.GET("/:hash/client", func(c *gin.Context) {
				if !paramsExistOrAbort(c, []string{"hash"}) {
					return
				}
				hash := c.Param("hash")
				for _, server := range Ctx.Servers {
					if server.Hash() == hash {
						c.JSON(200, gin.H{
							"status": true,
							"msg":    server.Clients,
						})
						c.Abort()
						return
					}
				}
				panicRESTfully(c, "No such server")
				return
			})
			serverAPIGroup.POST("", func(c *gin.Context) {
				if !formExistOrAbort(c, []string{"host", "port"}) {
					return
				}
				port, err := strconv.Atoi(c.PostForm("port"))
				if err != nil || port <= 0 || port > 65535 {
					panicRESTfully(c, "Invalid port number")
					return
				}
				hashFormat := "%i %u %m %o"
				server := CreateTCPServer(c.PostForm("host"), uint16(port), hashFormat)
				if server != nil {
					go (*server).Run()
					c.JSON(200, gin.H{
						"status": true,
						"msg":    server,
					})
					c.Abort()
				} else {
					c.JSON(200, gin.H{
						"status": false,
						"msg":    fmt.Sprintf("The server (%s:%d) already exists", c.PostForm("host"), port),
					})
					c.Abort()
				}
				return
			})
			serverAPIGroup.DELETE("/:hash", func(c *gin.Context) {
				if !paramsExistOrAbort(c, []string{"hash"}) {
					return
				}
				hash := c.Param("hash")
				for _, server := range Ctx.Servers {
					if server.Hash() == hash {
						Ctx.DeleteServer(server)
						c.JSON(200, gin.H{
							"status": true,
						})
						c.Abort()
						return
					}
				}
				panicRESTfully(c, "No such server")
				return
			})
		}
		clientAPIGroup := RESTfulAPIGroup.Group("/client")
		{

			// Client related
			clientAPIGroup.GET("", func(c *gin.Context) {
				clients := []TCPClient{}
				for _, server := range Ctx.Servers {
					for _, client := range (*server).GetAllTCPClients() {
						clients = append(clients, *client)
					}
				}
				c.JSON(200, gin.H{
					"status": true,
					"msg":    clients,
				})
				c.Abort()
				return
			})
			clientAPIGroup.GET("/:hash", func(c *gin.Context) {
				if !paramsExistOrAbort(c, []string{"hash"}) {
					return
				}
				hash := c.Param("hash")
				for _, server := range Ctx.Servers {
					if client, exist := server.Clients[hash]; exist {
						c.JSON(200, gin.H{
							"status": true,
							"msg":    client,
						})
						c.Abort()
						return
					}
				}
				panicRESTfully(c, "No such client")
				return
			})
			clientAPIGroup.DELETE("/:hash", func(c *gin.Context) {
				if !paramsExistOrAbort(c, []string{"hash"}) {
					return
				}
				hash := c.Param("hash")
				for _, server := range Ctx.Servers {
					if client, exist := server.Clients[hash]; exist {
						Ctx.DeleteTCPClient(client)
						c.JSON(200, gin.H{
							"status": true,
						})
						c.Abort()
						return
					}
				}
				panicRESTfully(c, "No such client")
				return
			})
			clientAPIGroup.POST("/:hash", func(c *gin.Context) {
				if !paramsExistOrAbort(c, []string{"hash"}) {
					return
				}
				if !formExistOrAbort(c, []string{"cmd"}) {
					return
				}
				hash := c.Param("hash")
				cmd := c.PostForm("cmd")
				for _, server := range Ctx.Servers {
					if client, exist := server.Clients[hash]; exist {
						if client.GetPtyEstablished() {
							c.JSON(200, gin.H{
								"status": false,
								"msg":    "The client is under PTY mode, please exit pty mode before execute command on it",
							})
						} else {
							c.JSON(200, gin.H{
								"status": true,
								"msg":    client.SystemToken(cmd),
							})
						}
						c.Abort()
						return
					}
				}
				panicRESTfully(c, "No such client")
				return
			})
		}
	}
	return endpoint
}
