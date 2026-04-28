package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

// AdminRolesHandler exposes the read-only permission catalogue and the
// CRUD surface for roles. Every route is gated by RequireGlobalRole
// (admin) at the routing layer; the handler itself relies on that gate.
type AdminRolesHandler struct {
	db *storage.DB
}

func NewAdminRolesHandler(db *storage.DB) *AdminRolesHandler {
	return &AdminRolesHandler{db: db}
}

// --- Wire shapes ----------------------------------------------------------

type permissionDTO struct {
	Slug        string `json:"slug"`
	Resource    string `json:"resource"`
	Description string `json:"description"`
}

type roleDTO struct {
	Slug        string    `json:"slug"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	IsBuiltin   bool      `json:"is_builtin"`
	IsGlobal    bool      `json:"is_global"`
	IsProject   bool      `json:"is_project"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Permissions []string  `json:"permissions,omitempty"`
}

func toRoleDTO(r *storage.Role) roleDTO {
	return roleDTO{
		Slug:        r.Slug,
		Name:        r.Name,
		Description: r.Description,
		IsBuiltin:   r.IsBuiltin,
		IsGlobal:    r.IsGlobal,
		IsProject:   r.IsProject,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
		Permissions: r.Permissions,
	}
}

type createRoleRequest struct {
	Slug        string   `json:"slug"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	IsGlobal    bool     `json:"is_global"`
	IsProject   bool     `json:"is_project"`
	Permissions []string `json:"permissions"`
}

type updateRoleRequest struct {
	// Pointer-typed so a missing field stays unchanged. The
	// permissions slice is required if present (empty list = strip
	// everything, allowed for non-admin roles).
	Name        *string   `json:"name,omitempty"`
	Description *string   `json:"description,omitempty"`
	IsGlobal    *bool     `json:"is_global,omitempty"`
	IsProject   *bool     `json:"is_project,omitempty"`
	Permissions *[]string `json:"permissions,omitempty"`
}

// --- Permissions ----------------------------------------------------------

// ListPermissions handles GET /api/v1/admin/permissions. Returns the
// canonical catalogue ordered by (resource, slug) so the UI can group
// without an in-app sort.
func (h *AdminRolesHandler) ListPermissions(c *gin.Context) {
	perms, err := h.db.Permissions().List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list permissions"})
		return
	}
	out := make([]permissionDTO, 0, len(perms))
	for _, p := range perms {
		out = append(out, permissionDTO{
			Slug: p.Slug, Resource: p.Resource, Description: p.Description,
		})
	}
	c.JSON(http.StatusOK, gin.H{"permissions": out})
}

// --- Roles ----------------------------------------------------------------

func (h *AdminRolesHandler) ListRoles(c *gin.Context) {
	var f storage.RoleFilter
	if v := c.Query("is_global"); v == "true" {
		t := true
		f.IsGlobal = &t
	} else if v == "false" {
		fl := false
		f.IsGlobal = &fl
	}
	if v := c.Query("is_project"); v == "true" {
		t := true
		f.IsProject = &t
	} else if v == "false" {
		fl := false
		f.IsProject = &fl
	}
	roles, err := h.db.Roles().List(c.Request.Context(), f)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list roles"})
		return
	}
	out := make([]roleDTO, 0, len(roles))
	for _, r := range roles {
		out = append(out, toRoleDTO(r))
	}
	c.JSON(http.StatusOK, gin.H{"roles": out})
}

func (h *AdminRolesHandler) GetRole(c *gin.Context) {
	role, err := h.db.Roles().Get(c.Request.Context(), c.Param("slug"))
	if errors.Is(err, storage.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "role not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "get role"})
		return
	}
	c.JSON(http.StatusOK, toRoleDTO(role))
}

func (h *AdminRolesHandler) CreateRole(c *gin.Context) {
	var req createRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	req.Slug = strings.TrimSpace(req.Slug)
	req.Name = strings.TrimSpace(req.Name)
	if req.Slug == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "slug required"})
		return
	}
	if req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name required"})
		return
	}
	if !req.IsGlobal && !req.IsProject {
		c.JSON(http.StatusBadRequest, gin.H{"error": "role must be global or project (or both)"})
		return
	}
	if err := h.validatePermissions(c, req.Permissions); err != nil {
		return
	}

	now := time.Now().UTC()
	role := &storage.Role{
		Slug:        req.Slug,
		Name:        req.Name,
		Description: req.Description,
		IsBuiltin:   false,
		IsGlobal:    req.IsGlobal,
		IsProject:   req.IsProject,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := h.db.Roles().Create(c.Request.Context(), role, req.Permissions); err != nil {
		if isUniqueViolation(err) {
			c.JSON(http.StatusConflict, gin.H{"error": "role with this slug already exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	h.audit(c, "role.create", role.Slug, gin.H{
		"name":        role.Name,
		"is_global":   role.IsGlobal,
		"is_project":  role.IsProject,
		"permissions": req.Permissions,
	}, "success", "")
	role.Permissions = req.Permissions
	c.JSON(http.StatusCreated, toRoleDTO(role))
}

func (h *AdminRolesHandler) UpdateRole(c *gin.Context) {
	slug := c.Param("slug")
	var req updateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}

	ctx := c.Request.Context()
	role, err := h.db.Roles().Get(ctx, slug)
	if errors.Is(err, storage.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "role not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "get role"})
		return
	}

	// Apply non-permission edits. Builtins lock name + slot affinity
	// — only description and permissions are mutable. Custom roles
	// can edit everything.
	if req.Name != nil {
		if role.IsBuiltin {
			c.JSON(http.StatusBadRequest, gin.H{"error": "builtin role name cannot be changed"})
			return
		}
		role.Name = strings.TrimSpace(*req.Name)
	}
	if req.Description != nil {
		role.Description = *req.Description
	}
	if req.IsGlobal != nil {
		if role.IsBuiltin {
			c.JSON(http.StatusBadRequest, gin.H{"error": "builtin role slot affinity cannot be changed"})
			return
		}
		role.IsGlobal = *req.IsGlobal
	}
	if req.IsProject != nil {
		if role.IsBuiltin {
			c.JSON(http.StatusBadRequest, gin.H{"error": "builtin role slot affinity cannot be changed"})
			return
		}
		role.IsProject = *req.IsProject
	}
	if !role.IsGlobal && !role.IsProject {
		c.JSON(http.StatusBadRequest, gin.H{"error": "role must remain global or project"})
		return
	}

	perms := role.Permissions
	if req.Permissions != nil {
		perms = *req.Permissions
		if err := h.validatePermissions(c, perms); err != nil {
			return
		}
	}

	if err := h.db.Roles().Update(ctx, role, perms); err != nil {
		// The admin-protect trigger surfaces as a generic SQL error;
		// catch it via substring match so the API consumer gets a
		// targeted 400 instead of 500.
		if strings.Contains(err.Error(), "admin role cannot lose admin:*") {
			c.JSON(http.StatusBadRequest,
				gin.H{"error": "admin role cannot have its admin:* permissions removed"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	h.audit(c, "role.update", slug, gin.H{
		"permissions": perms,
	}, "success", "")
	role.Permissions = perms
	c.JSON(http.StatusOK, toRoleDTO(role))
}

func (h *AdminRolesHandler) DeleteRole(c *gin.Context) {
	slug := c.Param("slug")
	err := h.db.Roles().Delete(c.Request.Context(), slug)
	if errors.Is(err, storage.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "role not found"})
		return
	}
	if errors.Is(err, storage.ErrRoleBuiltin) {
		c.JSON(http.StatusConflict, gin.H{"error": "builtin role cannot be deleted"})
		return
	}
	if errors.Is(err, storage.ErrRoleInUse) {
		c.JSON(http.StatusConflict, gin.H{"error": "role still assigned to users or project members"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "delete role"})
		return
	}
	h.audit(c, "role.delete", slug, nil, "success", "")
	c.Status(http.StatusNoContent)
}

// validatePermissions enforces "every requested perm exists in the
// catalogue". Sends 400 on the response and returns a non-nil error
// so the caller bails. The DB FK would catch it too, but a precise
// message ("permission %q is not in the catalogue") helps API clients
// debug typos before they end up writing CHECK-violation parsers.
func (h *AdminRolesHandler) validatePermissions(c *gin.Context, perms []string) error {
	if len(perms) == 0 {
		return nil
	}
	known := map[string]struct{}{}
	all, err := h.db.Permissions().List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "permission catalogue lookup"})
		return err
	}
	for _, p := range all {
		known[p.Slug] = struct{}{}
	}
	for _, p := range perms {
		if _, ok := known[p]; !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "unknown permission: " + p})
			return errors.New("unknown permission")
		}
	}
	return nil
}

func (h *AdminRolesHandler) audit(c *gin.Context, action, slug string, details interface{}, outcome, errText string) {
	RecordActivity(c, ActivityInput{
		Category:    storage.CategoryAdmin,
		Action:      action,
		TargetType:  "role",
		TargetID:    slug,
		TargetLabel: slug,
		Outcome:     outcome,
		Error:       errText,
		Meta:        details,
	})
}

// RegisterV1AdminRolesRoutes mounts the admin RBAC surface. Gated by
// RequireGlobalRole(admin) — only the admin builtin (or any custom
// global role with admin's full permission set) reaches these routes.
func RegisterV1AdminRolesRoutes(engine *gin.Engine, h *AdminRolesHandler, rbac *RBAC) {
	g := engine.Group("/api/v1/admin")
	g.Use(rbac.RequireAuth(), rbac.RequireGlobalRole(user.RoleAdmin))
	{
		g.GET("/permissions", h.ListPermissions)
		g.GET("/roles", h.ListRoles)
		g.POST("/roles", h.CreateRole)
		g.GET("/roles/:slug", h.GetRole)
		g.PATCH("/roles/:slug", h.UpdateRole)
		g.DELETE("/roles/:slug", h.DeleteRole)
	}
}
