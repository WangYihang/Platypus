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
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/spf13/viper"

	"github.com/WangYihang/Platypus/internal/activity"
	"github.com/WangYihang/Platypus/internal/api"
	"github.com/WangYihang/Platypus/internal/app"
	"github.com/WangYihang/Platypus/internal/core"
	"github.com/WangYihang/Platypus/internal/core/artifact"
	"github.com/WangYihang/Platypus/internal/enrollment"
	"github.com/WangYihang/Platypus/internal/ingress"
	"github.com/WangYihang/Platypus/internal/log"
	"github.com/WangYihang/Platypus/internal/mesh"
	"github.com/WangYihang/Platypus/internal/optoken"
	"github.com/WangYihang/Platypus/internal/pki"
	"github.com/WangYihang/Platypus/internal/settings"
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
	dbFile := cfg.RESTful.DBFileOrDefault()
	db, err := storage.Open(dbFile)
	if err != nil {
		log.Error("open database %q: %v", dbFile, err)
		os.Exit(1)
	}
	core.Ctx.Storage = db
	log.Success("Storage: %s", dbFile)

	// KEK source policy:
	//   1. PLATYPUS_CA_KEK env var (production path — key never touches disk).
	//   2. PLATYPUS_DEV=1 enables a dev-only fallback at <data-dir>/ca.kek so
	//      `docker compose up` bootstraps cleanly without operator action.
	//   3. Otherwise: refuse to start. Falling back to a co-located key file
	//      in production defeats AES-GCM-sealing the CA private key — a
	//      compromise of the data volume yields both ciphertext and key.
	if os.Getenv(pki.KEKEnvVar) == "" {
		if os.Getenv("PLATYPUS_DEV") == "1" {
			pki.KEKPath = filepath.Join(filepath.Dir(dbFile), "ca.kek")
			log.L.Warn("dev_mode_kek_fallback_enabled",
				"path", pki.KEKPath,
				"hint", "set "+pki.KEKEnvVar+" to keep the CA key out of the data volume",
			)
		} else {
			log.Error("CA key-encryption-key missing: set %s to a 32-byte hex value, "+
				"or set PLATYPUS_DEV=1 to enable the on-disk dev fallback at <data-dir>/ca.kek. "+
				"Generate one with: openssl rand -hex 32", pki.KEKEnvVar)
			os.Exit(1)
		}
	}

	// Seed the "system" pseudo-user and the "default" project before
	// anything touches FKs that point at them. Mesh's server-self-issue
	// flow (tryStartServerMesh → pki.EnsureCA) writes project_ca rows
	// whose project_id + created_by_user FKs require both of these
	// rows to exist. Idempotent on every boot; the admin bootstrap
	// path in handler_auth_v1 still runs its own GetBySlug guard.
	systemUserID, err := storage.EnsureSystemUser(ctx, db)
	if err != nil {
		log.Error("seed system user: %v", err)
		os.Exit(1)
	}
	if _, err := storage.EnsureDefaultProject(ctx, db, systemUserID); err != nil {
		log.Error("seed default project: %v", err)
		os.Exit(1)
	}

	// Audit-tail close: stamp disconnected_at on any sessions row a
	// previous instance left open. This is NOT a presence-state
	// repair — live presence lives exclusively in
	// core.AgentLinkService and the sessions handler intersects
	// against it on every read. The sweep just stops historical
	// queries from seeing eternally-open audit windows for links the
	// previous server never got to close (SIGKILL, OOM, etc.).
	if n, err := db.Sessions().StampOpenAuditRowsClosed(ctx); err != nil {
		log.L.Warn("historical_session_audit_close_failed", "error", err.Error())
	} else if n > 0 {
		log.L.Info("historical_session_audit_close", "rows", n,
			"hint", "previous instance exited without graceful shutdown — audit-tail disconnected_at stamps are best-effort approximations of crash time")
	}

	ingressAddr := cfg.Ingress.Addr
	if ingressAddr == "" {
		ingressAddr = defaultIngressAddr
	}
	publicAddr := cfg.Ingress.PublicAddr
	if publicAddr == "" {
		publicAddr = ingressAddr
	}
	api.PublicAddr = publicAddr

	// pkiSvc is constructed here (rather than inside buildRESTEngine)
	// so the mesh bring-up below can self-issue the server's leaf
	// cert against the configured project CA. buildRESTEngine
	// receives the same instance.
	pkiSvc := pki.New(db)

	// settingsReg is the live policy-config layer. Built up front so
	// the mesh Node (below) and the REST engine can both attach to
	// the same instance — admin edits to mesh.discovery_lan /
	// discovery_interval_seconds, token TTLs, and distributor
	// channel / presigned_ttl take effect on the next hot-path read
	// without a restart.
	settingsReg := settings.New(db, cfg)

	// Mesh node (optional). The server self-issues a cert-bound
	// leaf under cfg.Mesh.ProjectID's CA — same chain agents in
	// that project use — and joins the overlay. On any wiring
	// failure mesh is skipped with an error log; server startup
	// never aborts over mesh.
	var meshNode *mesh.Node
	if cfg.Mesh.PSKFile != "" {
		if node := tryStartServerMesh(ctx, pkiSvc, cfg, publicAddr, settingsReg); node != nil {
			core.Ctx.Mesh = node
			meshNode = node
		}
	}

	// If no cert was configured, self-issue a leaf from the project CA
	// and use it for ingress TLS. This replaces the stand-alone
	// self-signed dev fallback — the old cert chained to nothing the
	// agent-side trust store knew, so agents with PLATYPUS_PROJECT_CA
	// pinned refused the handshake with a confusing "unknown
	// authority" error. Issuing the ingress leaf from the same CA
	// agents pin makes `docker compose up` work without mkcert.
	certSource := ingress.CertSource{
		CertFile: cfg.Ingress.Cert,
		KeyFile:  cfg.Ingress.Key,
	}
	if cfg.Ingress.Cert == "" && cfg.Ingress.Key == "" {
		if issued, err := issueIngressLeafFromProjectCA(ctx, pkiSvc, cfg); err != nil {
			log.L.Warn("ingress_tls_autoissue_failed",
				"error", err.Error(),
				"hint", "falling back to stand-alone self-signed cert; agents that pin PLATYPUS_PROJECT_CA will fail the handshake",
			)
		} else {
			certSource.InMemoryCert = issued
		}
	}
	tlsCfg, err := ingress.BuildTLSConfig(certSource, ingress.DefaultProtocols)
	if err != nil {
		log.Error("ingress: build tls config: %v", err)
		os.Exit(1)
	}

	// Accept client certificates when presented, but don't reject
	// connections that lack them — browsers and the REST API still
	// connect without a client cert. The v2 agent-link handler
	// validates the chain in-handler against the live project-CA
	// pool so revocations / rotations take effect without a restart.
	tlsCfg.ClientAuth = tls.RequestClientCert

	// v1 AgentService is gone; v2 agents reach us through the h2/http1
	// ALPN path (see Gin's /api/v1/agent/link handler). We keep the
	// ptps-mesh ALPN so mesh peers can still dial the same port.
	dispatcher, err := ingress.New(ingress.Config{
		TLSConfig: tlsCfg,
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

	rest := buildRESTEngine(ctx, cfg, db, pkiSvc, settingsReg)

	// Audit retention reaper: sweeps every hour, consults settings
	// for the live retention window. A zero window keeps everything
	// forever and the sweep is a no-op.
	go activity.NewReaper(db, settingsReg, log.L).Run(ctx)

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

func buildRESTEngine(ctx context.Context, cfg *config.Config, db *storage.DB, pkiSvc *pki.Service, settingsReg *settings.Registry) http.Handler {
	rest := api.CreateRESTfulAPIServer()

	auth := api.NewAuth()

	// optoken cache + verifier: the hot-path reader for every opaque
	// bearer the server accepts (pst_ user sessions, aat_ AI agent
	// tokens). 4096 entries × 30s TTL bounds memory and keeps the
	// missed-revoke damage window short. The verifier is the SOLE
	// authenticator after the JWT pair was retired in Phase 2.
	authTokenCache := optoken.NewCache(4096, 30*time.Second)
	tokenVerifier := api.NewTokenVerifier(db, authTokenCache)
	auth.SetOpaqueVerifier(tokenVerifier)

	authH := api.NewAuthHandler(db, tokenVerifier, auth.GetSecret())
	usersH := api.NewUsersHandler(db)
	projectsH := api.NewProjectsHandler(db)

	// Agent link service registers live v2 agents by agent_id; every
	// downstream handler that needs to reach an agent (sessionsH's
	// dispatch, terminal, file REST, RPC REST) looks it up here.
	agentLinkSvc := core.NewAgentLinkService()
	hostsH := api.NewHostsHandler(db).WithAgentLinks(agentLinkSvc)
	sessionsH := api.NewSessionsV2Handler(db, agentLinkSvc)

	enrollSvc := enrollment.New(db).WithPKI(pkiSvc).WithSettings(settingsReg)
	patTokensH := api.NewPATTokensHandler(db, enrollSvc)
	aatH := api.NewAATHandler(db, tokenVerifier)

	// /api/v1/install/<token> and /v1/manifest/* now live on the same
	// gin engine — no dedicated distributor port. distributorBase is
	// the public HTTPS origin the server is reachable at so
	// admin-minted install links copy straight into `curl -k ... | sh`.
	distributorBase := "https://" + api.PublicAddr
	installH := api.NewInstallTokensHandler(db, enrollSvc, distributorBase)
	enrollV2H := api.NewEnrollV2Handler(enrollSvc, pkiSvc).WithDB(db)

	// v2 agent link handler (yamux-over-WebSocket, mTLS-auth'd).
	// agentLinkSvc was constructed upstream because SessionsV2Handler
	// also depends on it for project-dispatch.
	agentLinkH := api.NewAgentLinkHandler(agentLinkSvc, api.ProjectsCAPool(db)).WithDB(db)
	activitiesH := api.NewActivitiesHandler(db)
	caH := api.NewCAHandler(db, pkiSvc)
	topologyH := api.NewTopologyHandler(db)
	rbac := api.NewRBAC(db, tokenVerifier)

	api.SetActivityRecorder(api.NewActivityRecorder(db))
	core.SetEnrollment(enrollSvc)

	// Distributor (manifest + installer) is optional: if no S3
	// endpoint is configured we skip it. Dev servers and pre-release
	// deployments can run the full admin surface without wiring an
	// object store first; operators who later configure one just add
	// the endpoint and restart.
	if cfg.Distributor.Store.Endpoint != "" {
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
		core.RegisterDistributorRoutes(rest, core.DistributorArgs{
			Settings: settingsReg,
			Store:    store,
		})
	} else {
		log.Info("Distributor disabled: configure distributor.store.endpoint to enable installer downloads.")
	}

	api.RegisterWebSocketRoutes(rest, auth)
	api.RegisterV1Routes(rest, auth)
	api.RegisterV1AuthRoutes(rest, authH, usersH, rbac)
	api.RegisterV1ProjectsRoutes(rest, projectsH, rbac)
	api.RegisterV1HostsRoutes(rest, hostsH, rbac)
	api.RegisterV1ProjectSessionsRoutes(rest, sessionsH, rbac)
	api.RegisterV1PATTokenRoutes(rest, patTokensH, rbac)
	api.RegisterV1AATRoutes(rest, aatH, rbac)
	api.RegisterV1InstallTokenRoutes(rest, installH, rbac)
	api.RegisterV2AgentEnrollRoute(rest, enrollV2H)
	api.RegisterV2AgentLinkRoute(rest, agentLinkH)
	api.RegisterV2TerminalRoute(rest, agentLinkSvc, rbac)
	api.RegisterV2FileRoutes(rest, agentLinkSvc, rbac)
	api.RegisterV2AgentRPCRoutes(rest, agentLinkSvc, rbac)
	api.RegisterV1ActivitiesRoutes(rest, activitiesH, rbac)
	api.RegisterV1CARoutes(rest, caH, rbac)
	api.RegisterV1TopologyRoutes(rest, topologyH, rbac)
	api.RegisterV1AdminSettingsRoutes(rest, api.NewAdminSettingsHandler(settingsReg), rbac)
	api.RegisterSwaggerRoutes(rest)

	// TopologyStream + observer were v1 — they relied on the
	// AgentClient map and legacy sysinfo cache. Rebuilt on top of
	// v2 in Phase IV (mesh hardening) which is the right layer for
	// topology telemetry anyway.

	// Point the info endpoint at our live agent link registry so
	// /api/v1/info's session_count reflects actual v2 connections.
	api.LiveAgentCounter = func() int { return len(agentLinkSvc.All()) }

	log.L.Info("api_ready",
		"bootstrap_secret", auth.GetSecret(),
		"public_base", "https://"+api.PublicAddr,
	)

	core.Ctx.RESTful = rest
	return rest
}

// tryStartServerMesh self-issues a cert-bound mesh identity for the
// server against the configured project CA and starts the mesh
// node. Any step failure is logged and returns nil — mesh is
// optional; we never abort server startup over it.
//
// issueIngressLeafFromProjectCA self-issues a server TLS leaf from
// the same project CA agents pin via PLATYPUS_PROJECT_CA. Returns
// nil, err if the project CA isn't available yet (e.g. KEK
// misconfigured) — the caller logs the error and falls back to the
// stand-alone self-signed leaf. Hosts for the SAN come from
// cfg.Ingress.PublicAddrOrAddr() (split off the port).
func issueIngressLeafFromProjectCA(ctx context.Context, pkiSvc *pki.Service, cfg *config.Config) (*tls.Certificate, error) {
	projectID := cfg.Mesh.ProjectID
	if projectID == "" {
		projectID = storage.DefaultProjectID
	}
	// EnsureCA first so the private bits exist before the IssueServerCert
	// path tries to unseal them. createdBy is the seeded system user.
	if _, err := pkiSvc.EnsureCA(ctx, projectID, storage.SystemUserID); err != nil {
		return nil, fmt.Errorf("ensure project CA: %w", err)
	}
	addr := cfg.Ingress.PublicAddrOrAddr()
	if addr == "" {
		return nil, errors.New("ingress.public_addr / ingress.addr both empty")
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr // no port — treat the whole string as the hostname
	}
	hosts := []string{host}
	// Always add localhost + loopback so the `curl -fsSL http://localhost:<port>/api/v1/install/...`
	// bootstrap path, and plain `curl https://127.0.0.1:<port>/...`, both
	// verify against the same leaf without the operator having to line up
	// hostnames with public_addr.
	for _, extra := range []string{"localhost", "127.0.0.1", "::1"} {
		if extra != host {
			hosts = append(hosts, extra)
		}
	}
	res, err := pkiSvc.IssueServerCert(ctx, projectID, hosts, storage.SystemUserID)
	if err != nil {
		return nil, err
	}
	leaf, err := tls.X509KeyPair([]byte(res.CertPEM), []byte(res.KeyPEM))
	if err != nil {
		return nil, fmt.Errorf("parse self-issued leaf: %w", err)
	}
	return &leaf, nil
}

// Project scoping: cfg.Mesh.ProjectID picks which project's CA to
// chain the server's leaf to. Agents in the same project will
// trust the resulting identity via the same CA.
func tryStartServerMesh(ctx context.Context, pkiSvc *pki.Service, cfg *config.Config, publicAddr string, settingsReg *settings.Registry) *mesh.Node {
	projectID := cfg.Mesh.ProjectID
	if projectID == "" {
		log.Error("mesh: cfg.Mesh.ProjectID is required for the server to self-issue a leaf cert; skipping")
		return nil
	}
	// Ensure (or create) the project CA, then self-issue a leaf
	// with SAN "platypus://agent/server". createdBy points at the
	// seeded system user so the project_ca.created_by_user FK resolves.
	if _, err := pkiSvc.EnsureCA(ctx, projectID, storage.SystemUserID); err != nil {
		log.Error("mesh: ensure project CA for %q: %v", projectID, err)
		return nil
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		log.Error("mesh: generate server identity key: %v", err)
		return nil
	}
	// issued_certs.issued_reason CHECK constraint (migration 000005)
	// accepts enroll | rotation | reissue | admin. "admin" is the
	// closest fit for a server-originated, non-enrollment self-issue
	// — the server is acting on its own authority to mint a mesh
	// identity. Use it rather than introducing a new enum value,
	// which would need a SQLite-flavour table-recreate migration.
	certPEM, caPEM, err := pkiSvc.IssueForAgent(ctx, projectID, "server", pub, "admin")
	if err != nil || certPEM == "" {
		log.Error("mesh: self-issue server leaf: %v", err)
		return nil
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		log.Error("mesh: marshal server key: %v", err)
		return nil
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	meshID, err := mesh.LoadIdentityFromCert([]byte(certPEM), keyPEM)
	if err != nil {
		log.Error("mesh: load self identity: %v", err)
		return nil
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM([]byte(caPEM)) {
		log.Error("mesh: parse project CA PEM")
		return nil
	}

	bootstrapTarget := cfg.Mesh.BootstrapTarget
	if bootstrapTarget == "" {
		bootstrapTarget = publicAddr
	}
	advertise := cfg.Mesh.AdvertiseAddrs
	if len(advertise) == 0 && publicAddr != "" {
		advertise = []string{publicAddr}
	}
	node, err := mesh.NewNode(mesh.Config{
		PSKFile:           cfg.Mesh.PSKFile,
		Identity:          meshID,
		TrustedCAs:        pool,
		ListenAddr:        "", // listener is the unified ingress
		AdvertiseAddrs:    advertise,
		Peers:             cfg.Mesh.Peers,
		Role:              "server",
		DiscoveryLAN:      cfg.Mesh.DiscoveryLAN,
		DiscoveryInterval: cfg.Mesh.DiscoveryInterval,
		ProjectID:         projectID,
		BootstrapEnabled:  bootstrapTarget != "",
		BootstrapTarget:   bootstrapTarget,
		Settings:          settingsReg,
	}, nil)
	if err != nil {
		log.Error("mesh: NewNode: %v", err)
		return nil
	}
	if err := node.Start(ctx); err != nil {
		log.Error("mesh: start: %v", err)
		return nil
	}
	log.Success("Mesh enabled: node_id=%s advertise=%v", node.NodeID(), advertise)
	return node
}
