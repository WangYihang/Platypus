package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"time"

	"google.golang.org/protobuf/proto"

	agentplugin "github.com/WangYihang/Platypus/internal/agent/plugin"
	"github.com/WangYihang/Platypus/internal/link"
	"github.com/WangYihang/Platypus/internal/log"
	"github.com/WangYihang/Platypus/internal/storage"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// chooseCaps prefers the operator-authored grant set on a PluginSpec
// over the manifest's full declared set, then projects to the
// `[]string` shape protobuf's PluginInstallRequest expects. Sharing
// one helper keeps the "operator authority wins" rule in one place
// — and easy to audit if it ever needs to flip. The proto wire is
// the only `[]string` consumer of capability lists; everywhere
// else uses the typed CapabilityID.
func chooseCaps(specGrants []agentplugin.CapabilityID, manifestDeclared []agentplugin.CapabilityID) []string {
	chosen := specGrants
	if len(chosen) == 0 {
		chosen = manifestDeclared
	}
	return agentplugin.CapabilityIDsToStrings(chosen)
}

// mandatorySystemPlugins is the set every host gets regardless of the
// operator's baseline pick. sys-info is here because the host
// overview page renders blank without it; without sys-info the agent
// surfaces nothing about itself to the UI.
//
// Mutable from tests via TestMain only — production callers should
// treat this as a const.
var mandatorySystemPlugins = []string{
	"com.platypus.sys-info",
}

// pluginSyncTimeout caps how long the reconciliation goroutine runs.
// Generous enough for ~5 plugin installs end-to-end (verify_sig +
// extract + load) on a slow link, bounded so a wedged agent doesn't
// keep an orphan goroutine alive indefinitely.
const pluginSyncTimeout = 2 * time.Minute

// reconcileSystemPlugins brings the agent's installed plugin catalog
// in line with hostBaseline ∪ mandatorySystemPlugins. Idempotent: a
// re-run when every plugin is already installed is a single
// PluginMgmt:list and zero installs.
//
// systemBundle is rooted such that "publisher.pub" + each
// "<plugin_id>/<version>/{plugin.yaml,*.wasm,*.minisig}" path
// resolves with fs.ReadFile. The internal/server/sysplugins
// resolver picks between an operator's <data-dir>/system-plugins/
// override and the server binary's prebuilt embed.FS; this function
// can't tell which.
//
// agentOS / agentArch are the agent's reported runtime.GOOS /
// runtime.GOARCH (e.g. "linux"/"amd64"). The reconciler skips any
// plugin whose manifest declares non-empty os_targets/arch_targets
// that don't include the agent's pair. Empty / "" input is treated
// as "no filter" — preserves backward compat for fixtures that
// don't populate the host record yet.
//
// nil bundle → returns nil (reconciliation disabled). Missing
// entries in the bundle are logged and skipped; the agent surfaces
// plugin_not_installed when an RPC actually tries to use them.
// reconcileSystemPlugins is the agent-link reconciler. Takes the
// host's PluginSpec rows and pushes whatever's missing relative to
// the agent's reported catalog, threading per-spec
// config_overrides + granted_capabilities + schema_version onto
// each wire request so the agent's plugin runtime can hand them to
// OnInstall hooks. Empty / nil specs → only the mandatory core
// plugins get installed.
func reconcileSystemPlugins(
	ctx context.Context,
	sess pluginSyncSession,
	agentID string,
	hostSpecs []storage.PluginSpec,
	agentOS, agentArch string,
	systemBundle fs.FS,
) error {
	if systemBundle == nil {
		return nil
	}

	// Index the host's rich specs by plugin_id so the install loop
	// can look up per-plugin overrides (config_json, granted caps,
	// schema_version) without doing a linear scan per id.
	specByID := make(map[string]storage.PluginSpec, len(hostSpecs))
	hostBaseline := make([]string, 0, len(hostSpecs))
	for _, s := range hostSpecs {
		if s.PluginID == "" {
			continue
		}
		specByID[s.PluginID] = s
		hostBaseline = append(hostBaseline, s.PluginID)
	}
	desired := dedupeAppend(nil, hostBaseline)
	desired = dedupeAppend(desired, mandatorySystemPlugins)
	if len(desired) == 0 {
		return nil
	}

	installed, err := listInstalledViaMgmt(ctx, sess, agentID)
	if err != nil {
		return fmt.Errorf("list installed: %w", err)
	}
	// Track installed version per id so the reconciler upgrades in
	// place when the staged version is newer than what the agent
	// has. Same id, different version → re-run install: the agent's
	// catalog hot-swap handles the in-place replace (see
	// edge_cases_test.go: UpgradePathReplacesEarlierVersion).
	haveVersion := make(map[string]string, len(installed))
	for _, p := range installed {
		haveVersion[p.GetId()] = p.GetVersion()
	}

	pubkeyBytes, err := fs.ReadFile(systemBundle, "publisher.pub")
	if err != nil {
		return fmt.Errorf("read publisher.pub: %w", err)
	}
	catalog, err := enumerateSystemPlugins(systemBundle)
	if err != nil {
		return fmt.Errorf("enumerate system plugins: %w", err)
	}
	// enumerateSystemPlugins sorts by (id asc, version asc); the last
	// row for any id is the highest staged version.
	latestByID := make(map[string]systemPluginInfo, len(catalog))
	for _, p := range catalog {
		latestByID[p.ID] = p
	}

	for _, id := range desired {
		info, ok := latestByID[id]
		if !ok {
			log.Warn("plugin sync: %s not in system bundle; agent will return plugin_not_installed", id)
			continue
		}
		if !platformMatches(info.OSTargets, agentOS) {
			log.Info("plugin sync: skip %s for %s — os_targets=%v doesn't include %q",
				id, agentID, info.OSTargets, agentOS)
			continue
		}
		if !platformMatches(info.ArchTargets, agentArch) {
			log.Info("plugin sync: skip %s for %s — arch_targets=%v doesn't include %q",
				id, agentID, info.ArchTargets, agentArch)
			continue
		}
		if cur, present := haveVersion[id]; present && cur == info.Version {
			continue
		}
		action := "installed"
		if cur, present := haveVersion[id]; present && cur != info.Version {
			action = "upgraded from " + cur + " to"
		}
		// specByID may not have an entry for mandatory-core plugins
		// the operator didn't list explicitly. The zero PluginSpec
		// (empty config, empty caps) is the right default for them
		// — installOneViaMgmt falls back to manifest-declared caps.
		spec := specByID[id]
		if err := installOneViaMgmt(ctx, sess, agentID, info, spec, pubkeyBytes, systemBundle); err != nil {
			log.Warn("plugin sync: install %s@%s on %s: %v", id, info.Version, agentID, err)
			continue
		}
		log.Info("plugin sync: %s %s@%s on %s", action, id, info.Version, agentID)
	}
	return nil
}

// platformMatches reports whether `value` is allowed by `targets`.
// Empty `targets` means "no restriction" → always true. Empty
// `value` (agent didn't report os/arch yet — fresh enrol race)
// also returns true: better to push and let the wasm fail gracefully
// than to silently skip every plugin.
func platformMatches(targets []string, value string) bool {
	if len(targets) == 0 || value == "" {
		return true
	}
	for _, t := range targets {
		if t == value {
			return true
		}
	}
	return false
}

// pluginSyncSession is the minimal contract the reconciler needs from
// a link session. Defined as an interface so tests can substitute a
// fake without standing up yamux.
type pluginSyncSession interface {
	Open(streamType v2pb.StreamType, metadata []byte, correlationID string) (io.ReadWriteCloser, error)
}

func listInstalledViaMgmt(ctx context.Context, sess pluginSyncSession, agentID string) ([]*v2pb.PluginInfo, error) {
	req := &v2pb.PluginMgmtRequest{Op: &v2pb.PluginMgmtRequest_List{List: &v2pb.PluginListRequest{}}}
	meta, err := proto.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal list: %w", err)
	}
	stream, err := sess.Open(v2pb.StreamType_STREAM_TYPE_PLUGIN_MGMT, meta, "plugin-sync-list-"+agentID)
	if err != nil {
		return nil, fmt.Errorf("open list stream: %w", err)
	}
	defer func() { _ = stream.Close() }()
	resp, err := readSingleResponse(ctx, stream)
	if err != nil {
		return nil, fmt.Errorf("read list: %w", err)
	}
	if errMsg := resp.GetError(); errMsg != "" {
		return nil, errors.New(errMsg)
	}
	return resp.GetList().GetPlugins(), nil
}

func installOneViaMgmt(
	ctx context.Context,
	sess pluginSyncSession,
	agentID string,
	info systemPluginInfo,
	spec storage.PluginSpec,
	pubkeyBytes []byte,
	systemBundle fs.FS,
) error {
	manifestBytes, wasmBytes, sigBytes, err := readSystemBundle(systemBundle, info.ID, info.Version)
	if err != nil {
		return fmt.Errorf("read bundle: %w", err)
	}
	req := &v2pb.PluginMgmtRequest{
		Op: &v2pb.PluginMgmtRequest_Install{Install: &v2pb.PluginInstallRequest{
			PluginId:        info.ID,
			Version:         info.Version,
			PublisherPubkey: pubkeyBytes,
			Source: &v2pb.PluginInstallRequest_Inline{Inline: &v2pb.PluginInlineSource{
				WasmSizeBytes: uint64(len(wasmBytes)),
			}},
			// Capability source-of-truth: when the host's PluginSpec
			// carries an explicit grant set (the operator authored it
			// via the wizard / preset), trust that. Falls back to the
			// manifest's declared full set when the spec is empty —
			// the legacy "system-plugin trust boundary is publisher
			// signing key" posture, preserved for un-migrated specs.
			GrantedCapabilities: chooseCaps(spec.GrantedCapabilities, info.Capabilities),
			Actor:               "system:reconcile",
			ConfigJson:          spec.ConfigOverrides,
			ConfigSchemaVersion: int32(spec.SchemaVersion),
		}},
	}
	meta, err := proto.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal install: %w", err)
	}
	stream, err := sess.Open(v2pb.StreamType_STREAM_TYPE_PLUGIN_MGMT, meta,
		"plugin-sync-install-"+agentID+"-"+info.ID)
	if err != nil {
		return fmt.Errorf("open install stream: %w", err)
	}
	defer func() { _ = stream.Close() }()

	go pushInstallChunks(stream, manifestBytes, wasmBytes, sigBytes)

	progress, err := drainInstallProgress(ctx, stream)
	if err != nil {
		return fmt.Errorf("drain progress: %w", err)
	}
	if len(progress) == 0 {
		return errors.New("no progress frames")
	}
	last := progress[len(progress)-1]
	if last.Phase != v2pb.PluginInstallProgress_PHASE_INSTALLED.String() {
		return fmt.Errorf("ended in phase %s: %s", last.Phase, last.ErrorMessage)
	}
	return nil
}

// dedupeAppend appends ids onto out, skipping empty strings + ids
// already present. Order-preserving so logs read like the input.
func dedupeAppend(out []string, ids []string) []string {
	seen := make(map[string]bool, len(out))
	for _, id := range out {
		seen[id] = true
	}
	for _, id := range ids {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

// Compile-time assertion: *link.Session satisfies pluginSyncSession.
// The interface only exists to make the reconciler testable; the
// production caller passes the real session.
var _ pluginSyncSession = (*link.Session)(nil)
