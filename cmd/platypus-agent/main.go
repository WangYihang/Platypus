package main

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/cenkalti/backoff/v4"

	"github.com/WangYihang/Platypus/internal/agent"
	"github.com/WangYihang/Platypus/internal/link"
	"github.com/WangYihang/Platypus/internal/log"
	"github.com/WangYihang/Platypus/internal/mesh"
	"github.com/WangYihang/Platypus/pkg/installbundle"
	"github.com/WangYihang/Platypus/pkg/options"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	slog.SetDefault(logger)
	// log.L (used by all internal/agent library code) gets stable
	// per-process fields up-front. agent_id is filled in once we've
	// loaded / enrolled an Identity below — until then logs render
	// agent_id="" but at least carry service+hostname so cross-host
	// log aggregation can still bucket lines.
	hostname, _ := os.Hostname()
	log.SetBaseFields(
		"service", "platypus-agent",
		"hostname", hostname,
		"agent_version", "v2",
	)

	opts, err := options.InitOptions()
	if err != nil {
		printUsage(err)
		os.Exit(1)
	}

	// Self-contained install bundle: when the user pasted a
	// `pinst_<base64>` token, expand it into the equivalent (token +
	// server + CA bytes) trio so the rest of bootstrap stays
	// unchanged. Explicit --host / --port still win — they're an
	// escape hatch for an admin debugging an unrelated server.
	bundleCAOverride, err := expandInstallBundle(opts)
	if err != nil {
		printUsage(err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	identityDir := agent.ResolveIdentityDir(opts.DataDir)

	// Subcommand dispatch — `psk install` and `psk show` are
	// one-shot helpers that exit before we touch network state.
	switch opts.Sub {
	case options.SubcommandPSKInstall:
		os.Exit(runPSKInstall(logger, opts, identityDir))
	case options.SubcommandPSKShow:
		os.Exit(runPSKShow(logger, opts, identityDir))
	}

	if opts.Token == "" {
		// On a fresh install the token is required; on a re-run with
		// a persisted identity we tolerate an empty token (loadOrEnroll
		// will skip the redeem step and reuse the cached cert).
		if !agent.HasPersistedIdentity(identityDir) {
			printUsage(options.ErrMissingToken)
			os.Exit(1)
		}
	}
	if opts.RemoteHost == "" || opts.RemotePort == 0 {
		// Same logic for server endpoint: tolerate missing only
		// when persisted state lets us skip enrollment.
		if !agent.HasPersistedIdentity(identityDir) {
			printUsage(options.ErrMissingServer)
			os.Exit(1)
		}
	}

	state := agent.Init()
	pskPath, err := agent.ResolvePSKFile(agent.PSKResolveOptions{
		CLIPath:   opts.MeshPSKFile,
		EnvFile:   os.Getenv(options.EnvMeshPSKFile),
		EnvInline: os.Getenv(options.EnvMeshPSK),
		DataDir:   identityDir,
	})
	if err != nil {
		logger.Error("resolve mesh PSK", slog.String("error", err.Error()))
		os.Exit(1)
	}
	meshPSKFile := pskPath
	meshPeers := append([]string(nil), opts.MeshPeers...)

	// Project ID is derived from the enrolled cert when an identity
	// already exists on disk; fresh installs leave it empty until
	// after BootstrapV2, at which point the next process restart
	// picks it up.
	var meshProjectID string
	if id, err := agent.LoadIdentity(identityDir); err == nil && id != nil {
		if block, _ := pem.Decode(id.CertPEM); block != nil {
			if cert, err := x509.ParseCertificate(block.Bytes); err == nil {
				meshProjectID = projectIDFromURIs(cert.URIs)
			}
		}
	}
	// Mesh state lives under the active enrollment's per-CA subdir.
	// Migrate any pre-multi-CA flat layout so the active pointer
	// becomes meaningful before we read it.
	if err := agent.MigrateLegacyIdentity(identityDir); err != nil {
		logger.Warn("migrate legacy identity", slog.String("error", err.Error()))
	}
	if activeFP, err := agent.ReadActive(identityDir); err == nil && activeFP != "" {
		if persisted, err := agent.LoadPersistedMeshBootstrap(identityDir, activeFP); err != nil {
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
	}
	endpoint := fmt.Sprintf("%s:%d", opts.RemoteHost, opts.RemotePort)
	if opts.RemoteHost == "" {
		// Fall back to whatever the persisted identity dir knows
		// about — the v2 dial path will retry-with-cached behavior
		// once an identity is on disk.
		endpoint = ""
	}
	logger.Info("starting agent",
		slog.String("endpoint", endpoint),
		slog.String("token", opts.Token),
	)

	// Mesh overlay is opt-in: enable when the operator provides a
	// PSK file. Requires the agent to already be enrolled — the
	// cert-bound mesh identity comes from the same disk files as
	// the v2 dial identity. Fresh installs need to complete
	// BootstrapV2 (and restart the agent) before mesh starts.
	if meshPSKFile != "" {
		node := tryStartMesh(ctx, logger, identityDir, meshPSKFile, meshPeers, meshProjectID, opts)
		if node != nil {
			agent.AttachMesh(state, node)
		}
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

	// Bundle-expanded CA bytes win over the env-var path: the bundle
	// is the most specific source for "this enrollment's project CA".
	// Falls back to PLATYPUS_PROJECT_CA when no bundle was supplied.
	var caPEM []byte
	var caPool *x509.CertPool
	if len(bundleCAOverride) > 0 {
		caPEM = bundleCAOverride
		caPool = x509.NewCertPool()
		if !caPool.AppendCertsFromPEM(caPEM) {
			logger.Error("install bundle CA: no valid CERTIFICATE blocks")
			os.Exit(1)
		}
	} else {
		caEnv := os.Getenv(agent.ProjectCAEnvVar)
		caPool, err = agent.LoadProjectCA(caEnv)
		if err != nil {
			logger.Error("parse project CA env var", slog.String("error", err.Error()))
			os.Exit(1)
		}
		if caEnv != "" {
			decoded, decodeErr := base64.StdEncoding.DecodeString(caEnv)
			if decodeErr != nil {
				logger.Error("decode project CA env var", slog.String("error", decodeErr.Error()))
				os.Exit(1)
			}
			caPEM = decoded
		}
	}

	// Bootstrap can run before the agent has any persisted identity
	// (fresh install) so we promote agent_id into the log base fields
	// the first time we manage to load one off disk after a successful
	// enrollment. Idempotent across reconnects.
	var agentIDPromoted sync.Once
	promoteAgentID := func() {
		agentIDPromoted.Do(func() {
			id, err := agent.LoadIdentity(identityDir)
			if err != nil || id == nil {
				return
			}
			block, _ := pem.Decode(id.CertPEM)
			if block == nil {
				return
			}
			cert, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				return
			}
			agentID := agentIDFromURIs(cert.URIs)
			if agentID == "" {
				return
			}
			log.SetBaseFields("agent_id", agentID)
		})
	}

	var connectAttempt int
	connect := func() error {
		if ctx.Err() != nil {
			return backoff.Permanent(ctx.Err())
		}
		connectAttempt++
		dialStart := time.Now()
		logger.Info("agent connect (v2)",
			slog.String("endpoint", endpoint),
			slog.Int("attempt", connectAttempt),
		)

		sess, err := agent.BootstrapV2(ctx, agent.BootstrapV2Options{
			IdentityDir:  identityDir,
			ServerURL:    fmt.Sprintf("wss://%s/api/v1/agent/link", endpoint),
			EnrollURL:    fmt.Sprintf("https://%s", endpoint),
			PAT:          opts.Token,
			ProjectCAPEM: caPEM,
			ProjectCA:    caPool,
			Hostname:     hostname,
			AgentVersion: "v2",
		})
		if err != nil {
			// Approval gate: the agent is enrolled but the operator
			// hasn't clicked Approve yet. Don't spam the log with the
			// full error chain — print a single friendly status line
			// and let the backoff retry. Treated as a transient
			// failure so backoff keeps growing.
			if errors.Is(err, link.ErrPendingApproval) {
				logger.Info("agent waiting for admin approval — ask an administrator to approve this host in the Web UI",
					slog.String("endpoint", endpoint),
					slog.Int("attempt", connectAttempt),
				)
				return err
			}
			// Hard reject: the cert is dead from the server's
			// perspective. Stop retrying — backoff.Permanent breaks
			// the loop.
			if errors.Is(err, link.ErrApprovalRejected) {
				logger.Error("agent enrollment rejected by administrator; the locally-stored cert is no longer accepted; obtain a fresh install token and re-enroll",
					slog.String("endpoint", endpoint),
				)
				return backoff.Permanent(err)
			}
			logger.Warn("agent bootstrap failed",
				slog.String("endpoint", endpoint),
				slog.Int("attempt", connectAttempt),
				slog.Duration("elapsed", time.Since(dialStart)),
				slog.String("error", err.Error()),
			)
			return err
		}
		defer sess.Close()
		promoteAgentID()

		linkStart := time.Now()
		logger.Info("v2 link established; serving streams",
			slog.String("endpoint", endpoint),
			slog.Int("attempt", connectAttempt),
			slog.Duration("dial_elapsed", time.Since(dialStart)),
		)
		serveErr := agent.ServeLink(ctx, sess, agent.AgentHandlerDeps{
			RPC: agent.AgentRPCHandlers{
				Exec:        agent.HandleExec,
				ListDir:     agent.HandleListDir,
				Stat:        agent.HandleStat,
				Delete:      agent.HandleDelete,
				Rename:      agent.HandleRename,
				Mkdir:       agent.HandleMkdir,
				Chmod:       agent.HandleChmod,
				SysInfo:     agent.HandleSysInfo,
				ProcessList: agent.HandleProcessList,
			},
			Process:     agent.HandleProcessStream,
			FileRead:    agent.HandleFileReadStream,
			FileWrite:   agent.HandleFileWriteStream,
			FileScan:    agent.HandleFileScanStream,
			FileArchive: agent.HandleFileArchiveStream,
			TunnelPull:  agent.HandleTunnelPullStream,
		})
		reason := "peer_close"
		if serveErr != nil {
			reason = serveErr.Error()
		}
		logger.Info("v2 link terminated",
			slog.String("endpoint", endpoint),
			slog.Int("attempt", connectAttempt),
			slog.Duration("link_duration", time.Since(linkStart)),
			slog.String("reason", reason),
		)
		return serveErr
	}

	notify := func(err error, next time.Duration) {
		// Without this, every connect/enroll failure gets swallowed
		// by backoff and the operator sees nothing but repeating
		// "agent connect (v2)" lines — the exact reason several
		// recent debugging sessions had to resort to server-side
		// packet captures. Log one line per failure with the actual
		// error string and the retry delay.
		logger.Warn("agent connect failed, retrying",
			slog.String("error", err.Error()),
			slog.Duration("next_retry_in", next),
			slog.Int("attempt", connectAttempt),
		)
	}
	if err := backoff.RetryNotify(connect, bo, notify); err != nil {
		logger.Error("connection loop terminated", slog.String("error", err.Error()))
		os.Exit(1)
	}
	_ = state // kept for when mesh Phase IV wires v2 back into agent.State
	logger.Info("agent stopped")
}

// tryStartMesh loads the enrolled cert material, builds a cert-
// bound mesh.Identity + project-CA pool, and starts mesh. Any
// step failing is logged and returns nil — the agent continues in
// pure hub-and-spoke mode. A fresh-install agent hits the
// ErrIdentityNotFound branch (no cert on disk yet) and will only
// join the mesh after the next restart following BootstrapV2.
func tryStartMesh(ctx context.Context, logger *slog.Logger, identityDir, pskFile string, peers []string, projectID string, opts *options.Options) *mesh.Node {
	agentID, err := agent.LoadIdentity(identityDir)
	if err != nil {
		if errors.Is(err, agent.ErrIdentityNotFound) {
			logger.Info("mesh: deferred — agent not yet enrolled; retry after first BootstrapV2 success")
		} else {
			logger.Error("mesh: load agent identity", slog.String("error", err.Error()))
		}
		return nil
	}
	meshID, err := meshIdentityFromAgentID(agentID)
	if err != nil {
		logger.Error("mesh: build cert-bound identity", slog.String("error", err.Error()))
		return nil
	}
	pool, err := certPoolFromPEM(agentID.CAPEM)
	if err != nil {
		logger.Error("mesh: parse project CA", slog.String("error", err.Error()))
		return nil
	}
	node, err := mesh.NewNode(mesh.Config{
		PSKFile:           pskFile,
		Identity:          meshID,
		TrustedCAs:        pool,
		ListenAddr:        opts.MeshListen,
		Peers:             peers,
		AdvertiseAddrs:    opts.MeshAdvertise,
		Role:              "agent",
		DiscoveryLAN:      opts.MeshDiscoveryLAN,
		DiscoveryInterval: opts.MeshDiscoveryInterval,
		ProjectID:         projectID,
	}, logger)
	if err != nil {
		logger.Error("mesh init", slog.String("error", err.Error()))
		return nil
	}
	if err := node.Start(ctx); err != nil {
		logger.Error("mesh start", slog.String("error", err.Error()))
		return nil
	}
	logger.Info("mesh enabled",
		slog.String("node_id", node.NodeID()),
		slog.String("listen", node.ListenerAddr()))
	return node
}

// meshIdentityFromAgentID turns the enrolled agent.Identity (cert
// PEM + parsed Ed25519 key) into a mesh.Identity by re-marshalling
// the key to PKCS#8 PEM and feeding both into LoadIdentityFromCert.
func meshIdentityFromAgentID(id *agent.Identity) (*mesh.Identity, error) {
	keyDER, err := x509.MarshalPKCS8PrivateKey(id.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("marshal agent key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	return mesh.LoadIdentityFromCert(id.CertPEM, keyPEM)
}

// certPoolFromPEM builds an x509.CertPool from one or more
// concatenated CERTIFICATE PEM blocks.
func certPoolFromPEM(caPEM []byte) (*x509.CertPool, error) {
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, errors.New("no valid CERTIFICATE blocks in CA PEM")
	}
	return pool, nil
}

// agentIDFromURIs extracts the agent_id encoded in the cert's
// "platypus://agent/<id>" URI SAN. Mirrors the server-side
// parseAgentSANs but trimmed to the single field the agent-side log
// base needs. Returns "" when no matching SAN is present (older
// fixtures, malformed certs).
func agentIDFromURIs(uris []*url.URL) string {
	for _, u := range uris {
		if u == nil || u.Scheme != "platypus" || u.Host != "agent" {
			continue
		}
		return strings.TrimPrefix(u.Path, "/")
	}
	return ""
}

// projectIDFromURIs is the project_id companion to agentIDFromURIs.
// Lets the agent side derive the project from the enrolled cert
// without an extra config flag — the same data the server side
// already pins via mTLS chain validation.
func projectIDFromURIs(uris []*url.URL) string {
	for _, u := range uris {
		if u == nil || u.Scheme != "platypus" || u.Host != "project" {
			continue
		}
		return strings.TrimPrefix(u.Path, "/")
	}
	return ""
}

// expandInstallBundle inspects opts.Token for the `pinst_` prefix
// and, when present, replaces opts.Token / opts.RemoteHost /
// opts.RemotePort with the bundle's contents and returns the bundle's
// CA PEM bytes for the caller to plug into the TLS path. Explicit
// --host / --port flags win over the bundle's server endpoint.
//
// Returns nil bytes (and no error) when no bundle was supplied, so
// the legacy code path stays a no-op.
func expandInstallBundle(opts *options.Options) ([]byte, error) {
	if !installbundle.Looks(opts.Token) {
		return nil, nil
	}
	b, err := installbundle.Decode(opts.Token)
	if err != nil {
		return nil, fmt.Errorf("install bundle: %w", err)
	}
	opts.Token = b.PAT
	if opts.RemoteHost == "" || opts.RemotePort == 0 {
		// Bundle's server endpoint fills in only when the operator
		// didn't override on the CLI / env var.
		host, port, err := splitBundleHostPort(b.Server)
		if err != nil {
			return nil, fmt.Errorf("install bundle: %w", err)
		}
		opts.RemoteHost = host
		opts.RemotePort = port
	}
	return []byte(b.CACertPEM), nil
}

// splitBundleHostPort splits a "host:port" string. Mirrors the
// pkg/options helper but kept local here so the agent main.go has
// no cross-package dep just for one trivial parse.
func splitBundleHostPort(s string) (string, int, error) {
	i := strings.LastIndex(s, ":")
	if i <= 0 || i == len(s)-1 {
		return "", 0, fmt.Errorf("expected host:port, got %q", s)
	}
	host := s[:i]
	port, err := strconv.Atoi(s[i+1:])
	if err != nil {
		return "", 0, fmt.Errorf("port: %w", err)
	}
	return host, port, nil
}

// runPSKInstall handles `platypus-agent psk install <psk>`. Writes the
// PSK to <data-dir>/mesh.psk (or --psk-file when overridden) and
// exits. Returns the desired process exit code.
func runPSKInstall(logger *slog.Logger, opts *options.Options, dataDir string) int {
	target := opts.MeshPSKFile
	if target == "" {
		target = agent.DefaultPSKTarget(dataDir)
	}
	if err := agent.InstallPSK(target, opts.PSKArg); err != nil {
		logger.Error("psk install failed", slog.String("error", err.Error()))
		return 1
	}
	logger.Info("psk installed",
		slog.String("path", target),
		slog.String("hint", "subsequent agent runs will pick this up automatically"),
	)
	return 0
}

// runPSKShow handles `platypus-agent psk show`. Prints the resolved
// PSK file path and whether the file exists. Does NOT print the PSK
// itself — the file's contents stay on disk under 0600.
func runPSKShow(logger *slog.Logger, opts *options.Options, dataDir string) int {
	resolved, err := agent.ResolvePSKFile(agent.PSKResolveOptions{
		CLIPath:   opts.MeshPSKFile,
		EnvFile:   os.Getenv(options.EnvMeshPSKFile),
		EnvInline: os.Getenv(options.EnvMeshPSK),
		DataDir:   dataDir,
	})
	if err != nil {
		logger.Error("resolve psk", slog.String("error", err.Error()))
		return 1
	}
	if resolved == "" {
		fmt.Fprintln(os.Stdout, "no PSK installed; mesh participation is disabled")
		fmt.Fprintln(os.Stdout, "to install one, run: platypus-agent psk install <psk>")
		return 0
	}
	fmt.Fprintln(os.Stdout, resolved)
	return 0
}

// printUsage writes a hand-rolled help block to stderr. Avoids the
// stdlib flag dump (which would leak hidden flags) and gives a
// concrete pointer at the env vars / subcommands.
func printUsage(err error) {
	fmt.Fprintln(os.Stderr, "platypus-agent — connect a host to a Platypus server")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintln(os.Stderr, "  platypus-agent <install-token>")
	fmt.Fprintln(os.Stderr, "  platypus-agent psk install <psk>")
	fmt.Fprintln(os.Stderr, "  platypus-agent psk show")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Environment:")
	fmt.Fprintln(os.Stderr, "  "+options.EnvInstallToken+"   alternative to positional install-token")
	fmt.Fprintln(os.Stderr, "  "+options.EnvServerAddr+"           server host:port; alternative to --host/--port")
	fmt.Fprintln(os.Stderr, "  "+options.EnvDataDir+"        writable state dir (default: ~/.platypus/agent)")
	fmt.Fprintln(os.Stderr, "  "+options.EnvMeshPSKFile+"   absolute path to mesh PSK")
	fmt.Fprintln(os.Stderr, "  "+options.EnvMeshPSK+"        inline mesh PSK contents (overrides "+options.EnvMeshPSKFile+")")
	fmt.Fprintln(os.Stderr, "  PLATYPUS_PROJECT_CA       base64-encoded project CA cert (set by install scripts)")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "PSK resolution: "+options.PSKResolutionOrder)
	if err != nil {
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Error:", err.Error())
	}
}
