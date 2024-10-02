package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/WangYihang/Platypus/pkg/config"
	"github.com/WangYihang/Platypus/pkg/dependencies"
	"github.com/WangYihang/Platypus/pkg/listeners"
	"github.com/WangYihang/Platypus/pkg/models"
	"github.com/WangYihang/Platypus/pkg/options"
	"github.com/google/uuid"
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
						for _, l := range cfg.Listeners {
							switch l.Type {
							case listeners.ListenerTypePlainShell:
								go listeners.NewPlainShellListener(l.BindHost, l.BindPort).Start(logger)
							case listeners.ListenerTypeEncryptedShell:
								go listeners.NewEncryptedShellListener(l.BindHost, l.BindPort).Start(logger)
							case listeners.ListenerTypeRESTful:
								token := uuid.New().String()
								logger.Info("using generated uuid as token", zap.String("token", token))
								go listeners.NewRESTfulListener(l.BindHost, l.BindPort, token).Start(logger)
							default:
								logger.Error("unsupported listener type", zap.String("type", string(l.Type)))
								return fmt.Errorf("unsupported listener type: %s", l.Type)
							}
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
