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
	needAuth := endpoint.Group("/api/v1")
	needAuth.Use(authMiddleware.MiddlewareFunc())
	{
		// Refresh time can be longer than token timeout
		needAuth.GET("/refresh_token", authMiddleware.RefreshHandler)
		serverAPIGroup := needAuth.Group("/server")
		{
			serverAPIGroup.GET("", model_server.ListServers)
			serverAPIGroup.GET("/:hash", model_server.GetServerInfo)
			serverAPIGroup.GET("/:hash/client", model_server.GetServerClients)
			serverAPIGroup.POST("", model_server.CreateServer)
			serverAPIGroup.DELETE("/:hash", model_server.DeleteServer)
		}
		clientAPIGroup := needAuth.Group("/client")
		{
			// Client related
			clientAPIGroup.GET("", model_client.ListAllClients)
			clientAPIGroup.GET("/:hash", model_client.GetClientInfo)
			// Upgrade reverse shell client to termite client
			clientAPIGroup.GET("/:hash/upgrade/:target", model_client.UpgradeClient)
			clientAPIGroup.DELETE("/:hash", model_client.DeleteClient)
			clientAPIGroup.POST("/:hash", model_client.ExecuteCommand)
		}
		// Notification
		context.Ctx.NotifyWebSocket = websocket.CreateWebSocketServer()
		endpoint.GET("/notify", func(c *gin.Context) {
			context.Ctx.NotifyWebSocket.HandleRequest(c.Writer, c.Request)
		})

		ttyWebSocket := websocket.CreateTTYWebSocketServer()
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
	}
	return endpoint
}
