package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/core"
	"github.com/WangYihang/Platypus/internal/user"
)

// TopologyHandler serves the per-project mesh + machine topology
// snapshot used by the Topology page. All state is read from the
// live core context — no storage writes happen here.
type TopologyHandler struct{}

// NewTopologyHandler returns a handler instance. There's no state,
// but keeping the constructor matches the rest of the v2 handlers.
func NewTopologyHandler() *TopologyHandler { return &TopologyHandler{} }

// Get handles GET /api/v1/projects/:pid/topology.
//
// @Summary     Project topology snapshot
// @Description Returns the full machine + mesh graph for a project, including live sysinfo (CPU / memory / OS) and per-link traffic counters. The frontend subscribes to /notify topology.* events for deltas.
// @Tags        topology
// @Produce     json
// @Param       pid path string true "Project ID"
// @Security    BearerAuth
// @Success     200 {object} core.TopologySnapshot
// @Router      /api/v1/projects/{pid}/topology [get]
func (h *TopologyHandler) Get(c *gin.Context) {
	pid := c.Param("pid")
	snap, err := core.BuildTopologySnapshot(c.Request.Context(), pid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, snap)
}

// RegisterV1TopologyRoutes mounts /api/v1/projects/:pid/topology.
// Viewer role is enough — the snapshot is read-only.
func RegisterV1TopologyRoutes(engine *gin.Engine, h *TopologyHandler, rbac *RBAC) {
	engine.GET("/api/v1/projects/:pid/topology",
		rbac.RequireAuth(),
		rbac.RequireProjectRole("pid", user.RoleViewer),
		h.Get,
	)
}
