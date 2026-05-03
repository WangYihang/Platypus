package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"google.golang.org/protobuf/proto"

	"github.com/WangYihang/Platypus/internal/core"
	"github.com/WangYihang/Platypus/internal/link"
	"github.com/WangYihang/Platypus/internal/log"
	"github.com/WangYihang/Platypus/internal/user"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// AgentPluginsHandler exposes per-agent plugin lifecycle operations
// over REST. Each endpoint translates the HTTP request into a
// PluginMgmtRequest, opens a STREAM_TYPE_PLUGIN_MGMT stream against
// the live agent link, drains the response, and renders JSON.
//
// Per-op flow lives in the sibling files:
//   handler_plugins_install.go   POST   .../plugins (streaming install)
//   handler_plugins_readonly.go  GET    .../plugins, GET .../logs (single-frame replies)
//   handler_plugins_mutate.go    DELETE / PATCH .../plugins/:plugin_id
type AgentPluginsHandler struct {
	svc     *core.AgentLinkService
	catalog MarketplaceCatalog // optional; when nil install_marketplace returns 503
	fetcher ArtefactFetcher    // optional; defaults to net/http when nil
}

// MarketplaceCatalog is the catalog interface install_marketplace
// needs. Defined here as the minimal contract (Get one row by
// plugin_id+version) so the dependency stays one-way: AgentPluginsHandler
// doesn't have to import internal/core/plugin.
type MarketplaceCatalog interface {
	Get(ctx context.Context, pluginID, version string) (MarketplaceRow, bool, error)
}

// MarketplaceRow is the subset of catalog fields the install path
// needs. Carries the fetch URLs + the sha256 the agent verifies +
// the publisher's signing key.
type MarketplaceRow struct {
	PluginID        string
	Version         string
	PublisherKeyID  string
	PublisherPubkey []byte
	WasmURL         string
	SignatureURL    string
	ManifestURL     string
	WasmSHA256Hex   string
}

// ArtefactFetcher is the seam install_marketplace uses to fetch the
// manifest / wasm / signature URLs. Production wires the default
// http.Client; tests substitute an in-memory map keyed by URL.
type ArtefactFetcher interface {
	Fetch(ctx context.Context, url string) ([]byte, error)
}

// NewAgentPluginsHandler binds the handler to the live link registry.
// Constructor lives next to its consumer in main.go so the wiring
// stays grep-able.
func NewAgentPluginsHandler(svc *core.AgentLinkService) *AgentPluginsHandler {
	return &AgentPluginsHandler{svc: svc}
}

// WithMarketplace decorates the handler with the catalog + fetcher
// the install_marketplace endpoint needs. Called from main.go after
// the catalog is constructed; without this the endpoint returns
// 503 "marketplace not configured".
func (h *AgentPluginsHandler) WithMarketplace(catalog MarketplaceCatalog, fetcher ArtefactFetcher) *AgentPluginsHandler {
	h.catalog = catalog
	h.fetcher = fetcher
	return h
}

// pluginMgmtTimeout caps how long a non-install REST request blocks
// waiting for the agent. Generous for slow links / cold-start of the
// wasm runtime, but bounded so a wedged agent doesn't hang the API
// server's connection pool. Install uses pluginInstallTimeout (longer)
// because verify_sig + extract + load on big plugins can be slow.
const pluginMgmtTimeout = 30 * time.Second

// pluginInstallTimeout caps the install-stream lifetime. Big enough
// that even multi-MB plugins cross-verifying on a slow disk finish
// inside the window.
const pluginInstallTimeout = 5 * time.Minute

// openMgmtStream is the common preamble for every plugin REST handler:
//   1. resolve the live link by agent_id
//   2. marshal the PluginMgmtRequest into the StreamHeader.metadata
//   3. open the stream
//
// Returns the stream + the resolved link session id (for audit lines)
// + the agent id ready to record. On any of these failures it has
// already written the HTTP response, so callers should just return.
func (h *AgentPluginsHandler) openMgmtStream(
	c *gin.Context, req *v2pb.PluginMgmtRequest, corrPrefix string,
) (io.ReadWriteCloser, string, bool) {
	agentID := c.Param("agent_id")
	sess, sessionID, ok := h.svc.GetWithSessionID(agentID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not connected"})
		return nil, "", false
	}
	meta, err := proto.Marshal(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "marshal mgmt request: " + err.Error()})
		return nil, "", false
	}
	stream, err := sess.Open(v2pb.StreamType_STREAM_TYPE_PLUGIN_MGMT, meta,
		corrPrefix+"-"+agentID+"-"+sessionID)
	if err != nil {
		log.Warn("agent.plugins.%s: open stream agent=%s err=%v", corrPrefix, agentID, err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "open mgmt stream: " + err.Error()})
		return nil, "", false
	}
	return stream, sessionID, true
}

// readSingleResponse drains a one-shot PluginMgmtResponse off the
// stream. Used by list / uninstall / enable / get_logs (everything
// except install, which is multi-frame). Returns the parsed response
// and any wire-level error.
func readSingleResponse(ctx context.Context, stream io.ReadWriteCloser) (*v2pb.PluginMgmtResponse, error) {
	type result struct {
		resp *v2pb.PluginMgmtResponse
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		var resp v2pb.PluginMgmtResponse
		err := link.ReadFrame(stream, &resp)
		ch <- result{&resp, err}
	}()
	select {
	case r := <-ch:
		return r.resp, r.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// withDetachedTimeout returns a context that lasts up to d, decoupled
// from the request context so a closed browser tab doesn't yank a
// mid-flight install. Same pattern as withRequestOrTimeout in the
// upgrade handler.
func withDetachedTimeout(d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), d)
}

// pluginInfoJSON is the JSON shape a PluginInfo proto renders to.
// Defined explicitly (rather than autogenerated from proto) so the
// REST schema is owned in this package and protobuf field-tag changes
// don't accidentally leak into the public API.
type pluginInfoJSON struct {
	ID                  string   `json:"id"`
	Name                string   `json:"name"`
	Version             string   `json:"version"`
	Author              string   `json:"author"`
	Enabled             bool     `json:"enabled"`
	GrantedCapabilities []string `json:"granted_capabilities"`
	InstallUnix         uint64   `json:"install_unix"`
	SourceURL           string   `json:"source_url,omitempty"`
	PublisherKeyID      string   `json:"publisher_key_id"`
}

func toPluginInfoJSON(p *v2pb.PluginInfo) pluginInfoJSON {
	return pluginInfoJSON{
		ID:                  p.GetId(),
		Name:                p.GetName(),
		Version:             p.GetVersion(),
		Author:              p.GetAuthor(),
		Enabled:             p.GetEnabled(),
		GrantedCapabilities: p.GetCapabilities(),
		InstallUnix:         p.GetInstallUnix(),
		SourceURL:           p.GetSourceUrl(),
		PublisherKeyID:      p.GetPublisherKeyId(),
	}
}

var _ = errors.New // reserved for richer error returns once batch callers land
var _ = fmt.Sprintf // reserved for the install handler's progress strings; keep import stable.

// RegisterV1AgentPluginRoutes mounts the per-agent plugin endpoints.
// Gated by RequireAuth + RequireProjectRole(admin) — only project
// admins can manage what runs on a fleet host.
//
// The `:agent_id` path param has to match the existing fs / terminal /
// upgrade routes under the same prefix so Gin's trie doesn't reject
// the registration on startup.
func RegisterV1AgentPluginRoutes(engine *gin.Engine, h *AgentPluginsHandler, rbac *RBAC) {
	admin := engine.Group("/api/v1/projects/:pid/agents")
	admin.Use(rbac.RequireAuth(), rbac.RequireProjectRole("pid", user.RoleAdmin))
	{
		admin.GET("/:agent_id/plugins", h.List)
		admin.POST("/:agent_id/plugins", h.Install)
		admin.POST("/:agent_id/plugins/install_marketplace", h.InstallFromMarketplace)
		admin.DELETE("/:agent_id/plugins/:plugin_id", h.Uninstall)
		admin.PATCH("/:agent_id/plugins/:plugin_id", h.Enable)
		admin.GET("/:agent_id/plugins/:plugin_id/logs", h.Logs)
	}
}

