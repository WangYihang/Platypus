package restful

import (
	"io/ioutil"
	"time"

	"github.com/WangYihang/Platypus/internal/context"
	model_client "github.com/WangYihang/Platypus/internal/model/client"
	model_server "github.com/WangYihang/Platypus/internal/model/server"
	"github.com/WangYihang/Platypus/internal/util/fs"
	"github.com/WangYihang/Platypus/internal/util/log"
	"github.com/WangYihang/Platypus/internal/util/validator"
	web_jwt "github.com/WangYihang/Platypus/internal/web/jwt"
	"github.com/WangYihang/Platypus/internal/web/websocket"
	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"
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

	endpoint.Use(gin.Recovery())
	// Static files
	endpoint.Use(static.Serve("/", fs.BinaryFileSystem("./web/frontend/build")))
	endpoint.Use(static.Serve("/shell/", fs.BinaryFileSystem("./web/ttyd/dist")))
	// Authentication
	authMiddleware := web_jwt.Create()
	endpoint.POST("/login", authMiddleware.LoginHandler)
	apiNeedAuth := endpoint.Group("/api/v1")
	apiNeedAuth.Use(authMiddleware.MiddlewareFunc())
	{
		// Refresh time can be longer than token timeout
		apiNeedAuth.GET("/refresh_token", authMiddleware.RefreshHandler)
		serverAPIGroup := apiNeedAuth.Group("/servers")
		{
			serverAPIGroup.GET("", model_server.GetAllServers)
			serverAPIGroup.POST("", model_server.CreateServer)
			serverAPIGroup.GET("/:hash", model_server.GetServer)
			serverAPIGroup.GET("/:hash/start", model_server.StartServer)
			serverAPIGroup.GET("/:hash/stop", model_server.StopServer)
			serverAPIGroup.GET("/:hash/clients", model_server.GetAllClientsOfServer)
			serverAPIGroup.DELETE("/:hash", model_server.DeleteServer)
		}
		clientAPIGroup := apiNeedAuth.Group("/clients")
		{
			clientAPIGroup.GET("", model_client.GetAllClients)
			// Basic operations
			clientAPIGroup.GET("/:hash", model_client.GetClient)
			clientAPIGroup.GET("/:hash/collect", model_client.CollectClientInfo)
			// Proxies
			clientAPIGroup.GET("/:hash/proxies", model_client.GetAllProxies)
			clientAPIGroup.POST("/:hash/proxies", model_client.CreateProxy)
			clientAPIGroup.DELETE("/:hash/proxies", model_client.DeleteProxy)
			clientAPIGroup.GET("/:hash/proxies/:pid/start", model_client.StartProxy)
			clientAPIGroup.GET("/:hash/proxies/:pid/stop", model_client.StopProxy)
			// Lib functions
			clientAPIGroup.POST("/:hash/lib/readdir", model_client.LibReadDir)
			clientAPIGroup.POST("/:hash/lib/stat", model_client.LibStat)
			clientAPIGroup.POST("/:hash/lib/readfile", model_client.LibReadFile)
			clientAPIGroup.POST("/:hash/lib/writefile", model_client.LibWriteFile)
			clientAPIGroup.POST("/:hash/lib/fopen", model_client.LibFopen)
			clientAPIGroup.POST("/:hash/lib/fseek", model_client.LibFseek)
			clientAPIGroup.POST("/:hash/lib/fread", model_client.LibFread)
			clientAPIGroup.POST("/:hash/lib/fwrite", model_client.LibFwrite)
			clientAPIGroup.POST("/:hash/lib/fclose", model_client.LibFclose)
			// Persistence
			clientAPIGroup.GET("/:hash/persistence/crontab/install", model_client.InstallCrontab)
			clientAPIGroup.GET("/:hash/persistence/sshkey/install", model_client.InstallSshKey)
			// Upgrade
			clientAPIGroup.POST("/:hash/upgrade/:target", model_client.UpgradeToTermite)
			// Delete
			clientAPIGroup.DELETE("/:hash", model_client.DeleteClient)
		}
		// Notification

	}

	wsNeedAuth := endpoint.Group("/ws")
	wsNeedAuth.Use(authMiddleware.MiddlewareFunc())
	{
		context.Ctx.NotifyWebSocket = websocket.CreateWebSocketServer()
		wsNeedAuth.GET("/notify", func(c *gin.Context) {
			context.Ctx.NotifyWebSocket.HandleRequest(c.Writer, c.Request)
		})

		// TTY
		ttyWebSocket := websocket.CreateTTYWebSocketServer()
		wsNeedAuth.GET("/tty/:hash", func(c *gin.Context) {
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
	}
	return endpoint
}
