// Package main is the platypus-server entrypoint.
//
// @title           Platypus API
// @version         1.0
// @description     REST API for managing agent sessions, file transfer, and tunnels.
// @description     Every endpoint except /api/v1/auth/token requires a Bearer token obtained via that endpoint.
// @BasePath        /
// @securityDefinitions.apikey BearerAuth
// @in   header
// @name Authorization
// @description Value should be "Bearer <token>". Fetch a token via POST /api/v1/auth/token using the secret printed at server startup.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/spf13/viper"

	"github.com/WangYihang/Platypus/internal/api"
	"github.com/WangYihang/Platypus/internal/app"
	"github.com/WangYihang/Platypus/internal/core"
	"github.com/WangYihang/Platypus/internal/core/artifact"
	"github.com/WangYihang/Platypus/internal/enrollment"
	"github.com/WangYihang/Platypus/internal/ingress"
	"github.com/WangYihang/Platypus/internal/log"
	"github.com/WangYihang/Platypus/internal/mesh"
	"github.com/WangYihang/Platypus/internal/pki"
	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/utils/config"
	"github.com/WangYihang/Platypus/internal/utils/update"

	// Import the generated OpenAPI docs so `swag init`'s output is wired
	// into the binary. The swagger UI handler in internal/api looks up
	// docs by name ("swagger").
	_ "github.com/WangYihang/Platypus/docs"
)

const shutdownTimeout = 30 * time.Second

const defaultIngressAddr = ":9443"

func main() {
	log.Init()

	cfg, configFile, err := loadConfig()
	if err != nil {
		log.Error("config: %v", err)
		os.Exit(1)
	}

	log.L.Info("server_starting", "version", update.Version, "config", configFile)

	core.Ctx = app.New(cfg)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Open the persistent store before anything else that needs it
	// (enrollment, PKI, install tokens). Distributor / REST / agent
	// all share the same handle.
	if !cfg.RESTful.Enable {
		log.Error("RESTful is now the only supported control-plane mode; set restful.enable=true in the config.")
		os.Exit(1)
	}
	dbFile := cfg.RESTful.DBFileOrDefault()
	db, err := storage.Open(dbFile)
	if err != nil {
		log.Error("open database %q: %v", dbFile, err)
		os.Exit(1)
	}
	core.Ctx.Storage = db
	log.Success("Storage: %s", dbFile)

	ingressAddr := cfg.Ingress.Addr
	if ingressAddr == "" {
		ingressAddr = defaultIngressAddr
	}
	publicAddr := cfg.Ingress.PublicAddr
	if publicAddr == "" {
		publicAddr = ingressAddr
	}
	api.PublicAddr = publicAddr

	// Mesh node (optional). When enabled it publishes PublicAddr so
	// peers can dial through the ingress dispatcher; inbound mesh
	// connections arrive via dispatcher → Node.AcceptRaw.
	var meshNode *mesh.Node
	if cfg.Mesh.PSKFile != "" {
		bootstrapTarget := cfg.Mesh.BootstrapTarget
		if bootstrapTarget == "" {
			bootstrapTarget = publicAddr
		}
		advertise := cfg.Mesh.AdvertiseAddrs
		if len(advertise) == 0 && publicAddr != "" {
			advertise = []string{publicAddr}
		}
		node, err := mesh.NewNode(mesh.Config{
			IdentityDir:       cfg.Mesh.IdentityDir,
			PSKFile:           cfg.Mesh.PSKFile,
			ListenAddr:        "", // listener is the unified ingress
			AdvertiseAddrs:    advertise,
			Peers:             cfg.Mesh.Peers,
			Role:              "server",
			DiscoveryLAN:      cfg.Mesh.DiscoveryLAN,
			DiscoveryInterval: cfg.Mesh.DiscoveryInterval,
			ProjectID:         cfg.Mesh.ProjectID,
			BootstrapEnabled:  bootstrapTarget != "",
			BootstrapTarget:   bootstrapTarget,
		}, nil)
		if err != nil {
			log.Error("mesh init failed: %v", err)
			os.Exit(1)
		}
		if err := node.Start(ctx); err != nil {
			log.Error("mesh start failed: %v", err)
			os.Exit(1)
		}
		core.Ctx.Mesh = node
		meshNode = node
		log.Success("Mesh enabled: node_id=%s advertise=%v", node.NodeID(), advertise)
	}

	tlsCfg, err := ingress.BuildTLSConfig(ingress.CertSource{
		CertFile: cfg.Ingress.Cert,
		KeyFile:  cfg.Ingress.Key,
	}, ingress.DefaultProtocols)
	if err != nil {
		log.Error("ingress: build tls config: %v", err)
		os.Exit(1)
	}

	agentSvc := core.NewAgentService(core.AgentServiceConfig{
		HashFormat:     cfg.Ingress.HashFormat,
		ShellPath:      cfg.Ingress.ShellPath,
		IngressAddr:    publicAddr,
		ProjectID:      cfg.Mesh.ProjectID,
		DisableHistory: cfg.Ingress.DisableHistory,
	})
	core.SetAgentService(agentSvc)

	dispatcher, err := ingress.New(ingress.Config{
		TLSConfig: tlsCfg,
		OnAgent:   agentSvc.Handle,
		OnMesh: func(conn net.Conn) {
			if meshNode == nil {
				_ = conn.Close()
				return
			}
			meshNode.AcceptRaw(ctx, conn)
		},
	})
	if err != nil {
		log.Error("ingress: configure dispatcher: %v", err)
		os.Exit(1)
	}

	listener, err := net.Listen("tcp", ingressAddr)
	if err != nil {
		log.Error("ingress: listen %s: %v", ingressAddr, err)
		os.Exit(1)
	}

	rest := buildRESTEngine(cfg, db)

	go func() {
		if err := dispatcher.Serve(ctx, listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("ingress: %v", err)
		}
	}()

	httpLn := dispatcher.HTTPListener(listener.Addr())
	httpSrv := &http.Server{
		Handler:           rest,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		if err := httpSrv.Serve(httpLn); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("rest: %v", err)
		}
	}()

	log.L.Info("ingress_ready",
		"addr", ingressAddr,
		"public_addr", publicAddr,
		"tls_cert", cfg.Ingress.Cert,
	)

	log.L.Info("server_running")

	api.RecordSystemActivity(context.Background(), api.ActivityInput{
		Category:    "server",
		Action:      "server.start",
		TargetType:  "server",
		TargetLabel: "platypus-server",
		Meta: map[string]any{
			"version":      update.Version,
			"config":       configFile,
			"ingress":      ingressAddr,
			"mesh_enabled": cfg.Mesh.PSKFile != "",
		},
	})

	<-ctx.Done()
	log.Info("Shutdown signal received, draining connections...")

	api.RecordSystemActivity(context.Background(), api.ActivityInput{
		Category:    "server",
		Action:      "server.stop",
		TargetType:  "server",
		TargetLabel: "platypus-server",
		Meta: map[string]any{
			"version": update.Version,
			"reason":  "signal",
		},
	})

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	_ = httpSrv.Shutdown(shutdownCtx)
	log.Success("Server stopped cleanly")
}

func loadConfig() (*config.Config, string, error) {
	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("yml")
	v.AddConfigPath(".")

	if err := v.ReadInConfig(); err != nil {
		return nil, "", fmt.Errorf("read config: %w", err)
	}

	var cfg config.Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, v.ConfigFileUsed(), fmt.Errorf("unmarshal config: %w", err)
	}

	if err := validator.New().Struct(&cfg); err != nil {
		return nil, v.ConfigFileUsed(), formatValidationError(err)
	}

	return &cfg, v.ConfigFileUsed(), nil
}

func formatValidationError(err error) error {
	var ve validator.ValidationErrors
	if !errors.As(err, &ve) {
		return err
	}
	msg := "config validation failed:"
	for _, fe := range ve {
		msg += fmt.Sprintf("\n  - %s: %s (got %v)", fe.Namespace(), fe.Tag(), fe.Value())
	}
	return errors.New(msg)
}

// mustRandomHex generates a random hex string of the given byte length.
// Panics if the OS entropy source fails — at that point the server can't
// safely do any crypto at all, so a loud crash is the right response.
func mustRandomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Errorf("crypto/rand: %w", err))
	}
	return hex.EncodeToString(b)
}

func buildRESTEngine(cfg *config.Config, db *storage.DB) http.Handler {
	rest := api.CreateRESTfulAPIServer()

	accessKey := cfg.RESTful.JWTAccessKey
	if accessKey == "" {
		accessKey = mustRandomHex(32)
		log.Info("No JWTAccessKey configured — generated a random one. Set RESTful.JWTAccessKey to keep tokens valid across restarts.")
	}
	refreshKey := cfg.RESTful.JWTRefreshKey
	if refreshKey == "" {
		refreshKey = mustRandomHex(32)
		log.Info("No JWTRefreshKey configured — generated a random one. Set RESTful.JWTRefreshKey to keep refresh tokens valid across restarts.")
	}
	accessTTL := time.Duration(cfg.RESTful.AccessTTLOrDefault()) * time.Second
	refreshTTL := time.Duration(cfg.RESTful.RefreshTTLOrDefault()) * time.Second
	tokens, err := api.NewTokenIssuer(accessKey, refreshKey, accessTTL, refreshTTL)
	if err != nil {
		log.Error("token issuer: %v", err)
		os.Exit(1)
	}

	auth := api.NewAuth()
	auth.SetJWTFallback(tokens)
	authH := api.NewAuthHandler(db, tokens, auth.GetSecret())
	usersH := api.NewUsersHandler(db)
	projectsH := api.NewProjectsHandler(db)
	hostsH := api.NewHostsHandler(db)
	sessionsH := api.NewSessionsV2Handler(db)

	pkiSvc := pki.New(db)
	enrollSvc := enrollment.New(db).WithPKI(pkiSvc)
	patTokensH := api.NewPATTokensHandler(db, enrollSvc)

	// /install/<token> and /v1/manifest/* now live on the same gin
	// engine — no dedicated distributor port. distributorBase is the
	// public HTTPS origin the server is reachable at so admin-minted
	// install links copy straight into `curl -k ... | sh`.
	distributorBase := "https://" + api.PublicAddr
	installH := api.NewInstallTokensHandler(db, enrollSvc, distributorBase)
	agentSessionsH := api.NewAgentSessionsHandler(db)
	activitiesH := api.NewActivitiesHandler(db)
	caH := api.NewCAHandler(db, pkiSvc)
	topologyH := api.NewTopologyHandler(db)
	rbac := api.NewRBACWithStorage(tokens, db)

	api.SetActivityRecorder(api.NewActivityRecorder(db))
	core.SetEnrollment(enrollSvc)

	store, err := artifact.NewS3Store(artifact.S3Config{
		Endpoint:        cfg.Distributor.Store.Endpoint,
		Region:          cfg.Distributor.Store.Region,
		Bucket:          cfg.Distributor.Store.Bucket,
		Prefix:          cfg.Distributor.Store.Prefix,
		AccessKeyID:     cfg.Distributor.Store.AccessKeyID,
		SecretAccessKey: cfg.Distributor.Store.SecretAccessKey,
		UseSSL:          cfg.Distributor.Store.UseSSL,
	})
	if err != nil {
		log.Error("distributor: init artifact store: %v", err)
		os.Exit(1)
	}
	ttl, err := time.ParseDuration(cfg.Distributor.PresignedTTL)
	if err != nil || ttl <= 0 {
		ttl = 5 * time.Minute
	}
	core.RegisterDistributorRoutes(rest, core.DistributorArgs{
		Url:          cfg.Distributor.Url,
		Channel:      cfg.Distributor.ChannelOrDefault(),
		PresignedTTL: ttl,
		Store:        store,
	})

	api.RegisterWebSocketRoutes(rest, auth)
	api.RegisterV1Routes(rest, auth)
	api.RegisterV1AuthRoutes(rest, authH, usersH, rbac)
	api.RegisterV1ProjectsRoutes(rest, projectsH, rbac)
	api.RegisterV1HostsRoutes(rest, hostsH, rbac)
	api.RegisterV1ProjectSessionsRoutes(rest, sessionsH, rbac)
	api.RegisterV1PATTokenRoutes(rest, patTokensH, rbac)
	api.RegisterV1InstallTokenRoutes(rest, installH, rbac)
	api.RegisterV1AgentSessionsRoutes(rest, agentSessionsH, rbac)
	api.RegisterV1ActivitiesRoutes(rest, activitiesH, rbac)
	api.RegisterV1CARoutes(rest, caH, rbac)
	api.RegisterV1TopologyRoutes(rest, topologyH, rbac)
	api.RegisterSwaggerRoutes(rest)

	core.StartTopologyStream()
	core.InstallTopologyObserver()

	log.L.Info("api_ready",
		"bootstrap_secret", auth.GetSecret(),
		"public_base", "https://"+api.PublicAddr,
	)

	core.Ctx.RESTful = rest
	return rest
}
