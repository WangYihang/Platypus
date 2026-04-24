package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/settings"
	"github.com/WangYihang/Platypus/internal/user"
)

// AdminSettingsHandler exposes the admin-settings registry over HTTP.
// Every route is global-admin-gated and audited; see
// RegisterV1AdminSettingsRoutes for the wiring.
type AdminSettingsHandler struct {
	reg *settings.Registry
}

// NewAdminSettingsHandler wraps reg so it can be mounted on a gin engine.
func NewAdminSettingsHandler(reg *settings.Registry) *AdminSettingsHandler {
	return &AdminSettingsHandler{reg: reg}
}

// RegisterV1AdminSettingsRoutes mounts the admin-settings CRUD surface.
// All routes require a global admin JWT (mirrors the /api/v1/users
// pattern). Settings writes always go to the DB; reads are cached and
// invalidated on write, so successive calls return the newest value.
func RegisterV1AdminSettingsRoutes(engine *gin.Engine, h *AdminSettingsHandler, rbac *RBAC) {
	grp := engine.Group("/api/v1/admin/settings")
	grp.Use(rbac.RequireAuth(), rbac.RequireGlobalRole(user.RoleAdmin))
	{
		grp.GET("", h.List)
		grp.PUT("/:key", h.Update)
		grp.DELETE("/:key", h.Reset)
	}
}

// List returns a descriptor for every registered setting, including
// its default / YAML / DB / effective value and the derived source.
func (h *AdminSettingsHandler) List(c *gin.Context) {
	descs, err := h.reg.DescribeAll(c.Request.Context())
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError,
			gin.H{"error": "describe settings"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"settings": descs})
}

// Update accepts a JSON body of shape {"value": <raw-json>} where the
// inner value is already JSON-encoded to match the registered type.
// On success the DB row is upserted, the cache is invalidated, and an
// audit row is emitted; callers see 204 No Content.
func (h *AdminSettingsHandler) Update(c *gin.Context) {
	key := c.Param("key")
	claims, ok := ClaimsFromContext(c)
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "no auth"})
		return
	}

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "read body"})
		return
	}
	var req struct {
		Value json.RawMessage `json:"value"`
	}
	if len(body) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest,
				gin.H{"error": "invalid JSON body"})
			return
		}
	}
	if len(req.Value) == 0 {
		c.AbortWithStatusJSON(http.StatusBadRequest,
			gin.H{"error": "missing value"})
		return
	}
	rawVal := req.Value

	err = h.reg.Set(c.Request.Context(), key, string(rawVal), claims.UserID)
	switch {
	case errors.Is(err, settings.ErrUnknownKey):
		c.AbortWithStatusJSON(http.StatusNotFound,
			gin.H{"error": "unknown setting key"})
		return
	case errors.Is(err, settings.ErrBadValue):
		c.AbortWithStatusJSON(http.StatusBadRequest,
			gin.H{"error": err.Error()})
		return
	case err != nil:
		c.AbortWithStatusJSON(http.StatusInternalServerError,
			gin.H{"error": "set"})
		return
	}
	c.Status(http.StatusNoContent)
}

// Reset drops the DB override for key so subsequent reads fall back
// to YAML / hardcoded default.
func (h *AdminSettingsHandler) Reset(c *gin.Context) {
	key := c.Param("key")
	claims, ok := ClaimsFromContext(c)
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "no auth"})
		return
	}
	err := h.reg.Reset(c.Request.Context(), key, claims.UserID)
	switch {
	case errors.Is(err, settings.ErrUnknownKey):
		c.AbortWithStatusJSON(http.StatusNotFound,
			gin.H{"error": "unknown setting key"})
		return
	case err != nil:
		c.AbortWithStatusJSON(http.StatusInternalServerError,
			gin.H{"error": "reset"})
		return
	}
	c.Status(http.StatusNoContent)
}
