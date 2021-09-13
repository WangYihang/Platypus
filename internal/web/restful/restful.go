package restful

import (
	"io/ioutil"
	"time"

	"github.com/WangYihang/Platypus/internal/context"
	client_model "github.com/WangYihang/Platypus/internal/model/client"
	server_model "github.com/WangYihang/Platypus/internal/model/server"
	"github.com/WangYihang/Platypus/internal/util/fs"
	"github.com/WangYihang/Platypus/internal/util/log"
	"github.com/WangYihang/Platypus/internal/util/validator"
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

	context.Ctx.NotifyWebSocket = websocket.CreateWebSocketServer()
	endpoint.GET("/notify", func(c *gin.Context) {
		context.Ctx.NotifyWebSocket.HandleRequest(c.Writer, c.Request)
	})

	ttyWebSocket := websocket.CreateTermiteWebSocketServer()
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

	// Static files
	endpoint.Use(static.Serve("/", fs.BinaryFileSystem("./web/frontend/build")))
	// WebSocket TTYd
	endpoint.Use(static.Serve("/shell/", fs.BinaryFileSystem("./web/ttyd/dist")))

	// TODO: Websocket UI Auth (to be implemented)
	endpoint.GET("/token", func(c *gin.Context) {
		c.String(200, "")
	})

	// Server related
	// Simple group: v1
	RESTfulAPIGroup := endpoint.Group("/api/v1")
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
