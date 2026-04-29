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

	"github.com/alecthomas/kong"

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
	"github.com/WangYihang/Platypus/internal/recording"
	"github.com/WangYihang/Platypus/internal/settings"
	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/utils/config"
	"github.com/WangYihang/Platypus/internal/utils/update"
	"github.com/WangYihang/Platypus/internal/webui"

	// Import the generated OpenAPI docs so `swag init`'s output is wired
	// into the binary. The swagger UI handler in internal/api looks up
	// docs by name ("swagger").
	_ "github.com/WangYihang/Platypus/docs"
)

const shutdownTimeout = 30 * time.Second

func main() {
	log.Init()
	hostname, _ := os.Hostname()
	log.SetBaseFields(
		"service", "platypus-server",
		"version", update.Version,
		"hostname", hostname,
	)

	cfg := parseFlags()
	log.L.Info("server_starting", "version", update.Version,
		"data_dir", cfg.DataDir,
		"listen", cfg.Listen,
		"external_addr", cfg.ExternalAddr,
	)

	core.Ctx = app.New(cfg)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Ensure data_dir exists before any subsystem touches files inside
	// it. Auto-creation here (rather than failing at first Open) keeps
	// the "drop a binary on a fresh box" experience close to one-step.
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		log.Error("create data dir %q: %v", cfg.DataDir, err)
		os.Exit(1)
	}

	// Open the persistent store before anything else that needs it
	// (enrollment, PKI, install tokens). Distributor / REST / agent
	// all share the same handle.
	dbPath := cfg.DBPath()
	db, err := storage.Open(dbPath)
	if err != nil {
		log.Error("open database %q: %v", dbPath, err)
		os.Exit(1)
	}
	core.Ctx.Storage = db
	log.Success("Storage: %s", dbPath)

	// KEK source policy:
	//   1. --ca-kek / PLATYPUS_CA_KEK (production path — key never touches disk).
	//   2. --dev / PLATYPUS_DEV=1 enables a dev-only fallback at <data-dir>/ca.kek so
	//      `docker compose up` bootstraps cleanly without operator action.
	//   3. Otherwise: refuse to start. Falling back to a co-located key file
	//      in production defeats AES-GCM-sealing the CA private key — a
	//      compromise of the data volume yields both ciphertext and key.
	if cfg.CAKEK != "" {
		// kong already populated the env var indirectly; re-export so
		// internal/pki (which reads pki.KEKEnvVar at use time) finds it.
		_ = os.Setenv(pki.KEKEnvVar, cfg.CAKEK)
	} else if cfg.Dev {
		pki.KEKPath = cfg.CAKEKPath()
		log.L.Warn("dev_mode_kek_fallback_enabled",
			"path", pki.KEKPath,
			"hint", "set --ca-kek (or PLATYPUS_CA_KEK) to keep the CA key out of the data volume",
		)
	} else {
		log.Error("CA key-encryption-key missing: set --ca-kek (or PLATYPUS_CA_KEK) to a base64 32-byte value, " +
			"or pass --dev (PLATYPUS_DEV=1) to enable the on-disk dev fallback at <data-dir>/ca.kek. " +
			"Generate one with: openssl rand 32 | base64")
		os.Exit(1)
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

	ingressAddr := cfg.Listen
	publicAddr := cfg.ExternalAddr
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

	// Mesh node (optional). Activated by the presence of a mesh PSK
	// file at <data_dir>/mesh.psk — when missing, mesh stays inert
	// and the server still serves agent traffic on the unified
	// ingress. The server self-issues a cert-bound leaf under
	// cfg.MeshProjectID's CA — same chain agents in that project use.
	// On any wiring failure mesh is skipped with an error log; server
	// startup never aborts over mesh.
	var meshNode *mesh.Node
	if fileExists(cfg.MeshPSKPath()) {
		if node := tryStartServerMesh(ctx, pkiSvc, cfg, publicAddr, settingsReg); node != nil {
			core.Ctx.Mesh = node
			meshNode = node
		}
	}

	// Custom TLS leaf is opt-in via the cert.pem / key.pem file
	// convention under <data_dir>. When both files exist they're used
	// directly; otherwise we self-issue a leaf from the project CA
	// (the only chain agents with PLATYPUS_PROJECT_CA pinned will
	// trust). This dual scheme lets `docker compose up` Just Work
	// without mkcert while still letting an operator drop in their
	// own corporate-CA leaf without code changes.
	certPath, keyPath := cfg.CertPath(), cfg.KeyPath()
	customCert := fileExists(certPath) && fileExists(keyPath)
	certSource := ingress.CertSource{}
	if customCert {
		certSource.CertFile = certPath
		certSource.KeyFile = keyPath
	} else {
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

	rest, agentLinkSvc := buildRESTEngine(ctx, cfg, db, pkiSvc, settingsReg)

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
		"listen", ingressAddr,
		"external_addr", publicAddr,
		"custom_tls", customCert,
	)

	log.L.Info("server_running")

	api.RecordSystemActivity(context.Background(), api.ActivityInput{
		Category:    "server",
		Action:      "server.start",
		TargetType:  "server",
		TargetLabel: "platypus-server",
		Meta: map[string]any{
			"version":      update.Version,
			"listen":       ingressAddr,
			"external":     publicAddr,
			"data_dir":     cfg.DataDir,
			"mesh_enabled": fileExists(cfg.MeshPSKPath()),
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

	// Tear down hijacked agent-link WS connections first. Their accept
	// loops block in yamux until the underlying session closes —
	// http.Server.Shutdown does not track hijacked conns, so without
	// this sweep Shutdown waits the full grace window for handlers
	// that would never return on their own.
	agentLinkSvc.CloseAll()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	_ = httpSrv.Shutdown(shutdownCtx)
	log.Success("Server stopped cleanly")
}

// parseFlags reads CLI flags and env vars into a config.Options,
// runs PostParse to apply derived defaults + load secret files, and
// exits cleanly on --help or --version. Any parse / validation
// failure prints the error to stderr and exits non-zero.
//
// Lives next to main() so the wiring is easy to find when an env var
// gets renamed.
func parseFlags() *config.Options {
	var opts config.Options
	kctx := kong.Parse(&opts,
		kong.Name("platypus-server"),
		kong.Description("Platypus host management hub. Configure via flags or PLATYPUS_* env vars."),
		kong.Vars{"version": update.Version},
		kong.UsageOnError(),
	)
	if err := opts.PostParse(); err != nil {
		kctx.Fatalf("%v", err)
	}
	return &opts
}

// fileExists is a tiny test seam that returns true when path exists
// and is readable as a regular file. Used by the cert.pem / key.pem /
// mesh.psk file-convention probes during startup.
func fileExists(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.Mode().IsRegular()
}

// fileExistsDir returns true when path is a directory. Used to gate
// the distributor wiring on the presence of a populated
// <data_dir>/releases tree.
func fileExistsDir(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

func buildRESTEngine(ctx context.Context, cfg *config.Options, db *storage.DB, pkiSvc *pki.Service, settingsReg *settings.Registry) (http.Handler, *core.AgentLinkService) {
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
	enrollTokensH := api.NewEnrollmentTokensHandler(db, enrollSvc)
	accountPATH := api.NewAccountPATHandler(db, tokenVerifier)
	adminRolesH := api.NewAdminRolesHandler(db)

	// /api/v1/install/<token> and /v1/manifest/* now live on the same
	// gin engine — no dedicated distributor port. distributorBase is
	// the public HTTPS origin the server is reachable at so
	// admin-minted install links copy straight into `curl -k ... | sh`.
	distributorBase := "https://" + api.PublicAddr
	installH := api.NewInstallTokensHandler(db, enrollSvc, distributorBase)
	enrollV2H := api.NewEnrollV2Handler(enrollSvc, pkiSvc).WithDB(db).WithApprovalPolicy(settingsReg)

	// v2 agent link handler (yamux-over-WebSocket, mTLS-auth'd).
	// agentLinkSvc was constructed upstream because SessionsV2Handler
	// also depends on it for project-dispatch.
	agentLinkH := api.NewAgentLinkHandler(agentLinkSvc, api.ProjectsCAPool(db)).WithDB(db)
	activitiesH := api.NewActivitiesHandler(db)
	caH := api.NewCAHandler(db, pkiSvc)
	topologyH := api.NewTopologyHandler(db).WithAgentLinks(agentLinkSvc)
	rbac := api.NewRBAC(db, tokenVerifier)

	api.SetActivityRecorder(api.NewActivityRecorder(db))
	core.SetEnrollment(enrollSvc)

	// Distributor (manifest + installer) is enabled when an operator
	// has populated <data_dir>/releases/ with a signed manifest +
	// matching binaries (or a CI rsync has). When the directory is
	// missing we skip wiring entirely — agent self-upgrade fails
	// gracefully (404 on manifest fetch) but every other admin
	// surface keeps working.
	releasesDir := cfg.ReleasesDir()
	if fileExistsDir(releasesDir) {
		store, err := artifact.NewLocalStore(releasesDir)
		if err != nil {
			log.Error("distributor: init local artifact store at %s: %v", releasesDir, err)
			os.Exit(1)
		}
		core.RegisterDistributorRoutes(rest, core.DistributorArgs{
			Settings: settingsReg,
			Store:    store,
		})
		log.L.Info("distributor_enabled", "root", releasesDir)
	} else {
		log.L.Info("distributor_disabled",
			"hint", "drop a signed release tree under "+releasesDir+" to enable agent self-upgrade",
		)
	}

	api.RegisterWebSocketRoutes(rest, auth)
	api.RegisterV1Routes(rest, auth)
	api.RegisterV1AuthRoutes(rest, authH, usersH, rbac)
	api.RegisterV1ProjectsRoutes(rest, projectsH, rbac)
	api.RegisterV1HostsRoutes(rest, hostsH, rbac)
	api.RegisterV1ProjectSessionsRoutes(rest, sessionsH, rbac)
	api.RegisterV1EnrollmentTokenRoutes(rest, enrollTokensH, rbac)
	api.RegisterV1AccountPATRoutes(rest, accountPATH, rbac)
	api.RegisterV1AdminRolesRoutes(rest, adminRolesH, rbac)
	api.RegisterV1InstallTokenRoutes(rest, installH, rbac)
	api.RegisterV2AgentEnrollRoute(rest, enrollV2H)
	api.RegisterV2AgentLinkRoute(rest, agentLinkH)
	api.RegisterV1AgentUpgradeRoutes(rest, api.NewAgentUpgradeHandler(agentLinkSvc), rbac)

	// Terminal session recording: every operator shell is mirrored to
	// an asciinema v2 cast file under <data_dir>/recordings.
	// Recording is always on; the dir is created up-front so a slow
	// first session doesn't pay the mkdir cost.
	recDir := cfg.RecordingDir()
	if err := os.MkdirAll(recDir, 0o700); err != nil {
		log.L.Warn("recording_dir_create_failed",
			"dir", recDir,
			"error", err.Error(),
			"hint", "recordings will fail until this directory is writable",
		)
	} else {
		log.L.Info("terminal_recording_enabled", "dir", recDir)
	}
	recMgr := recording.New(db, recDir, true)

	// Audit-tail close for recordings: a previous instance that exited
	// without graceful shutdown leaves rows in `recording` state. Mark
	// them failed so the UI can render them as truncated rather than
	// "still recording" forever.
	if n, err := db.TerminalRecordings().MarkAbandoned(ctx, "server restarted before session ended", time.Now().UTC()); err != nil {
		log.L.Warn("recording_audit_close_failed", "error", err.Error())
	} else if n > 0 {
		log.L.Info("recording_audit_close", "rows", n)
	}

	// Host-id lookup callback consumed by the v2 terminal handler so
	// recording rows carry host_id without the recording package
	// importing storage in a way that would cycle.
	api.SetHostLookup(func(ctx context.Context, agentID string) (string, bool) {
		host, err := db.Hosts().GetByAgentID(ctx, agentID)
		if err != nil || host == nil {
			return "", false
		}
		return host.ID, true
	})

	api.RegisterV2TerminalRoute(rest, agentLinkSvc, rbac, recMgr)
	api.RegisterV1RecordingRoutes(rest, api.NewRecordingsHandler(db, recMgr), rbac)
	api.RegisterV2FileRoutes(rest, agentLinkSvc, rbac)
	// File-transfer archive + scan + transfers REST API. The cancel
	// registry is shared between the streaming handler (which
	// registers in-flight transfers) and the cancel REST endpoint
	// (which fires the matching cancel func). The broadcaster rides
	// the existing /notify melody so frontend subscribers get progress
	// events on the same connection they already keep open for session
	// lifecycle events.
	transferCancels := api.NewTransferCancelRegistry()
	transferRecorder := api.NewDBTransferRecorder(db)
	var archiveBroadcaster *api.EventBroadcaster
	if core.Ctx.NotifyWebSocket != nil {
		archiveBroadcaster = api.NewEventBroadcasterFromMelody(core.Ctx.NotifyWebSocket)
	}
	previewSigner, err := api.NewPreviewSigner()
	if err != nil {
		// crypto/rand failing at startup means the kernel CSPRNG is
		// unavailable — there's no useful degraded mode for an HTTP
		// server in that state, so fail loud.
		panic(fmt.Errorf("init preview signer: %w", err))
	}
	api.RegisterV2FileArchiveRoutes(rest, api.FileArchiveDeps{
		Service:       agentLinkSvc,
		RBAC:          rbac,
		Recorder:      transferRecorder,
		Broadcaster:   archiveBroadcaster,
		Cancels:       transferCancels,
		Hosts:         db.Hosts(),
		PreviewSigner: previewSigner,
	})
	api.RegisterV1TransferRoutes(rest, api.TransferRoutesDeps{
		DB:      db,
		RBAC:    rbac,
		Cancels: transferCancels,
	})
	api.RegisterV2AgentRPCRoutes(rest, agentLinkSvc, rbac)
	api.RegisterV1ActivitiesRoutes(rest, activitiesH, rbac)
	api.RegisterV1CARoutes(rest, caH, rbac)
	api.RegisterV1TopologyRoutes(rest, topologyH, rbac)
	api.RegisterV1AdminSettingsRoutes(rest, api.NewAdminSettingsHandler(settingsReg), rbac)
	api.RegisterSwaggerRoutes(rest)

	// Frontend bundle. Must be the last registration so explicit API
	// routes win first-match; webui's NoRoute handler is the catch-all
	// for both static asset serving and React Router SPA fallback.
	webui.RegisterRoutes(rest)

	// TopologyStream + observer were v1 — they relied on the
	// AgentClient map and legacy sysinfo cache. Rebuilt on top of
	// v2 in Phase IV (mesh hardening) which is the right layer for
	// topology telemetry anyway.

	// Point the info endpoint at our live agent link registry so
	// /api/v1/info's session_count reflects actual v2 connections.
	api.LiveAgentCounter = func() int { return len(agentLinkSvc.All()) }

	// Wire the cross-project counts the status bar polls at 1 Hz.
	// Online threshold matches the desktop frontend's
	// lib/time.ts ONLINE_WINDOW_MS (60 s) so a host that's "green"
	// on the dashboard is also counted in live_host_count.
	const onlineWindow = 60 * time.Second
	api.Counts = func(ctx context.Context) (storage.Counts, error) {
		return db.Counts(ctx, onlineWindow)
	}

	// Sample the server process's CPU% in the background so the
	// /api/v1/info handler doesn't pay the gopsutil-blocks-1s tax
	// on every poll. Uses the same ctx as the rest of main so it
	// stops cleanly on shutdown. NewCPUSampler can technically
	// fail; we degrade by leaving api.CPUPercent at its no-op
	// default rather than aborting startup over a status-bar field.
	if cpu, err := api.NewCPUSampler(); err != nil {
		log.Warn("server.cpu_sampler_init_failed: %v", err)
	} else {
		cpu.Start(ctx)
		api.CPUPercent = cpu.Percent
	}

	// Bootstrap-secret handling. The secret is only useful while the
	// users table is empty (Bootstrap is the one-shot first-admin flow);
	// after that it's dead weight. Two failure modes the previous code
	// had:
	//   1. Logging the value at INFO meant any reader of the server log
	//      (centralised logging, journalctl, docker logs) could replay
	//      it on a re-bootstrap window.
	//   2. The value persisted in scrollback long after first admin
	//      existed, even though it had no operational use anymore.
	// Mitigation: when no admin exists, write the secret to
	// <data-dir>/bootstrap.secret with mode 0600 and log only the path.
	// When an admin exists, log a redacted marker so operators can see
	// the server is up without leaking anything.
	bootstrapPath := filepath.Join(cfg.DataDir, "bootstrap.secret")
	adminCount, _ := db.Users().Count(ctx)
	if adminCount == 0 {
		_ = os.MkdirAll(cfg.DataDir, 0o700)
		if err := os.WriteFile(bootstrapPath, []byte(auth.GetSecret()+"\n"), 0o600); err != nil {
			log.L.Warn("bootstrap_secret_write_failed",
				"path", bootstrapPath,
				"error", err.Error(),
				"hint", "first-run admin bootstrap will be unreachable until this is resolved",
			)
		} else {
			log.L.Info("bootstrap_secret_written",
				"path", bootstrapPath,
				"hint", "use this once to create the first admin, then delete it",
			)
		}
	} else {
		// Tidy up any leftover bootstrap.secret from an earlier first-run.
		// Best-effort: ignore not-exist; surface anything else as a warn.
		if err := os.Remove(bootstrapPath); err != nil && !os.IsNotExist(err) {
			log.L.Warn("bootstrap_secret_cleanup_failed",
				"path", bootstrapPath,
				"error", err.Error(),
			)
		}
	}
	log.L.Info("api_ready",
		"bootstrap_secret", "<redacted; see bootstrap.secret in data dir on first run>",
		"public_base", "https://"+api.PublicAddr,
	)

	core.Ctx.RESTful = rest
	return rest, agentLinkSvc
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
// cfg.ExternalAddr (split off the port).
func issueIngressLeafFromProjectCA(ctx context.Context, pkiSvc *pki.Service, cfg *config.Options) (*tls.Certificate, error) {
	projectID := cfg.MeshProjectID
	if projectID == "" {
		projectID = storage.DefaultProjectID
	}
	// EnsureCA first so the private bits exist before the IssueServerCert
	// path tries to unseal them. createdBy is the seeded system user.
	if _, err := pkiSvc.EnsureCA(ctx, projectID, storage.SystemUserID); err != nil {
		return nil, fmt.Errorf("ensure project CA: %w", err)
	}
	addr := cfg.ExternalAddr
	if addr == "" {
		return nil, errors.New("--external-addr is empty")
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

// Project scoping: cfg.MeshProjectID picks which project's CA to
// chain the server's leaf to. Agents in the same project will
// trust the resulting identity via the same CA.
func tryStartServerMesh(ctx context.Context, pkiSvc *pki.Service, cfg *config.Options, publicAddr string, settingsReg *settings.Registry) *mesh.Node {
	projectID := cfg.MeshProjectID
	if projectID == "" {
		log.Error("mesh: --mesh-project is required for the server to self-issue a leaf cert; skipping")
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

	// Bootstrap target + advertise addr both default to the server's
	// own external_addr — it's the one address every agent already
	// knows how to reach. Operators with multi-server federation needs
	// can override these via the SettingsRegistry; the YAML-side knobs
	// were deemed runtime-policy and moved out of bootstrap config.
	bootstrapTarget := publicAddr
	var advertise []string
	if publicAddr != "" {
		advertise = []string{publicAddr}
	}
	node, err := mesh.NewNode(mesh.Config{
		PSKFile:        cfg.MeshPSKPath(),
		Identity:       meshID,
		TrustedCAs:     pool,
		ListenAddr:     "", // listener is the unified ingress
		AdvertiseAddrs: advertise,
		// Peers, DiscoveryLAN, DiscoveryInterval all flow through
		// the SettingsRegistry; cfg-side defaults are gone.
		Role:             "server",
		DiscoveryLAN:     true,
		ProjectID:        projectID,
		BootstrapEnabled: bootstrapTarget != "",
		BootstrapTarget:  bootstrapTarget,
		Settings:         settingsReg,
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
