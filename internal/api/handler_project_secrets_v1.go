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

// ProjectSecretsHandler is the project-scoped CRUD surface for the
// at-rest encrypted secret store. Secrets here are referenced by id
// from plugin configs (PluginSpec.config_overrides carries
// {"$secret":"sec_<id>"} placeholders); this handler is purely the
// admin-facing identity / management surface for those rows.
//
// Plaintext only flows through this surface inbound on Create. Once
// the value is sealed under the project KEK, no API path will ever
// surface it again — the resolver path uses storage.Reveal directly,
// inside the install pipeline, so there's no "fetch this secret's
// value" REST endpoint by design.
type ProjectSecretsHandler struct {
	db *storage.DB
}

func NewProjectSecretsHandler(db *storage.DB) *ProjectSecretsHandler {
	return &ProjectSecretsHandler{db: db}
}

// --- Request / Response shapes -------------------------------------------

type createProjectSecretRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
	// Value is the plaintext, supplied once at create time. Echoed
	// back nowhere, never persisted in plaintext, and the field is
	// pointedly NOT included in any response shape.
	Value string `json:"value" binding:"required"`
}

// projectSecretItem is the redacted view returned by all GET / POST
// paths. Pointedly carries no nonce, ciphertext, or value field —
// the type system enforces redaction.
type projectSecretItem struct {
	SecretID      string     `json:"secret_id"`
	ProjectID     string     `json:"project_id"`
	Name          string     `json:"name"`
	Description   string     `json:"description,omitempty"`
	CreatedByUser string     `json:"created_by_user,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	LastUsedAt    *time.Time `json:"last_used_at,omitempty"`
	Revoked       bool       `json:"revoked"`
	RevokedAt     *time.Time `json:"revoked_at,omitempty"`
}

func toProjectSecretItem(s storage.ProjectSecretRedacted) projectSecretItem {
	return projectSecretItem{
		SecretID:      s.SecretID,
		ProjectID:     s.ProjectID,
		Name:          s.Name,
		Description:   s.Description,
		CreatedByUser: s.CreatedByUser,
		CreatedAt:     s.CreatedAt,
		LastUsedAt:    s.LastUsedAt,
		Revoked:       s.Revoked,
		RevokedAt:     s.RevokedAt,
	}
}

// --- Handlers ------------------------------------------------------------

// Create handles POST /projects/:pid/secrets. Body carries plaintext;
// response carries the redacted shape only. Plaintext is wiped from
// the request locals on the way out.
func (h *ProjectSecretsHandler) Create(c *gin.Context) {
	projectID := c.Param("pid")
	claims, _ := ClaimsFromContext(c)

	var req createProjectSecretRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad request: " + err.Error()})
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}
	if req.Value == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "value is required"})
		return
	}
	plaintext := []byte(req.Value)
	// Defense-in-depth: clear req.Value so subsequent Gin error
	// handlers / panic recovery dumps don't see it. Go strings are
	// immutable so we can only drop the reference; the bytes
	// themselves go away with GC.
	req.Value = ""

	row, err := h.db.ProjectSecrets().Create(
		c.Request.Context(),
		projectID, req.Name, req.Description, claims.UserID, plaintext,
	)
	if err != nil {
		if strings.Contains(err.Error(), "idx_project_secrets_active_name") || strings.Contains(err.Error(), "UNIQUE") {
			h.audit(c, "project_secret.create", "", projectID, req.Name, "denied", "duplicate name")
			c.JSON(http.StatusConflict, gin.H{
				"error": "a secret with that name is already active in this project (revoke the old one first)",
			})
			return
		}
		h.audit(c, "project_secret.create", "", projectID, req.Name, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "create secret"})
		return
	}
	h.audit(c, "project_secret.create", row.SecretID, projectID, req.Name, "success", "")
	c.JSON(http.StatusCreated, toProjectSecretItem(row.Redacted()))
}

// List handles GET /projects/:pid/secrets. Returns redacted rows
// newest-first, including revoked ones so the UI can render a
// rotation history.
func (h *ProjectSecretsHandler) List(c *gin.Context) {
	projectID := c.Param("pid")
	rows, err := h.db.ProjectSecrets().ListByProject(c.Request.Context(), projectID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list secrets"})
		return
	}
	out := make([]projectSecretItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, toProjectSecretItem(r))
	}
	c.JSON(http.StatusOK, gin.H{"secrets": out})
}

// Delete handles DELETE /projects/:pid/secrets/:sid. Idempotent:
// revoking an already-revoked secret returns 204 so operator
// scripts that retry don't blow up. Cross-project lookups are 404,
// matching the rest of the v1 admin surface's information-hiding
// posture.
func (h *ProjectSecretsHandler) Delete(c *gin.Context) {
	projectID := c.Param("pid")
	secretID := c.Param("sid")
	claims, _ := ClaimsFromContext(c)

	existing, err := h.db.ProjectSecrets().Get(c.Request.Context(), secretID)
	if errors.Is(err, storage.ErrNotFound) || (err == nil && existing.ProjectID != projectID) {
		h.audit(c, "project_secret.revoke", secretID, projectID, "", "denied", "not found")
		c.Status(http.StatusNotFound)
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "get secret"})
		return
	}
	reason := c.Query("reason")
	if err := h.db.ProjectSecrets().Revoke(
		c.Request.Context(), secretID, claims.UserID, reason,
	); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			c.Status(http.StatusNotFound)
			return
		}
		h.audit(c, "project_secret.revoke", secretID, projectID, "", "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "revoke secret"})
		return
	}
	h.audit(c, "project_secret.revoke", secretID, projectID, "", "success", "")
	c.Status(http.StatusNoContent)
}

func (h *ProjectSecretsHandler) audit(c *gin.Context, action, secretID, projectID, label, outcome, errText string) {
	pid := projectID
	RecordActivity(c, ActivityInput{
		ProjectID:   &pid,
		Category:    storage.CategoryAdmin,
		Action:      action,
		TargetType:  "project_secret",
		TargetID:    secretID,
		TargetLabel: label,
		Outcome:     outcome,
		Error:       errText,
	})
}

// RegisterV1ProjectSecretRoutes mounts the secret CRUD at the same
// admin gate the rest of the v1 admin surface uses. Project admins
// (and global admins) can manage secrets; viewers cannot. The
// install / preset paths consume secrets via the resolver, never
// via this REST surface — there is deliberately no "reveal" endpoint
// because surfacing plaintext over HTTP defeats the at-rest sealing.
func RegisterV1ProjectSecretRoutes(engine *gin.Engine, h *ProjectSecretsHandler, rbac *RBAC) {
	grp := engine.Group("/api/v1/projects/:pid/secrets")
	grp.Use(rbac.RequireAuth(), rbac.RequireProjectRole("pid", user.RoleAdmin))
	{
		grp.POST("", h.Create)
		grp.GET("", h.List)
		grp.DELETE("/:sid", h.Delete)
	}
}
