package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cenkalti/backoff/v4"

	"github.com/WangYihang/Platypus/internal/agent"
	"github.com/WangYihang/Platypus/internal/mesh"
	"github.com/WangYihang/Platypus/pkg/options"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	slog.SetDefault(logger)

	opts, err := options.InitOptions()
	if err != nil {
		logger.Error("parse options", slog.String("error", err.Error()))
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	state := agent.Init()
	endpoint := fmt.Sprintf("%s:%d", opts.RemoteHost, opts.RemotePort)
	logger.Info("starting agent",
		slog.String("endpoint", endpoint),
		slog.String("token", opts.Token),
	)

	// Mesh overlay is opt-in: enable only when the operator provides a PSK
	// file. This preserves the legacy hub-and-spoke behaviour for agents
	// that have not yet been migrated.
	if opts.MeshPSKFile != "" {
		cfg := mesh.Config{
			IdentityDir:    opts.MeshIdentityDir,
			PSKFile:        opts.MeshPSKFile,
			ListenAddr:     opts.MeshListen,
			Peers:          opts.MeshPeers,
			AdvertiseAddrs: opts.MeshAdvertise,
			Role:           "agent",
		}
		node, err := mesh.NewNode(cfg, logger)
		if err != nil {
			logger.Error("mesh init", slog.String("error", err.Error()))
			os.Exit(1)
		}
		agent.AttachMesh(state, node)
		if err := node.Start(ctx); err != nil {
			logger.Error("mesh start", slog.String("error", err.Error()))
			os.Exit(1)
		}
		logger.Info("mesh enabled",
			slog.String("node_id", node.NodeID()),
			slog.String("listen", node.ListenerAddr()))
	}

	bo := backoff.WithContext(
		backoff.NewExponentialBackOff(
			backoff.WithMaxInterval(1*time.Minute),
			backoff.WithMaxElapsedTime(0),
		),
		ctx,
	)

	connect := func() error {
		if ctx.Err() != nil {
			return backoff.Permanent(ctx.Err())
		}
		logger.Info("connecting to server", slog.String("endpoint", endpoint))
		return agent.Connect(endpoint, opts.Token, state)
	}

	if err := backoff.Retry(connect, bo); err != nil {
		logger.Error("connection loop terminated", slog.String("error", err.Error()))
		os.Exit(1)
	}
	logger.Info("agent stopped")
}
