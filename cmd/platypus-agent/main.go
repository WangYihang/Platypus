package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
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
	pluginrt "github.com/WangYihang/Platypus/internal/agent/plugin"
	pluginbridge "github.com/WangYihang/Platypus/internal/agent/plugin/bridge"
	pluginsys "github.com/WangYihang/Platypus/internal/agent/plugin/system"
	"github.com/WangYihang/Platypus/internal/link"
	"github.com/WangYihang/Platypus/internal/log"
	"github.com/WangYihang/Platypus/internal/mesh"
	"github.com/WangYihang/Platypus/pkg/installbundle"
	"github.com/WangYihang/Platypus/pkg/options"
	"github.com/WangYihang/Platypus/pkg/version"
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
		"build_version", version.Version,
		"build_commit", version.Commit,
	)

	opts, err := options.InitOptions()
	if err != nil {
		printUsage(err)
		os.Exit(1)
	}

	// Self-contained install bundle: when the user pasted a
	// `pinst_<base64>` token, expand it into the equivalent (token +
	// server + CA bytes) trio so the rest of bootstrap stays
	// unchanged. Explicit --server still wins — escape hatch for an
	// admin debugging an unrelated server.
	bundleCAOverride, err := expandInstallBundle(opts)
	if err != nil {
		printUsage(err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	identityDir := agent.ResolveIdentityDir(opts.DataDir)

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

	// Mesh runs unconditionally once the agent has an enrolled
	// identity. Fresh installs hit ErrIdentityNotFound inside
	// tryStartMesh and will only join the mesh after the first
	// BootstrapV2 + agent restart.
	if node := tryStartMesh(ctx, logger, identityDir, meshPeers, meshProjectID, opts); node != nil {
		agent.AttachMesh(state, node)
	}

	// The plugin runtime is now load-bearing: every RPC handler and
	// every stream type the agent serves dispatches through a system
	// plugin. Failure to bring it up means the agent can't do
	// anything useful, so we exit hard rather than running in a
	// degraded mode that silently drops every server-initiated action.
	pluginRegistry, err := pluginrt.New(pluginrt.Options{
		Paths: pluginrt.NewPaths(identityDir),
	})
	if err != nil {
		logger.Error("plugin registry init failed; agent cannot serve RPCs without it",
			slog.String("error", err.Error()))
		os.Exit(1)
	}
	pluginRegistry.Load(ctx)
	defer pluginRegistry.Close(ctx)

	// System-plugin bootstrap: resolve the active source tree
	// (data-dir override > embedded fallback) then walk it,
	// installing any bundled plugins missing from the catalog.
	// Per-bundle failures are surfaced + counted so a corrupt
	// build is loud, not silent; the operator gets to see exactly
	// which plugins didn't load.
	src, fsErr := pluginsys.ResolveSource(opts.DataDir)
	if fsErr != nil {
		logger.Error("system plugin source unavailable; agent build is broken",
			slog.String("error", fsErr.Error()))
		os.Exit(1)
	}
	allowlist := resolveBaselineAllowlist(logger, identityDir, opts.BaselinePluginIDs)
	bootRes := pluginsys.EnsureInstalled(ctx, pluginRegistry, src.FS, pluginsys.EnsureOptions{
		Allowlist: allowlist,
	})
	logger.Info("system plugins bootstrap",
		slog.String("source", src.Origin),
		slog.Any("allowlist", allowlist),
		slog.Int("installed", len(bootRes.Installed)),
		slog.Int("skipped", len(bootRes.Skipped)),
		slog.Int("failed", len(bootRes.Failed)),
		slog.Int("filtered", len(bootRes.Filtered)),
	)
	if bootRes.SetupError != nil {
		// publisher.pub missing or unreadable — every signature
		// verify will fail, every plugin load will fail. Hard exit so
		// the operator sees the build issue immediately rather than
		// at first RPC.
		logger.Error("system plugins setup failed; cannot verify bundled plugins",
			slog.String("error", bootRes.SetupError.Error()))
		os.Exit(1)
	}
	if len(bootRes.Failed) > 0 {
		// At least one bundled system plugin couldn't be installed.
		// Don't exit — the rest may still be functional — but log
		// loudly so the operator notices the partial fleet.
		for _, f := range bootRes.Failed {
			logger.Error("system plugin install failed",
				slog.String("plugin_id", f.ID),
				slog.String("version", f.Version),
				slog.String("error", f.Err.Error()))
		}
	}

	warnUncoveredStreams(logger, pluginRegistry)

	bo := backoff.WithContext(
		backoff.NewExponentialBackOff(
			backoff.WithMaxInterval(1*time.Minute),
			backoff.WithMaxElapsedTime(0),
		),
		ctx,
	)

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
			IdentityDir:     identityDir,
			ServerURL:       fmt.Sprintf("wss://%s/api/v1/agent/link", endpoint),
			EnrollURL:       fmt.Sprintf("https://%s", endpoint),
			PAT:             opts.Token,
			ProjectCAPEM:    caPEM,
			ProjectCA:       caPool,
			Hostname:        hostname,
			BuildVersion:    version.Version,
			BuildCommit:     version.Commit,
			BuildDate:       version.Date,
			ProtocolVersion: link.ProtocolVersion,
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
		// Every RPC handler dispatches through a bridge wrapper that
		// proxies to the matching system plugin. Stream-type
		// dispatch goes through pluginRegistry.DispatchStream below
		// — the wasm system plugins (sys-files-read [file_read +
		// file_scan + file_archive], sys-files-write [file_write],
		// sys-process [process_open], sys-tunnel-pull) claim the
		// corresponding STREAM_TYPE_* and
		// serve every wire frame in-sandbox. There are no Go-resident
		// stream handlers any more.
		serveErr := agent.ServeLink(ctx, sess, agent.AgentHandlerDeps{
			RPC: agent.AgentRPCHandlers{
				Exec:               pluginbridge.Exec(pluginRegistry),
				ListDir:            pluginbridge.ListDir(pluginRegistry),
				Stat:               pluginbridge.Stat(pluginRegistry),
				Delete:             pluginbridge.Delete(pluginRegistry),
				Rename:             pluginbridge.Rename(pluginRegistry),
				Mkdir:              pluginbridge.Mkdir(pluginRegistry),
				Chmod:              pluginbridge.Chmod(pluginRegistry),
				SysInfo:            pluginbridge.SysInfo(pluginRegistry),
				ProcessList:        pluginbridge.ProcessList(pluginRegistry),
				SecurityScan:       pluginbridge.SecurityScan(pluginRegistry),
				ListSecurityChecks: pluginbridge.ListSecurityChecks(pluginRegistry),
				ConfigAudit:        pluginbridge.ConfigAudit(pluginRegistry),
				ListConfigAuditors: pluginbridge.ListConfigAuditors(pluginRegistry),
				PluginCall:         pluginRegistry.Invoke,
			},
			Upgrade:      buildUpgradeHandler(logger, fmt.Sprintf("https://%s", endpoint), caPool),
			PluginMgmt:   pluginRegistry.HandleMgmt,
			PluginStream: pluginRegistry.DispatchStream,
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
func tryStartMesh(ctx context.Context, logger *slog.Logger, identityDir string, peers []string, projectID string, opts *options.Options) *mesh.Node {
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

	if opts.MeshListen != "" {
		handler := mesh.NewLinkHandler(node, func() *x509.CertPool { return pool })
		pl, err := mesh.NewPeerListener(opts.MeshListen, meshID, pool, handler)
		if err != nil {
			logger.Error("mesh peer listener init", slog.String("error", err.Error()))
			return node
		}
		go func() {
			if err := pl.Serve(); err != nil {
				logger.Error("mesh peer listener serve", slog.String("error", err.Error()))
			}
		}()
		go func() {
			<-ctx.Done()
			shutdownCtx, cancel := contextWithTimeout(2 * time.Second)
			defer cancel()
			_ = pl.Shutdown(shutdownCtx)
		}()
		logger.Info("mesh peer listener up", slog.String("addr", pl.Addr()))
	}

	logger.Info("mesh enabled",
		slog.String("node_id", node.NodeID()),
		slog.String("listen", node.ListenerAddr()))
	return node
}

func contextWithTimeout(d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), d)
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

// buildUpgradeHandler wires an UpgradeRunner against this binary's
// resolved path, the project CA pool, and the baked-in signing
// pubkey. Returns a UpgradeHandler closure suitable for
// AgentHandlerDeps.Upgrade.
//
// The returned handler is nil — and therefore answers
// STREAM_TYPE_AGENT_UPGRADE with StreamReject{"unsupported_type"} —
// only when self-upgrade can't run safely on this binary: empty
// pubkey (release pipeline didn't sign), or os.Executable failed.
// Both cases are logged once at startup so operators see the
// degraded mode without trawling the wire.
func buildUpgradeHandler(logger *slog.Logger, distributorBaseURL string, caPool *x509.CertPool) agent.UpgradeHandler {
	if agent.SigningPublicKey == "" {
		logger.Warn("self-upgrade disabled: AGENT_SIGNING_PUBKEY not baked into this build (build with -ldflags '-X github.com/WangYihang/Platypus/internal/agent.SigningPublicKey=<base64-pubkey>')")
		return nil
	}
	binPath, err := agent.ResolveBinaryPath()
	if err != nil || binPath == "" {
		logger.Warn("self-upgrade disabled: cannot resolve own binary path",
			slog.String("error", fmt.Sprintf("%v", err)))
		return nil
	}
	runner := &agent.UpgradeRunner{
		DistributorBaseURL:  distributorBaseURL,
		HTTPClient:          newDistributorHTTPClient(caPool),
		SigningPublicKeyB64: agent.SigningPublicKey,
		BinaryPath:          binPath,
		ExitFn:              os.Exit,
	}
	return runner.Handle
}

// newDistributorHTTPClient mirrors the TLS posture of EnrollClient:
// project CA in the root pool, http/1.1 only, generous timeout
// because artifact downloads can run minutes on slow links. Reused
// across all phases of one upgrade so the manifest, signature, and
// artifact GETs share a single TLS handshake.
func newDistributorHTTPClient(caPool *x509.CertPool) *http.Client {
	return &http.Client{
		Timeout: 10 * time.Minute, // big binaries on slow links
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				MinVersion: tls.VersionTLS12,
				RootCAs:    caPool,
				NextProtos: []string{"http/1.1"},
			},
		},
	}
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

// mandatoryCorePluginIDs is the set of system plugins that get
// installed regardless of the operator's allowlist. Today: just
// sys-info, because the Host overview page in the desktop UI is
// blank without it. Anything else needs explicit operator opt-in
// (either via --baseline-plugins at enroll time, or via the
// per-agent plugin REST surface afterwards).
var mandatoryCorePluginIDs = []string{
	"com.platypus.sys-info",
}

// expectedStreamTypes is the set of stream types the desktop UI
// drives in production: the 6 file-system / process / tunnel
// capabilities that used to live in 1194 lines of Go and now live
// in the wasm replacements under example/plugins/system/. We use
// this list at boot to warn the operator when the agent's baseline
// allowlist left one or more uncovered — server-initiated streams
// of those types will return plugin_not_installed at dispatch time,
// and the operator should know up front rather than discovering
// the gap by clicking around.
//
// AGENT_UPGRADE / RPC / PLUGIN_MGMT / PLUGIN_STREAM are deliberately
// not in this list — they're handled by built-ins (Upgrade /
// AgentRPCHandlers / PluginMgmt / DispatchPluginStream) that don't
// depend on a plugin claim.
var expectedStreamTypes = []string{
	"STREAM_TYPE_PROCESS_OPEN",
	"STREAM_TYPE_FILE_READ",
	"STREAM_TYPE_FILE_WRITE",
	"STREAM_TYPE_FILE_SCAN",
	"STREAM_TYPE_FILE_ARCHIVE",
	"STREAM_TYPE_TUNNEL_PULL",
}

// warnUncoveredStreams scans the plugin registry's claim table and
// emits one Warn line per expectedStreamTypes member that no
// enabled plugin owns. Non-fatal: the agent boots regardless. The
// warning gives operators a heads-up that a server-initiated
// stream of that type will come back as
// "plugin_not_installed: <id>" until they install the matching
// wasm plugin via the per-agent plugin REST.
func warnUncoveredStreams(logger *slog.Logger, reg *pluginrt.Registry) {
	claimed := reg.ClaimedStreamTypes()
	for _, t := range expectedStreamTypes {
		if _, ok := claimed[t]; ok {
			continue
		}
		logger.Warn("system stream uncovered; server invocations will return plugin_not_installed",
			slog.String("stream_type", t))
	}
}

// resolveBaselineAllowlist decides which embedded system plugins
// are eligible to install on this boot. Resolution order:
//
//  1. Persisted baseline.json (steady state — operator decision is
//     already captured on disk from a previous boot).
//  2. opts.BaselinePluginIDs (first-boot path — install bundle /
//     CLI flag tells us what the operator picked at enroll time).
//     Persists to baseline.json so future boots take path (1) and
//     no longer depend on the install bundle being present.
//  3. nil (no baseline at all — interpreted by the system bootstrap
//     as "install nothing", which when merged with mandatory core
//     produces the secure default of sys-info only).
//
// In every case the mandatory core is union'd in so the host
// overview is never blank. Returning nil here would be ambiguous
// (the system bootstrap reads nil as "install nothing"), so the
// secure default flows through this function as `mandatoryCorePluginIDs`.
func resolveBaselineAllowlist(logger *slog.Logger, identityDir string, fromOpts []string) []string {
	persisted, err := agent.LoadBaseline(identityDir)
	switch {
	case err == nil:
		logger.Info("baseline loaded from disk",
			slog.Int("count", len(persisted)),
			slog.Any("plugin_ids", persisted),
		)
		return mergeWithCore(persisted)
	case errors.Is(err, agent.ErrBaselineNotFound):
		// First boot: persist the operator's choice (or the empty
		// set) so the next boot doesn't re-evaluate the install bundle.
		if saveErr := agent.SaveBaseline(identityDir, fromOpts); saveErr != nil {
			// Non-fatal: the agent still boots, but every subsequent
			// boot will re-run this branch with the install token's
			// payload. Loud log so the operator notices.
			logger.Warn("baseline persist failed; will re-evaluate next boot",
				slog.String("error", saveErr.Error()))
		}
		logger.Info("baseline captured from install token",
			slog.Int("count", len(fromOpts)),
			slog.Any("plugin_ids", fromOpts),
		)
		return mergeWithCore(fromOpts)
	default:
		// File present but unreadable / corrupt. Don't silently fall
		// through to the install-bundle path — that would let a
		// truncated baseline.json silently re-enable plugins the
		// operator removed. Fall back to mandatory core only and log
		// loudly.
		logger.Error("baseline file unreadable; falling back to mandatory core",
			slog.String("error", err.Error()))
		return append([]string(nil), mandatoryCorePluginIDs...)
	}
}

// mergeWithCore returns the union of allow with the mandatory core.
// Order is preserved (operator-chosen ids first, mandatory core
// appended) so debug logs read well. Empty input yields just the
// mandatory core.
func mergeWithCore(allow []string) []string {
	seen := make(map[string]struct{}, len(allow)+len(mandatoryCorePluginIDs))
	out := make([]string, 0, len(allow)+len(mandatoryCorePluginIDs))
	for _, id := range allow {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	for _, id := range mandatoryCorePluginIDs {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// expandInstallBundle inspects opts.Token for the `pinst_` prefix
// and, when present, replaces opts.Token / opts.RemoteHost /
// opts.RemotePort with the bundle's contents and returns the bundle's
// CA PEM bytes for the caller to plug into the TLS path. Explicit
// --server (or PLATYPUS_SERVER) wins over the bundle's server endpoint.
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
	// Bundle baseline only fills in when the operator didn't already
	// pass --baseline-plugins / PLATYPUS_BASELINE_PLUGINS — explicit
	// CLI wins, mirroring the host/port precedence above.
	if len(opts.BaselinePluginIDs) == 0 && len(b.BaselinePluginIDs) > 0 {
		opts.BaselinePluginIDs = append([]string(nil), b.BaselinePluginIDs...)
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

// printUsage writes a short hint to stderr when bootstrap inputs
// (token / server address) are missing in a state that can't
// recover from persisted identity. kong already produces a full
// flags listing on --help; this is just the "you ran the binary
// with no args and have no enrolled state on disk" guidance.
func printUsage(err error) {
	fmt.Fprintln(os.Stderr, "platypus-agent — connect a host to a Platypus server")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "First-run bootstrap needs both an install token and a server address.")
	fmt.Fprintln(os.Stderr, "Pass them as: platypus-agent --server HOST:PORT <install-token>")
	fmt.Fprintln(os.Stderr, "Or as a single token from the Web UI: platypus-agent <HOST:PORT@token>")
	fmt.Fprintln(os.Stderr, "Or via env: "+options.EnvServerAddr+" + "+options.EnvInstallToken)
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "After enrollment, persisted identity under --data-dir lets re-runs need")
	fmt.Fprintln(os.Stderr, "neither value. Run with --help for the full flag listing.")
	if err != nil {
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Error:", err.Error())
	}
}
