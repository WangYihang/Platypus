package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/WangYihang/Platypus/internal/link"
	"github.com/WangYihang/Platypus/internal/log"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

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
// systemBundleDir/<systemPluginsDirName>/{publisher.pub,<id>/<v>/...}
// is the source of truth; missing entries here are logged and skipped
// (the agent will respond with plugin_not_installed when an RPC tries
// to use them, surfaced by the frontend's humanizeError).
func reconcileSystemPlugins(
	ctx context.Context,
	sess pluginSyncSession,
	agentID string,
	hostBaseline []string,
	systemBundleDir string,
) error {
	if systemBundleDir == "" {
		return nil
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

	root := filepath.Join(systemBundleDir, systemPluginsDirName)
	pubkeyBytes, err := os.ReadFile(filepath.Join(root, "publisher.pub"))
	if err != nil {
		return fmt.Errorf("read publisher.pub: %w", err)
	}
	catalog, err := enumerateSystemPlugins(root)
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
			log.Warn("plugin sync: %s not staged in <data_dir>/system-plugins/; agent will return plugin_not_installed", id)
			continue
		}
		if cur, present := haveVersion[id]; present && cur == info.Version {
			continue
		}
		action := "installed"
		if cur, present := haveVersion[id]; present && cur != info.Version {
			action = "upgraded from " + cur + " to"
		}
		if err := installOneViaMgmt(ctx, sess, agentID, info, pubkeyBytes, root); err != nil {
			log.Warn("plugin sync: install %s@%s on %s: %v", id, info.Version, agentID, err)
			continue
		}
		log.Info("plugin sync: %s %s@%s on %s", action, id, info.Version, agentID)
	}
	return nil
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
	pubkeyBytes []byte,
	root string,
) error {
	manifestBytes, wasmBytes, sigBytes, err := readSystemBundle(root, info.ID, info.Version)
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
			// At sync time the operator's pre-authorization is implicit
			// in baseline_plugin_ids — grant the full declared set
			// from the manifest. The system-plugin trust boundary is
			// "whoever signed publisher.pub", and the operator already
			// accepted that boundary at enroll time.
			GrantedCapabilities: info.Capabilities,
			Actor:               "system:reconcile",
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
