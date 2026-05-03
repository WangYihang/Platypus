package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// List handles GET .../plugins. Returns the agent's installed-plugin
// inventory as a JSON array. Maps directly onto the agent's
// PluginListResponse.
func (h *AgentPluginsHandler) List(c *gin.Context) {
	req := &v2pb.PluginMgmtRequest{
		Op: &v2pb.PluginMgmtRequest_List{List: &v2pb.PluginListRequest{}},
	}
	stream, _, ok := h.openMgmtStream(c, req, "plugins-list")
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
	list := resp.GetList().GetPlugins()
	out := make([]pluginInfoJSON, 0, len(list))
	for _, p := range list {
		out = append(out, toPluginInfoJSON(p))
	}
	c.JSON(http.StatusOK, gin.H{"plugins": out})
}

// pluginLogEntryJSON mirrors PluginLogEntry. Same schema-isolation
// reason as pluginInfoJSON.
type pluginLogEntryJSON struct {
	UnixNano      int64  `json:"unix_nano"`
	Level         string `json:"level"`
	Message       string `json:"message"`
	CorrelationID string `json:"correlation_id,omitempty"`
}

// Logs handles GET .../plugins/:plugin_id/logs?tail=N. Pulls the most
// recent N log entries from the agent's per-plugin in-memory ring.
// tail=0 / unset returns everything currently buffered (capped
// agent-side).
func (h *AgentPluginsHandler) Logs(c *gin.Context) {
	pluginID := c.Param("plugin_id")
	tail := parseUintQuery(c, "tail", 0)

	req := &v2pb.PluginMgmtRequest{
		Op: &v2pb.PluginMgmtRequest_GetLogs{GetLogs: &v2pb.PluginGetLogsRequest{
			PluginId:  pluginID,
			TailLines: uint32(tail),
		}},
	}
	stream, _, ok := h.openMgmtStream(c, req, "plugins-logs")
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
	logs := resp.GetGetLogs()
	if logs.GetError() != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": logs.GetError()})
		return
	}
	out := make([]pluginLogEntryJSON, 0, len(logs.GetEntries()))
	for _, e := range logs.GetEntries() {
		out = append(out, pluginLogEntryJSON{
			UnixNano:      e.GetUnixNano(),
			Level:         e.GetLevel(),
			Message:       e.GetMessage(),
			CorrelationID: e.GetCorrelationId(),
		})
	}
	c.JSON(http.StatusOK, gin.H{"entries": out})
}

// parseUintQuery reads c.Query(key) as an unsigned int with a default.
// Defensive against operators passing junk in URL params; rather than
// 400ing on a typo, falls back to the default and lets the caller
// re-issue with the right value.
func parseUintQuery(c *gin.Context, key string, def uint64) uint64 {
	v := c.Query(key)
	if v == "" {
		return def
	}
	var out uint64
	for i := 0; i < len(v); i++ {
		ch := v[i]
		if ch < '0' || ch > '9' {
			return def
		}
		out = out*10 + uint64(ch-'0')
	}
	return out
}
