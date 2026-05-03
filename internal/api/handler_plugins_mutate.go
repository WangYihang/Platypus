package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// uninstallRequest is the optional POST body for DELETE .../plugins/:id.
// Body is fully optional — the path param is the source of truth for
// the id; the body just carries flags.
type uninstallRequest struct {
	PurgeState bool `json:"purge_state"`
}

// Uninstall handles DELETE .../plugins/:plugin_id. Drops the plugin
// from the agent's catalog + on-disk install dir. When body.purge_state
// is true the plugin's per-id state/ subdir is removed too;
// otherwise it survives for a future reinstall (subject to the Phase 2
// state-relocation noted in registry_lifecycle.go).
func (h *AgentPluginsHandler) Uninstall(c *gin.Context) {
	pluginID := c.Param("plugin_id")
	claims, _ := ClaimsFromContext(c)

	var body uninstallRequest
	_ = c.ShouldBindJSON(&body) // body is optional

	req := &v2pb.PluginMgmtRequest{
		Op: &v2pb.PluginMgmtRequest_Uninstall{Uninstall: &v2pb.PluginUninstallRequest{
			PluginId:   pluginID,
			PurgeState: body.PurgeState,
			Actor:      "user:" + claims.UserID,
		}},
	}
	stream, _, ok := h.openMgmtStream(c, req, "plugins-uninstall")
	if !ok {
		return
	}
	defer func() { _ = stream.Close() }()

	ctx, cancel := withDetachedTimeout(pluginMgmtTimeout)
	defer cancel()

	resp, err := readSingleResponse(ctx, stream)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "read mgmt response: " + err.Error()})
		return
	}
	if resp.GetError() != "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": resp.GetError()})
		return
	}
	uninstall := resp.GetUninstall()
	if uninstall.GetError() != "" {
		// Agent rejected for a domain reason (unknown plugin id, etc.) —
		// surface as 4xx so the UI shows the message; 5xx is reserved
		// for transport failures.
		c.JSON(http.StatusBadRequest, gin.H{"error": uninstall.GetError()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok", "plugin_id": pluginID})
}

// enableRequest is the PATCH body for plugin enable/disable.
type enableRequest struct {
	Enabled bool `json:"enabled"`
}

// Enable handles PATCH .../plugins/:plugin_id with body {enabled: bool}.
// Disabled plugins stay loaded (cheap) but Invoke returns
// "plugin disabled" without entering the wasm runtime.
func (h *AgentPluginsHandler) Enable(c *gin.Context) {
	pluginID := c.Param("plugin_id")
	claims, _ := ClaimsFromContext(c)

	var body enableRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "body must be {\"enabled\": bool}"})
		return
	}

	req := &v2pb.PluginMgmtRequest{
		Op: &v2pb.PluginMgmtRequest_Enable{Enable: &v2pb.PluginEnableRequest{
			PluginId: pluginID,
			Enabled:  body.Enabled,
			Actor:    "user:" + claims.UserID,
		}},
	}
	stream, _, ok := h.openMgmtStream(c, req, "plugins-enable")
	if !ok {
		return
	}
	defer func() { _ = stream.Close() }()

	ctx, cancel := withDetachedTimeout(pluginMgmtTimeout)
	defer cancel()

	resp, err := readSingleResponse(ctx, stream)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "read mgmt response: " + err.Error()})
		return
	}
	if resp.GetError() != "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": resp.GetError()})
		return
	}
	enable := resp.GetEnable()
	if enable.GetError() != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": enable.GetError()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok", "plugin_id": pluginID, "enabled": body.Enabled})
}
