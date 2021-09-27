package restful

import (
	"io/ioutil"
	"time"

	model_client "github.com/WangYihang/Platypus/internal/model/client"
	model_misc "github.com/WangYihang/Platypus/internal/model/misc"
	model_server "github.com/WangYihang/Platypus/internal/model/server"
	"github.com/WangYihang/Platypus/internal/util/fs"
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

	// HTTP
	authMiddleware := web_jwt.Create()
	endpoint.POST("/login", authMiddleware.LoginHandler)
	// HTTP API
	apiNeedAuth := endpoint.Group("/api/v1")
	apiNeedAuth.Use(authMiddleware.MiddlewareFunc())
	{
		// Refresh time can be longer than token timeout
		apiNeedAuth.GET("/refresh_token", authMiddleware.RefreshHandler)
		runtimeAPIGroup := apiNeedAuth.Group("/runtime")
		{
			runtimeAPIGroup.GET("/cpu", model_misc.GetCpuUsage)
			runtimeAPIGroup.GET("/memory", model_misc.GetMemoryUsage)
			runtimeAPIGroup.GET("/gc", model_misc.GetGcUsage)
			runtimeAPIGroup.GET("/version", model_misc.GetVersion)
		}
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
	}

	// WebSocket
	wsNeedAuth := endpoint.Group("/ws")
	wsNeedAuth.Use(authMiddleware.MiddlewareFunc())
	{
		wsNeedAuth.GET("/notify", websocket.Notify)
		wsNeedAuth.GET("/tty/:hash", websocket.EstablishTTY)
	}
	return endpoint
}
