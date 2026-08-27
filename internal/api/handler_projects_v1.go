package api

import (
	"errors"
	"net/http"
	"regexp"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

// ProjectsHandler serves /projects and /projects/:pid/members routes.
// Handlers assume the routing layer has already applied the right auth
// gate via RBAC middleware (RequireAuth + either RequireGlobalRole or
// RequireProjectRole depending on the route).
type ProjectsHandler struct {
	db *storage.DB
}

func NewProjectsHandler(db *storage.DB) *ProjectsHandler {
	return &ProjectsHandler{db: db}
}

// slugPattern matches "prod", "staging-eu", "red_team_2026". Rejects slashes,
// colons, and whitespace so the value is safe to splice into routes and
// logs directly.
var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,62}$`)

type createProjectRequest struct {
	Name string `json:"name" binding:"required"`
	Slug string `json:"slug" binding:"required"`
}

type addMemberRequest struct {
	UserID string `json:"user_id" binding:"required"`
	Role   string `json:"role" binding:"required"`
}

type projectResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	CreatedAt time.Time `json:"created_at"`
	CreatedBy string    `json:"created_by"`
	// AISummariesEnabled mirrors the per-project opt-in for sending
	// finished terminal recordings to an LLM for a one-line summary.
	// Always present (not omitempty) so the settings UI can render the
	// toggle's real state rather than inferring off from a missing key.
	AISummariesEnabled bool `json:"ai_summaries_enabled"`
}

type memberResponse struct {
	UserID   string    `json:"user_id"`
	Username string    `json:"username"`
	Role     user.Role `json:"role"`
}

func toProjectResponse(p *storage.Project) projectResponse {
	return projectResponse{
		ID:                 p.ID,
		Name:               p.Name,
		Slug:               p.Slug,
		CreatedAt:          p.CreatedAt,
		CreatedBy:          p.CreatedBy,
		AISummariesEnabled: p.AISummariesEnabled,
	}
}

// Create handles POST /projects. Requires RoleAdmin globally.
func (h *ProjectsHandler) Create(c *gin.Context) {
	var req createProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !slugPattern.MatchString(req.Slug) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "slug must match " + slugPattern.String()})
		return
	}

	claims, _ := ClaimsFromContext(c)
	p := &storage.Project{
		ID:        uuid.NewString(),
		Name:      req.Name,
		Slug:      req.Slug,
		CreatedAt: time.Now().UTC(),
		CreatedBy: claims.UserID,
	}
	if err := h.db.Projects().Create(c.Request.Context(), p); err != nil {
		if isUniqueViolation(err) {
			c.JSON(http.StatusConflict, gin.H{"error": "slug already in use"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "create project"})
		return
	}
	RecordActivity(c, ActivityInput{
		ProjectID:   &p.ID,
		Category:    storage.CategoryProject,
		Action:      "project.create",
		TargetType:  "project",
		TargetID:    p.ID,
		TargetLabel: p.Slug,
		Meta:        map[string]any{"name": p.Name, "slug": p.Slug},
	})
	c.JSON(http.StatusCreated, toProjectResponse(p))
}

// List handles GET /projects and returns projects visible to the caller:
// global admins see everything; everyone else sees only projects they
// hold a project_members row in.
func (h *ProjectsHandler) List(c *gin.Context) {
	claims, ok := ClaimsFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "no claims"})
		return
	}
	projects, err := h.db.Projects().ListForUser(c.Request.Context(), claims.UserID, claims.Role)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list projects"})
		return
	}
	out := make([]projectResponse, 0, len(projects))
	for _, p := range projects {
		out = append(out, toProjectResponse(p))
	}
	c.JSON(http.StatusOK, gin.H{"projects": out})
}

// Get handles GET /projects/:pid. Gated by RequireProjectRole(viewer) so
// the reply is only sent to members (or global admins).
func (h *ProjectsHandler) Get(c *gin.Context) {
	p, err := h.db.Projects().GetByID(c.Request.Context(), c.Param("pid"))
	if errors.Is(err, storage.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "lookup project"})
		return
	}
	c.JSON(http.StatusOK, toProjectResponse(p))
}

// Delete handles DELETE /projects/:pid. Global-admin only.
func (h *ProjectsHandler) Delete(c *gin.Context) {
	pid := c.Param("pid")
	err := h.db.Projects().Delete(c.Request.Context(), pid)
	if errors.Is(err, storage.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "delete project"})
		return
	}
	RecordActivity(c, ActivityInput{
		ProjectID:   &pid,
		Category:    storage.CategoryProject,
		Action:      "project.delete",
		TargetType:  "project",
		TargetID:    pid,
		TargetLabel: pid,
	})
	c.Status(http.StatusNoContent)
}

// updateProjectRequest is the body for PATCH /projects/:pid. Pointer
// field so "absent" and "explicitly false" stay distinguishable — a
// PATCH that omits the key must not silently turn the opt-in off.
type updateProjectRequest struct {
	AISummariesEnabled *bool `json:"ai_summaries_enabled"`
}

// Update handles PATCH /projects/:pid. Currently only the AI-summary
// opt-in is settable. Gated at the project-admin level rather than
// viewer: enabling it sends terminal output to a third party, which is
// not a read-only decision.
func (h *ProjectsHandler) Update(c *gin.Context) {
	var req updateProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if req.AISummariesEnabled == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no updatable fields in body"})
		return
	}

	pid := c.Param("pid")
	err := h.db.Projects().SetAISummariesEnabled(c.Request.Context(), pid, *req.AISummariesEnabled)
	if errors.Is(err, storage.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "update project"})
		return
	}

	action := "project.ai_summaries.disable"
	if *req.AISummariesEnabled {
		action = "project.ai_summaries.enable"
	}
	RecordActivity(c, ActivityInput{
		ProjectID:   &pid,
		Category:    storage.CategoryProject,
		Action:      action,
		TargetType:  "project",
		TargetID:    pid,
		TargetLabel: pid,
	})

	p, err := h.db.Projects().GetByID(c.Request.Context(), pid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "reload project"})
		return
	}
	c.JSON(http.StatusOK, toProjectResponse(p))
}

// AddMember handles POST /projects/:pid/members. Gated by
// RequireProjectRole(admin) — project-admins can add/update members
// without being global admins.
func (h *ProjectsHandler) AddMember(c *gin.Context) {
	var req addMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	role, err := user.ParseRole(req.Role)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	pid := c.Param("pid")
	if err := h.db.Projects().AddMember(c.Request.Context(), pid, req.UserID, role); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "add member"})
		return
	}
	RecordActivity(c, ActivityInput{
		ProjectID:   &pid,
		Category:    storage.CategoryProject,
		Action:      "project.member_add",
		TargetType:  "user",
		TargetID:    req.UserID,
		TargetLabel: req.UserID,
		Meta:        map[string]any{"role": string(role)},
	})
	c.Status(http.StatusNoContent)
}

func (h *ProjectsHandler) RemoveMember(c *gin.Context) {
	pid := c.Param("pid")
	uid := c.Param("uid")
	err := h.db.Projects().RemoveMember(c.Request.Context(), pid, uid)
	if errors.Is(err, storage.ErrNotFound) {
		c.Status(http.StatusNotFound)
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "remove member"})
		return
	}
	RecordActivity(c, ActivityInput{
		ProjectID:   &pid,
		Category:    storage.CategoryProject,
		Action:      "project.member_remove",
		TargetType:  "user",
		TargetID:    uid,
		TargetLabel: uid,
	})
	c.Status(http.StatusNoContent)
}

func (h *ProjectsHandler) ListMembers(c *gin.Context) {
	members, err := h.db.Projects().ListMembers(c.Request.Context(), c.Param("pid"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list members"})
		return
	}
	out := make([]memberResponse, 0, len(members))
	for _, m := range members {
		out = append(out, memberResponse{
			UserID:   m.UserID,
			Username: m.Username,
			Role:     m.Role,
		})
	}
	c.JSON(http.StatusOK, gin.H{"members": out})
}

// RegisterV1ProjectsRoutes mounts the /projects surface. The auth model
// varies per route, so the middleware chain is expressed here rather than
// on a single shared group.
func RegisterV1ProjectsRoutes(engine *gin.Engine, h *ProjectsHandler, rbac *RBAC) {
	// Authenticated routes: list + create.
	engine.GET("/api/v1/projects",
		rbac.RequireAuth(),
		h.List,
	)
	engine.POST("/api/v1/projects",
		rbac.RequireAuth(),
		rbac.RequireGlobalRole(user.RoleAdmin),
		h.Create,
	)

	// Per-project routes: viewer for reads, admin for member management.
	engine.GET("/api/v1/projects/:pid",
		rbac.RequireAuth(),
		rbac.RequireProjectRole("pid", user.RoleViewer),
		h.Get,
	)
	engine.DELETE("/api/v1/projects/:pid",
		rbac.RequireAuth(),
		rbac.RequireGlobalRole(user.RoleAdmin),
		h.Delete,
	)
	engine.PATCH("/api/v1/projects/:pid",
		rbac.RequireAuth(),
		rbac.RequireProjectRole("pid", user.RoleAdmin),
		h.Update,
	)
	engine.GET("/api/v1/projects/:pid/members",
		rbac.RequireAuth(),
		rbac.RequireProjectRole("pid", user.RoleViewer),
		h.ListMembers,
	)
	engine.POST("/api/v1/projects/:pid/members",
		rbac.RequireAuth(),
		rbac.RequireProjectRole("pid", user.RoleAdmin),
		h.AddMember,
	)
	engine.DELETE("/api/v1/projects/:pid/members/:uid",
		rbac.RequireAuth(),
		rbac.RequireProjectRole("pid", user.RoleAdmin),
		h.RemoveMember,
	)
}
