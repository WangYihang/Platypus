package context

import (
	"fmt"
	"strconv"
	"time"

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
	// gin.SetMode(gin.ReleaseMode)
	gin.SetMode(gin.DebugMode)
	// gin.DefaultWriter = ioutil.Discard
	rest := gin.Default()

	rest.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "DELETE", "PUT", "PATCH"},
		AllowHeaders:     []string{"Origin"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	// Websocket
	m := melody.New()
	m.Upgrader.Subprotocols = []string{"tty"}
	rest.GET("/ws", func(c *gin.Context) {
		m.HandleRequest(c.Writer, c.Request)
	})

	m.HandleConnect(func(s *melody.Session) {
		// Set to interactive
		if Ctx.Current != nil {
			// Lock
			Ctx.Current.Interacting.Lock()
			Ctx.Current.Interactive = true

			// Incase somebody is interacting via cli
			Ctx.Current.EstablishPTY()
			// SET_WINDOW_TITLE '1'
			s.WriteBinary([]byte("1" + "/bin/bash (ubuntu)"))
			// SET_PREFERENCES '2'
			s.WriteBinary([]byte("2" + "{ }"))

			// OUTPUT '0'
			Ctx.Current.Write([]byte("\n"))
			go func(s *melody.Session) {
				for Ctx.Current != nil && !s.IsClosed() {
					Ctx.Current.GetConn().SetReadDeadline(time.Time{})
					msg := make([]byte, 0x100)
					n, err := Ctx.Current.ReadConnLock(msg)
					if err != nil {
						log.Error("Read from socket failed: %s", err)
						return
					}
					s.WriteBinary([]byte("0" + string(msg[0:n])))
				}
			}(s)
		} else {
			// TODO: Notify front end
		}
	})

	m.HandleMessageBinary(func(s *melody.Session, msg []byte) {
		if Ctx.Current != nil && Ctx.Current.Interactive {
			opcode := msg[0]
			body := msg[1:]
			switch opcode {
			case '0': // INPUT '0'
				Ctx.Current.Write(body)
			case '1': // RESIZE_TERMINAL '1'
				// Raw reverse shell does not support resize terminal size when
				// in interactive foreground program, eg: vim
				// var ws WindowSize
				// json.Unmarshal(body, &ws)
				// Ctx.Current.SetWindowSize(&ws)
			case '2': // PAUSE '2'
				// TODO: Pause, support for zmodem
			case '3': // RESUME '3'
				// TODO: Pause, support for zmodem
			case '{': // JSON_DATA '{'
				// Raw reverse shell does not support resize terminal size when
				// in interactive foreground program, eg: vim
				// var ws WindowSize
				// json.Unmarshal([]byte("{"+string(body)), &ws)
				// Ctx.Current.SetWindowSize(&ws)
			default:
				fmt.Println("Invalid message: ", string(msg))
			}
		} else {
			// TODO: Notify front end
		}
	})

	m.HandleDisconnect(func(s *melody.Session) {
		if Ctx.Current != nil && Ctx.Current.Interactive {
			Ctx.Current.Interactive = false
			Ctx.Current.Interacting.Unlock()
		}
	})

	rest.Use(static.Serve("/", static.LocalFile("./html/dist", false)))
	rest.GET("/token", func(c *gin.Context) {
		c.String(200, "")
	})

	// Server related
	rest.GET("/server", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status": true,
			"msg":    Ctx.Servers,
		})
		c.Abort()
		return
	})
	rest.GET("/server/:hash", func(c *gin.Context) {
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
	rest.GET("/server/:hash/client", func(c *gin.Context) {
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
	rest.POST("/server", func(c *gin.Context) {
		if !formExistOrAbort(c, []string{"host", "port"}) {
			return
		}
		port, err := strconv.Atoi(c.Param("port"))
		if err != nil {
			panicRESTfully(c, "Invalid port number")
			return
		}
		hashFormat := "%i %u %m %o"
		server := CreateTCPServer(c.Param("host"), uint16(port), hashFormat)
		go (*server).Run()
		c.JSON(200, gin.H{
			"status": true,
			"msg":    server,
		})
		c.Abort()
		return
	})
	rest.DELETE("/server/:hash", func(c *gin.Context) {
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

	// Client related
	rest.GET("/client", func(c *gin.Context) {
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
	rest.GET("/client/:hash", func(c *gin.Context) {
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
	rest.DELETE("/client/:hash", func(c *gin.Context) {
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
	rest.POST("/client/:hash", func(c *gin.Context) {
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
				c.JSON(200, gin.H{
					"status": true,
					"msg":    client.SystemToken(cmd),
				})
				c.Abort()
				return
			}
		}
		panicRESTfully(c, "No such client")
		return
	})

	return rest
}
