package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/WangYihang/Platypus/pkg/config"
	"github.com/WangYihang/Platypus/pkg/dependencies"
	"github.com/WangYihang/Platypus/pkg/models"
	"github.com/WangYihang/Platypus/pkg/options"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

func main() {
	// Parse command line options
	opts, err := options.InitServerOptions()
	if err != nil {
		slog.Debug("error occured while parsing options", slog.String("error", err.Error()))
		os.Exit(1)
	}

	// Load configuration
	cfg, err := dependencies.InitConfig(opts.ConfigFile)()
	if err != nil {
		slog.Error("error occurred while initializing config", slog.String("error", err.Error()))
		os.Exit(1)
	}

	// Initialize the application
	app := fx.New(
		fx.Provide(
			func() *config.Config { return cfg },
			dependencies.InitLogger(models.Development),
		),
		fx.Invoke(
			func(lc fx.Lifecycle, cfg *config.Config, logger *zap.Logger) {
				lc.Append(fx.Hook{
					OnStart: func(context.Context) error {
						logger.Info("config loaded", zap.Any("config", cfg))
						logger.Info("starting plain listeners")
						for _, listener := range cfg.PlainListeners {
							go listener.Start(logger)
						}
						logger.Info("starting encrypted listeners")
						for _, listener := range cfg.EncryptedListeners {
							go listener.Start(logger)
						}
						logger.Info("starting RESTful listeners")
						for _, listener := range cfg.RestfulListeners {
							go listener.Start(logger)
						}
						return nil
					},
					OnStop: func(context.Context) error {
						return nil
					},
				})
			},
		),
	)

	// Run the application
	app.Run()
}
