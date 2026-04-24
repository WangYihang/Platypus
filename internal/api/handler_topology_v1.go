package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

// TopologyHandler serves the per-project mesh + machine topology
// snapshot used by the Topology page. Live snapshots come from the
// in-memory aggregator; historical stats come from the time-series
// tables populated by core.StartTopologyStream.
type TopologyHandler struct {
	db *storage.DB
}

// NewTopologyHandler wires in the storage handle for history queries.
// db may be nil when the server is started without persistence; the
// history endpoints then degrade to empty responses.
func NewTopologyHandler(db *storage.DB) *TopologyHandler {
	return &TopologyHandler{db: db}
}

// Get handles GET /api/v1/projects/:pid/topology.
//
// After the v1 deletion pass the live machine + mesh aggregator
// (core.BuildTopologySnapshot) is gone. The endpoint now returns
// just the historical stats the storage layer keeps; real-time
// deltas flow via /notify topology.* events. A follow-up commit
// will rebuild a v2-native snapshot producer on top of
// AgentLinkService.
//
// @Summary     Project topology snapshot (history-only)
// @Tags        topology
// @Produce     json
// @Param       pid path string true "Project ID"
// @Security    BearerAuth
// @Router      /api/v1/projects/{pid}/topology [get]
func (h *TopologyHandler) Get(c *gin.Context) {
	_ = c.Param("pid")
	c.JSON(http.StatusOK, gin.H{
		"nodes": []any{},
		"links": []any{},
	})
}

// linkHistoryPoint is the JSON shape for a single row returned by
// GetLinkHistory. RttNs is only set when the sample recorded an RTT.
type linkHistoryPoint struct {
	At       time.Time `json:"at"`
	BytesIn  int64     `json:"bytes_in"`
	BytesOut int64     `json:"bytes_out"`
	MsgsIn   int64     `json:"msgs_in"`
	MsgsOut  int64     `json:"msgs_out"`
	RTTNs    *int64    `json:"rtt_ns,omitempty"`
}

// machineHistoryPoint is the JSON shape for a single row returned by
// GetMachineHistory.
type machineHistoryPoint struct {
	At         time.Time `json:"at"`
	CPUPercent *float64  `json:"cpu_percent,omitempty"`
	MemPercent *float64  `json:"mem_percent,omitempty"`
}

// GetLinkHistory handles
// GET /api/v1/projects/:pid/topology/links/:a/:b/stats?since=&until=&max=
//
// since / until are RFC3339 timestamps; when omitted the handler
// defaults to [now-1h, now]. max is an optional point cap — when set
// the repo thins the result to at most that many evenly-spaced
// samples.
func (h *TopologyHandler) GetLinkHistory(c *gin.Context) {
	if h.db == nil {
		c.JSON(http.StatusOK, gin.H{"points": []linkHistoryPoint{}})
		return
	}
	opts := parseHistoryOpts(c)
	rows, err := h.db.MeshStats().ListLinkHistory(
		c.Request.Context(),
		c.Param("pid"), c.Param("a"), c.Param("b"),
		opts,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list link history"})
		return
	}
	out := make([]linkHistoryPoint, 0, len(rows))
	for _, r := range rows {
		out = append(out, linkHistoryPoint{
			At: r.At, BytesIn: r.BytesIn, BytesOut: r.BytesOut,
			MsgsIn: r.MsgsIn, MsgsOut: r.MsgsOut, RTTNs: r.RTTNs,
		})
	}
	c.JSON(http.StatusOK, gin.H{"points": out})
}

// GetMachineHistory handles
// GET /api/v1/projects/:pid/topology/machines/:hid/stats?since=&until=&max=
func (h *TopologyHandler) GetMachineHistory(c *gin.Context) {
	if h.db == nil {
		c.JSON(http.StatusOK, gin.H{"points": []machineHistoryPoint{}})
		return
	}
	opts := parseHistoryOpts(c)
	rows, err := h.db.MeshStats().ListMachineHistory(
		c.Request.Context(), c.Param("hid"), opts,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list machine history"})
		return
	}
	out := make([]machineHistoryPoint, 0, len(rows))
	for _, r := range rows {
		out = append(out, machineHistoryPoint{
			At: r.At, CPUPercent: r.CPUPercent, MemPercent: r.MemPercent,
		})
	}
	c.JSON(http.StatusOK, gin.H{"points": out})
}

func parseHistoryOpts(c *gin.Context) storage.LinkHistoryOpts {
	now := time.Now().UTC()
	since := now.Add(-1 * time.Hour)
	until := now
	if v := c.Query("since"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			since = t
		}
	}
	if v := c.Query("until"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			until = t
		}
	}
	maxPoints := 0
	if v := c.Query("max"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxPoints = n
		}
	}
	return storage.LinkHistoryOpts{Since: since, Until: until, MaxPoints: maxPoints}
}

// RegisterV1TopologyRoutes mounts the per-project topology routes.
// Viewer role is enough for every endpoint — all are read-only.
func RegisterV1TopologyRoutes(engine *gin.Engine, h *TopologyHandler, rbac *RBAC) {
	grp := engine.Group("/api/v1/projects/:pid/topology")
	grp.Use(rbac.RequireAuth(), rbac.RequireProjectRole("pid", user.RoleViewer))
	{
		grp.GET("", h.Get)
		grp.GET("/links/:a/:b/stats", h.GetLinkHistory)
		grp.GET("/machines/:hid/stats", h.GetMachineHistory)
	}
}
