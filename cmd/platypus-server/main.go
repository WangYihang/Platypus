// Package main is the platypus-server entrypoint.
//
// @title           Platypus API
// @version         1.0
// @description     REST API for managing agent listeners, sessions, file transfer, and tunnels.
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
	"github.com/WangYihang/Platypus/internal/log"
	"github.com/WangYihang/Platypus/internal/mesh"
	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/utils/config"
	"github.com/WangYihang/Platypus/internal/utils/update"

	// Import the generated OpenAPI docs so `swag init`'s output is wired
	// into the binary. The swagger UI handler in internal/api looks up
	// docs by name ("swagger").
	_ "github.com/WangYihang/Platypus/docs"
)

const shutdownTimeout = 30 * time.Second

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

	servers := startHTTPServers(cfg)

	if cfg.Mesh.PSKFile != "" {
		node, err := mesh.NewNode(mesh.Config{
			IdentityDir:    cfg.Mesh.IdentityDir,
			PSKFile:        cfg.Mesh.PSKFile,
			ListenAddr:     cfg.Mesh.ListenAddr,
			AdvertiseAddrs: cfg.Mesh.AdvertiseAddrs,
			Peers:          cfg.Mesh.Peers,
			Role:           "server",
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
		log.Success("Mesh enabled: node_id=%s listen=%s", node.NodeID(), node.ListenerAddr())
	}

	for _, s := range cfg.Listeners {
		listener := core.CreateTCPServer(s.Host, s.Port, s.HashFormat, s.DisableHistory, s.PublicIP, s.ShellPath)
		if listener != nil {
			time.Sleep(0x100 * time.Millisecond)
			go (*listener).Run()
		}
	}

	log.L.Info("server_running")

	<-ctx.Done()
	log.Info("Shutdown signal received, draining connections...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	for _, srv := range servers {
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Error("server shutdown: %v", err)
		}
	}
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

func startHTTPServers(cfg *config.Config) []*http.Server {
	var servers []*http.Server

	dh := cfg.Distributor.Host
	dp := cfg.Distributor.Port
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
	distributor := core.CreateDistributorServer(core.DistributorArgs{
		Host:         dh,
		Port:         dp,
		Url:          cfg.Distributor.Url,
		Channel:      cfg.Distributor.ChannelOrDefault(),
		PresignedTTL: ttl,
		Store:        store,
	})
	distributorSrv := &http.Server{
		Addr:              fmt.Sprintf("%s:%d", dh, dp),
		Handler:           distributor,
		ReadHeaderTimeout: 10 * time.Second,
	}
	servers = append(servers, distributorSrv)

	go func() {
		if err := distributorSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("distributor: %v", err)
		}
	}()
	log.Success("Distributor at: http://%s:%d/", dh, dp)

	if cfg.RESTful.Enable {
		rh := cfg.RESTful.Host
		rp := cfg.RESTful.Port
		rest := api.CreateRESTfulAPIServer()

		// Open the persistent store used by users, projects, hosts, etc.
		// If it fails at startup there's nothing useful the server can do,
		// so bail loudly.
		dbFile := cfg.RESTful.DBFileOrDefault()
		db, err := storage.Open(dbFile)
		if err != nil {
			log.Error("open database %q: %v", dbFile, err)
			os.Exit(1)
		}
		core.Ctx.Storage = db
		log.Success("Storage: %s", dbFile)

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

		// Legacy Auth (shared secret + WS tickets) still guards the
		// existing v1 routes. RBAC + new auth handlers layer on top and
		// gate the new /auth/* + /users/* + /projects/* routes
		// introduced by the redesign.
		auth := api.NewAuth()
		auth.SetJWTFallback(tokens) // browsers use JWTs; legacy middleware needs to accept them on /ws/ticket
		authH := api.NewAuthHandler(db, tokens, auth.GetSecret())
		usersH := api.NewUsersHandler(db)
		projectsH := api.NewProjectsHandler(db)
		hostsH := api.NewHostsHandler(db)
		listenersH := api.NewListenersV2Handler(db, core.CoreLiveListeners{})
		sessionsH := api.NewSessionsV2Handler(db)
		enrollSvc := enrollment.New(db)
		patTokensH := api.NewPATTokensHandler(db, enrollSvc)
		// Install-artifact admin endpoints use the distributor's host:port
		// when rendering the curl command, so admins get a pasteable link
		// without having to know the topology themselves.
		distributorBase := fmt.Sprintf("http://%s:%d", cfg.Distributor.Host, cfg.Distributor.Port)
		installH := api.NewInstallTokensHandler(db, enrollSvc, distributorBase)
		agentSessionsH := api.NewAgentSessionsHandler(db)
		auditH := api.NewAuditHandler(db)
		rbac := api.NewRBACWithStorage(tokens, db)

		// Expose the enrollment service globally so the agent-facing TCP
		// handshake in internal/core can call it without plumbing an
		// extra parameter through CreateTCPServer.
		core.SetEnrollment(enrollSvc)

		api.RegisterWebSocketRoutes(rest, auth)
		api.RegisterV1Routes(rest, auth)
		api.RegisterV1AuthRoutes(rest, authH, usersH, rbac)
		api.RegisterV1ProjectsRoutes(rest, projectsH, rbac)
		api.RegisterV1HostsRoutes(rest, hostsH, rbac)
		api.RegisterV1ProjectListenersRoutes(rest, listenersH, rbac)
		api.RegisterV1ProjectSessionsRoutes(rest, sessionsH, rbac)
		api.RegisterV1PATTokenRoutes(rest, patTokensH, rbac)
		api.RegisterV1InstallTokenRoutes(rest, installH, rbac)
		api.RegisterV1AgentSessionsRoutes(rest, agentSessionsH, rbac)
		api.RegisterV1AuditRoutes(rest, auditH, rbac)
		api.RegisterSwaggerRoutes(rest)

		log.L.Info("api_ready",
			"bootstrap_secret", auth.GetSecret(),
			"bootstrap_url", fmt.Sprintf("http://%s:%d/api/v1/auth/bootstrap", rh, rp),
			"login_url", fmt.Sprintf("http://%s:%d/api/v1/auth/login", rh, rp),
			"token_url", fmt.Sprintf("http://%s:%d/api/v1/auth/token", rh, rp),
			"docs_url", fmt.Sprintf("http://%s:%d/swagger/index.html", rh, rp),
		)

		restSrv := &http.Server{
			Addr:              fmt.Sprintf("%s:%d", rh, rp),
			Handler:           rest,
			ReadHeaderTimeout: 10 * time.Second,
		}
		servers = append(servers, restSrv)

		go func() {
			if err := restSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Error("rest: %v", err)
			}
		}()
		log.Success("RESTful API at: http://%s:%d/api/v1/", rh, rp)
		core.Ctx.RESTful = rest
	}

	return servers
}
