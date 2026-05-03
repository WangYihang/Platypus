package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	corepkg "github.com/WangYihang/Platypus/internal/core/plugin"
	"github.com/WangYihang/Platypus/internal/user"
)

// MarketplaceHandler exposes the server-side catalog cache over REST.
// Authenticated read endpoints (no admin gate — every authenticated
// user can browse the marketplace) plus an admin-only refresh
// trigger so an operator doesn't have to wait for the periodic
// refresh worker to see a freshly-published plugin.
//
// All endpoints serve from the cache, never from the upstream index
// directly: a slow or down index doesn't degrade the operator UI's
// browsing latency.
type MarketplaceHandler struct {
	catalog *corepkg.Catalog
}

func NewMarketplaceHandler(catalog *corepkg.Catalog) *MarketplaceHandler {
	return &MarketplaceHandler{catalog: catalog}
}

// Search handles GET /api/v1/marketplace/plugins?q=…. Empty q
// returns everything, sorted by name. Filter is a substring match
// against the plugin's display name.
func (h *MarketplaceHandler) Search(c *gin.Context) {
	q := c.Query("q")
	rows, err := h.catalog.Search(c.Request.Context(), q)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if rows == nil {
		rows = []corepkg.PluginRow{}
	}
	c.JSON(http.StatusOK, gin.H{"plugins": rows})
}

// Versions handles GET /api/v1/marketplace/plugins/:id/versions.
func (h *MarketplaceHandler) Versions(c *gin.Context) {
	id := c.Param("id")
	rows, err := h.catalog.Versions(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if rows == nil {
		rows = []corepkg.PluginRow{}
	}
	c.JSON(http.StatusOK, gin.H{"versions": rows})
}

// Status handles GET /api/v1/marketplace/status. Returns the
// last-sync row so the UI can render "Last refreshed at HH:MM
// (status: ok / http_error / parse_error)".
func (h *MarketplaceHandler) Status(c *gin.Context) {
	rs, ok, err := h.catalog.LastRefresh(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !ok {
		c.JSON(http.StatusOK, gin.H{"status": "never_synced"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": rs})
}

// Refresh handles POST /api/v1/marketplace/refresh. Synchronous +
// admin-only — the operator sees the result of their click. The
// background worker (started in main.go) handles periodic refreshes
// independently.
func (h *MarketplaceHandler) Refresh(c *gin.Context) {
	n, err := h.catalog.Refresh(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"plugin_count": n})
}

// RegisterV1MarketplaceRoutes mounts the four endpoints. Browse +
// status are auth-only (any logged-in user can window-shop the
// marketplace); refresh is admin-only (it hits the network +
// rebuilds DB rows). Versions is auth-only — same posture as Search.
func RegisterV1MarketplaceRoutes(engine *gin.Engine, h *MarketplaceHandler, rbac *RBAC) {
	read := engine.Group("/api/v1/marketplace")
	read.Use(rbac.RequireAuth())
	{
		read.GET("/plugins", h.Search)
		read.GET("/plugins/:id/versions", h.Versions)
		read.GET("/status", h.Status)
	}
	admin := engine.Group("/api/v1/marketplace")
	admin.Use(rbac.RequireAuth(), rbac.RequireGlobalRole(user.RoleAdmin))
	{
		admin.POST("/refresh", h.Refresh)
	}
}
