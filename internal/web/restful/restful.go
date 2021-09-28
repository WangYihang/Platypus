package restful

import (
	"io/ioutil"
	"time"

	client_controller "github.com/WangYihang/Platypus/internal/controller/client"
	misc_controller "github.com/WangYihang/Platypus/internal/controller/misc"
	runtime_controller "github.com/WangYihang/Platypus/internal/controller/runtime"
	server_controller "github.com/WangYihang/Platypus/internal/controller/server"
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
			runtimeAPIGroup.GET("/cpu", runtime_controller.GetCpuState)
			runtimeAPIGroup.GET("/memory", runtime_controller.GetMemoryState)
			runtimeAPIGroup.GET("/gc", runtime_controller.GetGcState)
			runtimeAPIGroup.GET("/version", runtime_controller.GetVersion)
		}
		serverAPIGroup := apiNeedAuth.Group("/servers")
		{
			serverAPIGroup.GET("", server_controller.GetAllServers)
			serverAPIGroup.POST("", server_controller.CreateServer)
			serverAPIGroup.GET("/:hash", server_controller.GetServer)
			serverAPIGroup.GET("/:hash/start", server_controller.StartServer)
			serverAPIGroup.GET("/:hash/stop", server_controller.StopServer)
			serverAPIGroup.GET("/:hash/clients", server_controller.GetAllClientsOfServer)
			serverAPIGroup.DELETE("/:hash", server_controller.DeleteServer)
		}
		clientAPIGroup := apiNeedAuth.Group("/clients")
		{
			clientAPIGroup.GET("", client_controller.GetAllClients)
			// Basic operations
			clientAPIGroup.GET("/:hash", client_controller.GetClient)
			clientAPIGroup.GET("/:hash/collect", client_controller.CollectClientInfo)
			// Proxies
			clientAPIGroup.GET("/:hash/proxies", client_controller.GetAllProxies)
			clientAPIGroup.POST("/:hash/proxies", client_controller.CreateProxy)
			clientAPIGroup.DELETE("/:hash/proxies", client_controller.DeleteProxy)
			clientAPIGroup.GET("/:hash/proxies/:pid/start", client_controller.StartProxy)
			clientAPIGroup.GET("/:hash/proxies/:pid/stop", client_controller.StopProxy)
			// Lib functions
			clientAPIGroup.POST("/:hash/lib/readdir", client_controller.LibReadDir)
			clientAPIGroup.POST("/:hash/lib/stat", client_controller.LibStat)
			clientAPIGroup.POST("/:hash/lib/readfile", client_controller.LibReadFile)
			clientAPIGroup.POST("/:hash/lib/writefile", client_controller.LibWriteFile)
			clientAPIGroup.POST("/:hash/lib/fopen", client_controller.LibFopen)
			clientAPIGroup.POST("/:hash/lib/fseek", client_controller.LibFseek)
			clientAPIGroup.POST("/:hash/lib/fread", client_controller.LibFread)
			clientAPIGroup.POST("/:hash/lib/fwrite", client_controller.LibFwrite)
			clientAPIGroup.POST("/:hash/lib/fclose", client_controller.LibFclose)
			// Persistence
			clientAPIGroup.GET("/:hash/persistence/crontab/install", client_controller.InstallCrontab)
			clientAPIGroup.GET("/:hash/persistence/sshkey/install", client_controller.InstallSshKey)
			// Upgrade
			clientAPIGroup.POST("/:hash/upgrade/:target", client_controller.UpgradeToTermite)
			// Delete
			clientAPIGroup.DELETE("/:hash", client_controller.DeleteClient)
		}
		// Termite compiler
		apiNeedAuth.POST("/compile", misc_controller.CompileHandler)
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
