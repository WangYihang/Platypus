package dependencies

import (
	"fmt"

	"github.com/WangYihang/Platypus/pkg/models"
	"go.uber.org/zap"
)

// InitLogger initializes the logger based on the environment
func InitLogger(env models.Environment) func() (*zap.Logger, error) {
	return func() (*zap.Logger, error) {
		switch env {
		case models.Development:
			return zap.NewDevelopment()
		case models.Staging:
			return zap.NewDevelopment()
		case models.Production:
			return zap.NewProduction()
		default:
			return nil, fmt.Errorf("unsupported environment: %s", env)
		}
	}
}
