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

	// v2 connect loop: BootstrapV2 (enroll or load identity + dial) →
	// ServeLink (accept streams, dispatch to per-type handlers) →
	// return on any error so backoff retries. meshProjectID is
	// retained as a local for when Phase IV rebuilds mesh on top
	// of the v2 link primitives; nothing else in the v2 path
	// touches it yet.
	_ = meshProjectID

	caPool, err := agent.LoadProjectCA(os.Getenv(agent.ProjectCAEnvVar))
	if err != nil {
		logger.Error("parse project CA env var", slog.String("error", err.Error()))
		os.Exit(1)
	}

	hostname, _ := os.Hostname()

	connect := func() error {
		if ctx.Err() != nil {
			return backoff.Permanent(ctx.Err())
		}
		logger.Info("agent connect (v2)", slog.String("endpoint", endpoint))

		sess, err := agent.BootstrapV2(ctx, agent.BootstrapV2Options{
			IdentityDir:  identityDir,
			ServerURL:    fmt.Sprintf("wss://%s/api/v1/agent/link", endpoint),
			EnrollURL:    fmt.Sprintf("https://%s", endpoint),
			PAT:          opts.Token,
			ProjectCA:    caPool,
			Hostname:     hostname,
			AgentVersion: "v2",
		})
		if err != nil {
			return err
		}
		defer sess.Close()

		logger.Info("v2 link established; serving streams")
		return agent.ServeLink(ctx, sess, agent.AgentHandlerDeps{
			RPC: agent.AgentRPCHandlers{
				Exec:    agent.HandleExec,
				ListDir: agent.HandleListDir,
				Stat:    agent.HandleStat,
				Delete:  agent.HandleDelete,
				Rename:  agent.HandleRename,
				Mkdir:   agent.HandleMkdir,
				Chmod:   agent.HandleChmod,
				SysInfo: agent.HandleSysInfo,
			},
			Process:    agent.HandleProcessStream,
			FileRead:   agent.HandleFileReadStream,
			FileWrite:  agent.HandleFileWriteStream,
			TunnelPull: agent.HandleTunnelPullStream,
		})
	}

	if err := backoff.Retry(connect, bo); err != nil {
		logger.Error("connection loop terminated", slog.String("error", err.Error()))
		os.Exit(1)
	}
	_ = state // kept for when mesh Phase IV wires v2 back into agent.State
	logger.Info("agent stopped")
}
