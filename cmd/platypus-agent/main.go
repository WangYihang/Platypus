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
	identityDir := agent.ResolveIdentityDir(opts.IdentityDir)
	meshPSKFile := opts.MeshPSKFile
	meshPeers := append([]string(nil), opts.MeshPeers...)
	meshProjectID := opts.MeshProjectID
	if persisted, err := agent.LoadPersistedMeshBootstrap(identityDir); err != nil {
		logger.Warn("load persisted mesh bootstrap", slog.String("error", err.Error()))
	} else if persisted != nil {
		if meshPSKFile == "" {
			meshPSKFile = persisted.PSKFile
		}
		if meshProjectID == "" {
			meshProjectID = persisted.ProjectID
		}
		if len(meshPeers) == 0 {
			meshPeers = append(meshPeers, persisted.Peers...)
		}
	}
	endpoint := fmt.Sprintf("%s:%d", opts.RemoteHost, opts.RemotePort)
	logger.Info("starting agent",
		slog.String("endpoint", endpoint),
		slog.String("token", opts.Token),
	)

	// Mesh overlay is opt-in: enable only when the operator provides a PSK
	// file. This preserves the legacy hub-and-spoke behaviour for agents
	// that have not yet been migrated.
	if meshPSKFile != "" {
		cfg := mesh.Config{
			IdentityDir:       agent.MeshStateDir(identityDir),
			PSKFile:           meshPSKFile,
			ListenAddr:        opts.MeshListen,
			Peers:             meshPeers,
			AdvertiseAddrs:    opts.MeshAdvertise,
			Role:              "agent",
			DiscoveryLAN:      opts.MeshDiscoveryLAN,
			DiscoveryInterval: opts.MeshDiscoveryInterval,
			ProjectID:         meshProjectID,
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
		return agent.ConnectWithOptions(endpoint, opts.Token, state, &agent.ConnectOptions{
			IdentityDir:   identityDir,
			MeshProjectID: meshProjectID,
		})
	}

	if err := backoff.Retry(connect, bo); err != nil {
		logger.Error("connection loop terminated", slog.String("error", err.Error()))
		os.Exit(1)
	}
	logger.Info("agent stopped")
}
