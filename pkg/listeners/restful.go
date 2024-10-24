package listeners

import (
	"fmt"

	"github.com/WangYihang/Platypus/pkg/routes"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// RESTfulListener represents a RESTful listener.
type RESTfulListener struct {
	commonListener
	Token string `json:"token" yaml:"token" toml:"token"`
}

// NewRESTfulListener creates a new RESTful listener.
func NewRESTfulListener(host string, port uint16, token string) *RESTfulListener {
	return &RESTfulListener{
		commonListener: commonListener{
			BindHost: host,
			BindPort: port,
		},
		Token: token,
	}
}

// Start starts the RESTful listener.
func (l *RESTfulListener) Start(logger *zap.Logger) {
	logger.Info("starting RESTful listener", zap.String("host", l.BindHost), zap.Uint16("port", l.BindPort))
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()
	logger.Info("configuring routes with token", zap.String("token", l.Token))
	routes.ConfigureRoutes(r, logger, l.Token)
	err := r.Run(fmt.Sprintf("%s:%d", l.BindHost, l.BindPort))
	if err != nil {
		logger.Error("failed to start RESTful listener", zap.Error(err))
	}
}
