package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/ipinfo"
)

// IPInfoHandler exposes ad-hoc IP enrichment to the Web UI for IP
// values that don't ride along on a richer payload (sysinfo's
// public_ip, mesh link telemetry, etc.). List endpoints already
// embed *_info fields server-side; this is the escape hatch for
// the rest.
type IPInfoHandler struct{}

func NewIPInfoHandler() *IPInfoHandler { return &IPInfoHandler{} }

// Get handles GET /api/v1/ipinfo?ip=...
//
// The result mirrors what list endpoints inline as `*_info`. The
// underlying lookup is in-process and LRU-cached, so a 50-host
// dashboard polling at 1 Hz only exercises the xdb once per
// distinct address.
func (h *IPInfoHandler) Get(c *gin.Context) {
	raw := c.Query("ip")
	if raw == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ip query parameter is required"})
		return
	}
	c.JSON(http.StatusOK, ipinfo.Lookup(raw))
}

// RegisterV1IPInfoRoutes mounts the lookup endpoint. RequireAuth is
// the only gate — there's no project context here, and the data
// being returned is already public (geolocation of an arbitrary IP).
func RegisterV1IPInfoRoutes(engine *gin.Engine, h *IPInfoHandler, rbac *RBAC) {
	engine.GET("/api/v1/ipinfo", rbac.RequireAuth(), h.Get)
}
