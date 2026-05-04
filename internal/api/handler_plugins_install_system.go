package api

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/agent/plugin"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// installSystemRequest is the POST body for the
// "install a system plugin from the server's local catalog" path.
//
// System plugins live under <data-dir>/system-plugins/<id>/<v>/ —
// the publisher (or production seeder) writes them there. Distinct
// from the marketplace path because:
//   · they're signed by the SYSTEM publisher key, not the
//     marketplace key (one publisher.pub at the system catalog root
//     vs. per-row pubkey in the marketplace)
//   · they ship as on-disk files, not URLs — no fetcher dance
//   · they're auto-installable at enroll time via the install
//     bundle's baseline_plugin_ids, so the post-enroll install
//     flow is the only "outside the wizard" path the operator has
//     to add one
//
// granted_capabilities mirrors the operator-confirmed dialog. For
// system plugins the convention is to pass exactly the manifest's
// declared set — they're pre-vetted by the system publisher key,
// so granting their declared caps wholesale matches the trust
// boundary the operator already accepted at enroll time.
type installSystemRequest struct {
	PluginID            string   `json:"plugin_id" binding:"required"`
	Version             string   `json:"version" binding:"required"`
	GrantedCapabilities []string `json:"granted_capabilities"`
}

// WithSystemBundle decorates the handler with the data-dir that
// holds the server's system-plugin catalog. Without this the
// install_system endpoint returns 503.
func (h *AgentPluginsHandler) WithSystemBundle(dataDir string) *AgentPluginsHandler {
	h.systemBundleDir = dataDir
	return h
}

// InstallFromSystem handles
// POST /api/v1/projects/:pid/agents/:agent_id/plugins/install_system.
//
// Reads the three artefacts (plugin.yaml + .wasm + .minisig) and
// the publisher pubkey from <data-dir>/system-plugins/, then
// streams them into the same agent install path the marketplace +
// inline endpoints use. The agent re-verifies sha256 + minisign,
// so a corrupt on-disk artefact still surfaces as the expected
// PHASE_FAILED inside the install progress stream rather than a
// silent partial install.
func (h *AgentPluginsHandler) InstallFromSystem(c *gin.Context) {
	if h.systemBundleDir == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "system plugin bundle not configured on this server",
		})
		return
	}
	claims, _ := ClaimsFromContext(c)

	var body installSystemRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Read the three artefacts off disk. Each read is bounded by the
	// surrounding pluginInstallTimeout via the agent stream; the disk
	// reads themselves are local and fast (a few MB of wasm).
	root := filepath.Join(h.systemBundleDir, systemPluginsDirName)
	pubkeyBytes, err := os.ReadFile(filepath.Join(root, "publisher.pub"))
	if err != nil {
		// publisher.pub at the catalog root is the trust anchor for
		// every bundle inside. Without it the agent would refuse
		// every install with signature_mismatch — bail early with a
		// clearer error.
		c.JSON(http.StatusFailedDependency, gin.H{
			"error": "system bundle missing publisher.pub at " + root + " — re-run the publisher",
		})
		return
	}

	manifestBytes, wasmBytes, sigBytes, err := readSystemBundle(root, body.PluginID, body.Version)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": fmt.Sprintf("system plugin %s@%s not staged on this server: %s",
				body.PluginID, body.Version, err.Error()),
		})
		return
	}

	// Stream into the same agent endpoint the marketplace path uses.
	// Same verify_sig + sha + load pipeline; the agent has no idea
	// the bytes came from local disk vs. an HTTP URL.
	req := &v2pb.PluginMgmtRequest{
		Op: &v2pb.PluginMgmtRequest_Install{Install: &v2pb.PluginInstallRequest{
			PluginId:        body.PluginID,
			Version:         body.Version,
			PublisherPubkey: pubkeyBytes,
			Source: &v2pb.PluginInstallRequest_Inline{Inline: &v2pb.PluginInlineSource{
				WasmSizeBytes: uint64(len(wasmBytes)),
			}},
			GrantedCapabilities: body.GrantedCapabilities,
			Actor:               "user:" + claims.UserID,
		}},
	}
	stream, _, opened := h.openMgmtStream(c, req, "plugins-install-system")
	if !opened {
		return
	}
	defer func() { _ = stream.Close() }()

	go pushInstallChunks(stream, manifestBytes, wasmBytes, sigBytes)

	ctx, cancel := withDetachedTimeout(pluginInstallTimeout)
	defer cancel()

	progress, drainErr := drainInstallProgress(ctx, stream)
	resp := installResponse{
		PluginID: body.PluginID,
		Version:  body.Version,
		Progress: progress,
	}
	if len(progress) > 0 {
		last := progress[len(progress)-1]
		switch {
		case last.Phase == v2pb.PluginInstallProgress_PHASE_INSTALLED.String():
			resp.Status = "installed"
		case last.Phase == v2pb.PluginInstallProgress_PHASE_FAILED.String():
			resp.Status = "failed"
		default:
			resp.Status = "in_progress"
		}
	} else {
		resp.Status = "in_progress"
	}
	if drainErr != nil && resp.Status == "in_progress" {
		c.JSON(http.StatusAccepted, resp)
		return
	}
	c.JSON(http.StatusOK, resp)
}

// readSystemBundle pulls plugin.yaml + the wasm + the minisig off
// the system catalog at <root>/<plugin_id>/<version>/. The wasm
// filename comes from the manifest's runtime.entry — we parse the
// manifest first to learn it. Errors carry enough context for the
// operator to find the gap (missing dir vs. bad manifest vs.
// missing artefact).
func readSystemBundle(root, pluginID, version string) (manifest, wasm, sig []byte, err error) {
	dir := filepath.Join(root, pluginID, version)
	manifest, err = os.ReadFile(filepath.Join(dir, "plugin.yaml"))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("read manifest: %w", err)
	}
	m, err := plugin.ParseManifest(manifest)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("parse manifest: %w", err)
	}
	wasm, err = os.ReadFile(filepath.Join(dir, m.Runtime.Entry))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("read wasm: %w", err)
	}
	sig, err = os.ReadFile(filepath.Join(dir, m.Runtime.Entry+".minisig"))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("read sig: %w", err)
	}
	_ = time.Now() // reserved if we ever want to log the staging mtime
	return manifest, wasm, sig, nil
}
