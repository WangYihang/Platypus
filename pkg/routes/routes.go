package routes

import (
	"github.com/WangYihang/Platypus/pkg/controllers"
	"github.com/WangYihang/Platypus/pkg/middlewares"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// ConfigureRoutes sets up the Gin router with all necessary routes and middleware.
func ConfigureRoutes(r *gin.Engine, logger *zap.Logger, token string) {
	r.Use(gin.Recovery())
	r.Use(middlewares.RequestIDMiddleware())
	r.Use(middlewares.ZapLogger(logger))
	r.Use(middlewares.AuthMiddleware(token))

	apiGroup := r.Group("/api/v1")
	apiGroup.GET("/health", controllers.HealthCheckController)
	apiGroup.GET("/status", controllers.NewStatusController())
	apiGroup.GET("/version", controllers.VersionController)

	logsGroup := apiGroup.Group("/logs")
	{
		logsGroup.GET("", controllers.ListLogsController)
		logsGroup.GET("/:id", controllers.GetLogController)
		logsGroup.POST("", controllers.CreateLogController)
		logsGroup.PUT("/:id", controllers.UpdateLogController)
		logsGroup.DELETE("/:id", controllers.DeleteLogController)
	}

	listenersGroup := apiGroup.Group("/listeners")
	{
		listenersGroup.GET("", controllers.ListListenersController)
		listenersGroup.POST("", controllers.CreateListenerController)
		listenersGroup.GET("/:id", controllers.GetListenerController)
		listenersGroup.PUT("/:id", controllers.UpdateListenerController)
		listenersGroup.DELETE("/:id", controllers.DeleteListenerController)
	}

	binariesGroup := apiGroup.Group("/binaries")
	{
		binariesGroup.GET("", controllers.ListBinariesController)
		binariesGroup.POST("", controllers.CreateBinaryController)
		binariesGroup.GET("/:id", controllers.GetBinaryController)
		binariesGroup.PUT("/:id", controllers.UpdateBinaryController)
		binariesGroup.DELETE("/:id", controllers.DeleteBinaryController)
	}

	machinesGroup := apiGroup.Group("/machines")
	{
		machinesGroup.GET("", controllers.ListMachinesController)
		machinesGroup.POST("", controllers.CreateMachineController)
		machinesGroup.GET("/:id", controllers.GetMachineController)
		machinesGroup.PUT("/:id", controllers.UpdateMachineController)
		machinesGroup.DELETE("/:id", controllers.DeleteMachineController)

		machineFilesystemGroup := machinesGroup.Group("/:id/filesystem")
		{
			machineFilesystemGroup.GET("", controllers.ListFilesystemController)
			machineFilesystemGroup.GET("/:id", controllers.GetFilesystemController)
			machineFilesystemGroup.POST("", controllers.CreateFilesystemController)
			machineFilesystemGroup.PUT("/:id", controllers.UpdateFilesystemController)
			machineFilesystemGroup.DELETE("/:id", controllers.DeleteFilesystemController)
		}

		machineTunnelsGroup := machinesGroup.Group("/:id/tunnels")
		{
			machineTunnelsGroup.GET("", controllers.ListTunnelsController)
			machineTunnelsGroup.GET("/:id", controllers.GetTunnelController)
			machineTunnelsGroup.POST("", controllers.CreateTunnelController)
			machineTunnelsGroup.PUT("/:id", controllers.UpdateTunnelController)
			machineTunnelsGroup.DELETE("/:id", controllers.DeleteTunnelController)
		}

		machineSessionsGroup := machinesGroup.Group("/:id/sessions")
		{
			machineSessionsGroup.GET("", controllers.ListSessionsController)
			machineSessionsGroup.GET("/:id", controllers.GetSessionController)
			machineSessionsGroup.POST("", controllers.CreateSessionController)
			machineSessionsGroup.PUT("/:id", controllers.UpdateSessionController)
			machineSessionsGroup.DELETE("/:id", controllers.DeleteSessionController)
		}

		recordsGroup := machinesGroup.Group("/:id/records")
		{
			recordsGroup.GET("", controllers.ListRecordsController)
			recordsGroup.GET("/:id", controllers.GetRecordController)
			recordsGroup.POST("", controllers.CreateRecordController)
			recordsGroup.PUT("/:id", controllers.UpdateRecordController)
			recordsGroup.DELETE("/:id", controllers.DeleteRecordController)
		}
	}
}
