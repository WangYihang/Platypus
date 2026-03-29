package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/WangYihang/Platypus/internal/agent"
	"github.com/WangYihang/Platypus/pkg/dependencies"
	"github.com/WangYihang/Platypus/pkg/models"
	"github.com/WangYihang/Platypus/pkg/options"
	"github.com/WangYihang/Platypus/pkg/utils"
	"github.com/cenkalti/backoff/v4"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

func onStart(ctx context.Context, opts *options.Options, logger *zap.Logger, state *agent.State) error {
	logger.Info("starting application", zap.String("host", opts.RemoteHost), zap.Int("port", opts.RemotePort), zap.String("token", opts.Token), zap.String("env", opts.Environment))
	if opts.Environment == string(models.Production) {
		utils.StartDaemonMode(logger, nil)
	}
	return nil
}

func onStop(ctx context.Context, opts *options.Options, logger *zap.Logger) error {
	logger.Info("Stopping application...")
	return nil
}

func main() {
	opts, err := options.InitOptions()
	if err != nil {
		slog.Debug("error occured while parsing options", slog.String("error", err.Error()))
		os.Exit(1)
	}

	state := agent.Init()

	app := fx.New(
		fx.Provide(
			dependencies.InitLogger(models.Development),
		),
		fx.Invoke(
			func(lc fx.Lifecycle, logger *zap.Logger) {
				lc.Append(fx.Hook{
					OnStart: func(context.Context) error {
						return onStart(context.Background(), opts, logger, state)
					},
					OnStop: func(context.Context) error {
						return onStop(context.Background(), opts, logger)
					},
				})
			},
			func(logger *zap.Logger) {
				logger.Info("starting application")
				endpoint := fmt.Sprintf("%s:%d", opts.RemoteHost, opts.RemotePort)
				operation := func() error {
					logger.Info("connecting to server", zap.String("endpoint", endpoint))
					return agent.Connect(endpoint, opts.Token, state)
				}
				err := backoff.Retry(operation, backoff.NewExponentialBackOff(
					backoff.WithMaxInterval(1*time.Minute),
					backoff.WithMaxElapsedTime(0),
				))
				if err != nil {
					logger.Error("connect to server failed", zap.String("error", err.Error()))
				}
			},
		),
	)
	app.Run()
}
