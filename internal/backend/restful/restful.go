package restful

import (
	"fmt"
	"net/http"
	"time"

	"github.com/WangYihang/Platypus/internal/backend/jwt"
	agent_controller "github.com/WangYihang/Platypus/internal/controllers/agent"
	binary_controller "github.com/WangYihang/Platypus/internal/controllers/binary"
	listener_controller "github.com/WangYihang/Platypus/internal/controllers/listener"
	record_controller "github.com/WangYihang/Platypus/internal/controllers/record"
	runtime_controller "github.com/WangYihang/Platypus/internal/controllers/runtime"
	template_controller "github.com/WangYihang/Platypus/internal/controllers/template"
	user_controller "github.com/WangYihang/Platypus/internal/controllers/user"
	"github.com/WangYihang/Platypus/internal/utils/config"
	sentrygin "github.com/getsentry/sentry-go/gin"
	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"
	swaggerfiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

func StartEndpoint() {
	// Create RESTful Server
	endpoint := CreateEndpoint()
	// Start RESTful Server
	cfg := config.LoadConfig()
	address := fmt.Sprintf("%s:%d", cfg.RESTful.Host, cfg.RESTful.Port)
	fmt.Printf("RESTful Server listening on %s\n", address)
	endpoint.Run(address)
}

func CreateEndpoint() *gin.Engine {
	// gin.SetMode(gin.ReleaseMode)
	// gin.DefaultWriter = ioutil.Discard
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

	// Sentry stuff
	endpoint.Use(sentrygin.New(sentrygin.Options{
		Repanic: true,
	}))

	endpoint.Use(func(ctx *gin.Context) {
		if hub := sentrygin.GetHubFromContext(ctx); hub != nil {
			hub.Scope().SetTag("someRandomTag", "maybeYouNeedIt")
		}
		ctx.Next()
	})

	// HTTP
	authMiddleware := jwt.Create()
	endpoint.POST("/login", authMiddleware.LoginHandler)

	endpoint.Use(static.Serve("/", static.LocalFile("web/cdnmon-frontend/dist", false)))
	endpoint.Use(static.Serve("/ttyd", static.LocalFile("web/ttyd/html/dist", false)))

	// Swagger UI
	endpoint.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerfiles.Handler))

	endpoint.GET("/panic", func(ctx *gin.Context) {
		// sentrygin handler will catch it just fine, and because we attached "someRandomTag"
		// in the middleware before, it will be sent through as well
		panic("y tho")
	})

	// HTTP API
	apiNeedAuth := endpoint.Group("/api/v1")
	apiNeedAuth.Use(authMiddleware.MiddlewareFunc())
	{
		userAPIGroup := apiNeedAuth.Group("/user")
		{
			userAPIGroup.GET("/me", user_controller.GetCurrentUser)
		}

		runtimeAPIGroup := apiNeedAuth.Group("/runtime")
		{
			runtimeAPIGroup.GET("/cpu", runtime_controller.GetCpuState)
			runtimeAPIGroup.GET("/memory", runtime_controller.GetMemoryState)
			runtimeAPIGroup.GET("/gc", runtime_controller.GetGcState)
			runtimeAPIGroup.GET("/version", runtime_controller.GetVersion)
		}

		tokenAPIGroup := apiNeedAuth.Group("/token")
		{
			// Refresh time can be longer than token timeout
			tokenAPIGroup.GET("/refresh", authMiddleware.RefreshHandler)
		}

		listenerAPIGroup := apiNeedAuth.Group("/listener")
		{
			listenerAPIGroup.GET("", listener_controller.GetAllListeners)
			listenerAPIGroup.POST("", listener_controller.CreateListener)
			listenerAPIGroup.GET("/:id", listener_controller.GetListener)
			listenerAPIGroup.GET("/:id/enable", listener_controller.EnableListener)
			listenerAPIGroup.GET("/:id/disable", listener_controller.DisableListener)
			listenerAPIGroup.GET("/:id/agent", listener_controller.GetAllAgentsOfListener)
			listenerAPIGroup.DELETE("/:id", listener_controller.DeleteListener)
		}

		templateAPIGroup := apiNeedAuth.Group("/template")
		{
			templateAPIGroup.GET("", template_controller.GetAllTemplates)
			templateAPIGroup.GET("/:id", template_controller.GetTemplate)
		}

		binaryAPIGroup := apiNeedAuth.Group("/binary")
		{
			binaryAPIGroup.GET("", binary_controller.GetAllBinaries)
			binaryAPIGroup.GET("/:id", binary_controller.GetBinary)
			binaryAPIGroup.GET("/:id/raw", binary_controller.RawBinary)
			binaryAPIGroup.POST("", binary_controller.CreateBinary)
		}

		agentAPIGroup := apiNeedAuth.Group("/agent")
		{
			agentAPIGroup.GET("", agent_controller.GetAllAgents)
			// Basic operations
			agentAPIGroup.GET("/:id", agent_controller.GetAgent)
			agentAPIGroup.GET("/:id/refresh", agent_controller.CollectAgentInfo)
			// Proxies
			agentAPIGroup.GET("/:id/proxy", agent_controller.GetAllProxies)
			agentAPIGroup.POST("/:id/proxy", agent_controller.CreateProxy)
			agentAPIGroup.DELETE("/:id/proxy", agent_controller.DeleteProxy)
			agentAPIGroup.GET("/:id/proxy/:pid/start", agent_controller.StartProxy)
			agentAPIGroup.GET("/:id/proxy/:pid/stop", agent_controller.StopProxy)
			// Directory operate functions
			agentAPIGroup.POST("/:id/lib/readdir", agent_controller.LibReadDir)
			agentAPIGroup.POST("/:id/lib/stat", agent_controller.LibStat)
			// IO functions
			agentAPIGroup.POST("/:id/lib/readfile", agent_controller.LibReadFile)
			agentAPIGroup.POST("/:id/lib/writefile", agent_controller.LibWriteFile)
			agentAPIGroup.POST("/:id/lib/fopen", agent_controller.LibFopen)
			agentAPIGroup.POST("/:id/lib/fseek", agent_controller.LibFseek)
			agentAPIGroup.POST("/:id/lib/fread", agent_controller.LibFread)
			agentAPIGroup.POST("/:id/lib/fwrite", agent_controller.LibFwrite)
			agentAPIGroup.POST("/:id/lib/fclose", agent_controller.LibFclose)
			// Persistence
			agentAPIGroup.GET("/:id/persistence/crontab/install", agent_controller.InstallCrontab)
			agentAPIGroup.GET("/:id/persistence/crontab/uninstall", agent_controller.InstallCrontab)
			agentAPIGroup.GET("/:id/persistence/sshkey/install", agent_controller.InstallSshKey)
			agentAPIGroup.GET("/:id/persistence/sshkey/uninstall", agent_controller.InstallCrontab)
			// Upgrade
			agentAPIGroup.POST("/:id/upgrade/meterpreter/:target", agent_controller.UpgradeToTermite)
			agentAPIGroup.POST("/:id/upgrade/cobaltstrike/:target", agent_controller.UpgradeToTermite)
			agentAPIGroup.POST("/:id/upgrade/termite_tcp/:target", agent_controller.UpgradeToTermite)
			agentAPIGroup.POST("/:id/upgrade/termite_udp/:target", agent_controller.UpgradeToTermite)
			agentAPIGroup.POST("/:id/upgrade/termite_icmp/:target", agent_controller.UpgradeToTermite)
			agentAPIGroup.POST("/:id/upgrade/termite_dns/:target", agent_controller.UpgradeToTermite)
			// Delete
			agentAPIGroup.DELETE("/:id", agent_controller.DeleteClient)
			// Interactive TTY
			agentAPIGroup.GET("/:id/tty", agent_controller.SpawnWebsocketTTY)
		}

		recordAPIGroup := apiNeedAuth.Group("/record")
		{
			recordAPIGroup.GET("", record_controller.GetAllRecords)
			recordAPIGroup.GET("/:id", record_controller.GetRecord)
			recordAPIGroup.GET("/:id/raw", record_controller.RawRecord)
		}
		// apiNeedAuth.GET("/ws", websocket.Notify)
	}

	endpoint.NoRoute(func(c *gin.Context) {
		c.Redirect(http.StatusTemporaryRedirect, "/")
	})

	return endpoint
}
