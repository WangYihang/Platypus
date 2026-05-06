package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/core"
	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

// EnrollmentPresetsHandler serves the project-scoped CRUD for saved
// enrollment configurations. Presets are operator inputs only — every
// preset use still mints a fresh single-use install token via the
// install-artifact flow — so this handler stays pure storage with
// audit logging on writes.
type EnrollmentPresetsHandler struct {
	db *storage.DB
}

func NewEnrollmentPresetsHandler(db *storage.DB) *EnrollmentPresetsHandler {
	return &EnrollmentPresetsHandler{db: db}
}

// --- Request / Response shapes -------------------------------------------

type upsertEnrollmentPresetRequest struct {
	Name                string   `json:"name" binding:"required"`
	Description         string   `json:"description"`
	ServerEndpoint      string   `json:"server_endpoint"`
	TargetOS            string   `json:"target_os"`
	TargetArch          string   `json:"target_arch"`
	TTLSeconds          *int     `json:"ttl_seconds"`
	PATMaxUses          *int     `json:"pat_max_uses"`
	AutoApprove         bool     `json:"auto_approve"`
	SkipTLSVerification bool     `json:"skip_tls_verification"`
	BaselinePluginIDs   []string `json:"baseline_plugin_ids"`
	PATDescription      string   `json:"pat_description"`
}

type enrollmentPresetItem struct {
	PresetID            string    `json:"preset_id"`
	ProjectID           string    `json:"project_id"`
	Name                string    `json:"name"`
	Description         string    `json:"description,omitempty"`
	ServerEndpoint      string    `json:"server_endpoint,omitempty"`
	TargetOS            string    `json:"target_os,omitempty"`
	TargetArch          string    `json:"target_arch,omitempty"`
	TTLSeconds          *int      `json:"ttl_seconds,omitempty"`
	PATMaxUses          *int      `json:"pat_max_uses,omitempty"`
	AutoApprove         bool      `json:"auto_approve"`
	SkipTLSVerification bool      `json:"skip_tls_verification"`
	BaselinePluginIDs   []string  `json:"baseline_plugin_ids,omitempty"`
	PATDescription      string    `json:"pat_description,omitempty"`
	IsSeed              bool      `json:"is_seed"`
	CreatedByUser       string    `json:"created_by_user,omitempty"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

func toEnrollmentPresetItem(p *storage.EnrollmentPreset) enrollmentPresetItem {
	return enrollmentPresetItem{
		PresetID:            p.PresetID,
		ProjectID:           p.ProjectID,
		Name:                p.Name,
		Description:         p.Description,
		ServerEndpoint:      p.ServerEndpoint,
		TargetOS:            p.TargetOS,
		TargetArch:          p.TargetArch,
		TTLSeconds:          p.TTLSeconds,
		PATMaxUses:          p.PATMaxUses,
		AutoApprove:         p.AutoApprove,
		SkipTLSVerification: p.SkipTLSVerification,
		BaselinePluginIDs:   p.BaselinePluginIDs,
		PATDescription:      p.PATDescription,
		IsSeed:              p.IsSeed,
		CreatedByUser:       p.CreatedByUser,
		CreatedAt:           p.CreatedAt,
		UpdatedAt:           p.UpdatedAt,
	}
}

// --- Handlers ------------------------------------------------------------

// Create handles POST /projects/:pid/enrollment-presets.
func (h *EnrollmentPresetsHandler) Create(c *gin.Context) {
	projectID := c.Param("pid")
	claims, _ := ClaimsFromContext(c)

	var req upsertEnrollmentPresetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad request: " + err.Error()})
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}

	id, err := storage.NewPresetID()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "id gen"})
		return
	}
	now := time.Now().UTC()
	p := &storage.EnrollmentPreset{
		PresetID:            id,
		ProjectID:           projectID,
		Name:                strings.TrimSpace(req.Name),
		Description:         req.Description,
		ServerEndpoint:      strings.TrimSpace(req.ServerEndpoint),
		TargetOS:            req.TargetOS,
		TargetArch:          req.TargetArch,
		TTLSeconds:          req.TTLSeconds,
		PATMaxUses:          req.PATMaxUses,
		AutoApprove:         req.AutoApprove,
		SkipTLSVerification: req.SkipTLSVerification,
		BaselinePluginIDs:   req.BaselinePluginIDs,
		PATDescription:      req.PATDescription,
		CreatedByUser:       claims.UserID,
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	if err := h.db.EnrollmentPresets().Create(c.Request.Context(), p); err != nil {
		// SQLite's UNIQUE error doesn't get a distinct sentinel here, so
		// best-effort match: the index name is the only stable string we
		// can rely on across drivers.
		if strings.Contains(err.Error(), "idx_enrollment_presets_name") || strings.Contains(err.Error(), "UNIQUE") {
			h.audit(c, "enrollment_preset.create", id, projectID, req, "denied", "duplicate name")
			c.JSON(http.StatusConflict, gin.H{"error": "a preset with that name already exists in this project"})
			return
		}
		h.audit(c, "enrollment_preset.create", id, projectID, req, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "create preset"})
		return
	}
	h.audit(c, "enrollment_preset.create", id, projectID, req, "success", "")
	c.JSON(http.StatusCreated, toEnrollmentPresetItem(p))
}

// List handles GET /projects/:pid/enrollment-presets.
func (h *EnrollmentPresetsHandler) List(c *gin.Context) {
	projectID := c.Param("pid")
	rows, err := h.db.EnrollmentPresets().ListByProject(c.Request.Context(), projectID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list presets"})
		return
	}
	out := make([]enrollmentPresetItem, 0, len(rows))
	for _, p := range rows {
		out = append(out, toEnrollmentPresetItem(p))
	}
	c.JSON(http.StatusOK, gin.H{"presets": out})
}

// Get handles GET /projects/:pid/enrollment-presets/:rid.
func (h *EnrollmentPresetsHandler) Get(c *gin.Context) {
	projectID := c.Param("pid")
	presetID := c.Param("rid")
	p, err := h.db.EnrollmentPresets().Get(c.Request.Context(), presetID)
	if errors.Is(err, storage.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "preset not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "get preset"})
		return
	}
	// Cross-project lookup is a 404, not a 403 — same information-hiding
	// posture the rest of the v1 admin surface uses.
	if p.ProjectID != projectID {
		c.JSON(http.StatusNotFound, gin.H{"error": "preset not found"})
		return
	}
	c.JSON(http.StatusOK, toEnrollmentPresetItem(p))
}

// Update handles PUT /projects/:pid/enrollment-presets/:rid. Full-PUT
// semantics: every mutable field on the body replaces what's stored.
func (h *EnrollmentPresetsHandler) Update(c *gin.Context) {
	projectID := c.Param("pid")
	presetID := c.Param("rid")

	existing, err := h.db.EnrollmentPresets().Get(c.Request.Context(), presetID)
	if errors.Is(err, storage.ErrNotFound) || (err == nil && existing.ProjectID != projectID) {
		c.JSON(http.StatusNotFound, gin.H{"error": "preset not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "get preset"})
		return
	}

	var req upsertEnrollmentPresetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad request: " + err.Error()})
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}

	existing.Name = strings.TrimSpace(req.Name)
	existing.Description = req.Description
	existing.ServerEndpoint = strings.TrimSpace(req.ServerEndpoint)
	existing.TargetOS = req.TargetOS
	existing.TargetArch = req.TargetArch
	existing.TTLSeconds = req.TTLSeconds
	existing.PATMaxUses = req.PATMaxUses
	existing.AutoApprove = req.AutoApprove
	existing.SkipTLSVerification = req.SkipTLSVerification
	existing.BaselinePluginIDs = req.BaselinePluginIDs
	existing.PATDescription = req.PATDescription
	existing.UpdatedAt = time.Now().UTC()

	if err := h.db.EnrollmentPresets().Update(c.Request.Context(), existing); err != nil {
		if strings.Contains(err.Error(), "idx_enrollment_presets_name") || strings.Contains(err.Error(), "UNIQUE") {
			h.audit(c, "enrollment_preset.update", presetID, projectID, req, "denied", "duplicate name")
			c.JSON(http.StatusConflict, gin.H{"error": "a preset with that name already exists in this project"})
			return
		}
		if errors.Is(err, storage.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "preset not found"})
			return
		}
		h.audit(c, "enrollment_preset.update", presetID, projectID, req, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "update preset"})
		return
	}
	h.audit(c, "enrollment_preset.update", presetID, projectID, req, "success", "")
	c.JSON(http.StatusOK, toEnrollmentPresetItem(existing))
}

// Delete handles DELETE /projects/:pid/enrollment-presets/:rid.
func (h *EnrollmentPresetsHandler) Delete(c *gin.Context) {
	projectID := c.Param("pid")
	presetID := c.Param("rid")

	existing, err := h.db.EnrollmentPresets().Get(c.Request.Context(), presetID)
	if errors.Is(err, storage.ErrNotFound) || (err == nil && existing.ProjectID != projectID) {
		h.audit(c, "enrollment_preset.delete", presetID, projectID, nil, "denied", "not found")
		c.Status(http.StatusNotFound)
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "get preset"})
		return
	}
	if err := h.db.EnrollmentPresets().Delete(c.Request.Context(), presetID); err != nil {
		h.audit(c, "enrollment_preset.delete", presetID, projectID, nil, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "delete preset"})
		return
	}
	h.audit(c, "enrollment_preset.delete", presetID, projectID, nil, "success", "")
	c.Status(http.StatusNoContent)
}

// Seed handles POST /projects/:pid/enrollment-presets/seed. Idempotent:
// inserts the system-default presets that don't already exist (filtered
// against the live install manifest) and returns the resulting list.
// Safe to call from the FE on every fresh wizard open with an empty
// list — INSERT OR IGNORE in storage absorbs the no-op case.
func (h *EnrollmentPresetsHandler) Seed(c *gin.Context) {
	projectID := c.Param("pid")
	claims, _ := ClaimsFromContext(c)

	supported := h.livePlatforms(c.Request.Context())
	now := time.Now().UTC()
	rows, err := h.db.EnrollmentPresets().SeedSystemPresets(
		c.Request.Context(), projectID, supported, now, claims.UserID,
	)
	if err != nil {
		h.audit(c, "enrollment_preset.seed", "", projectID, nil, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "seed presets"})
		return
	}
	h.audit(c, "enrollment_preset.seed", "", projectID, nil, "success", "")
	out := make([]enrollmentPresetItem, 0, len(rows))
	for _, p := range rows {
		out = append(out, toEnrollmentPresetItem(p))
	}
	c.JSON(http.StatusOK, gin.H{"presets": out})
}

// livePlatforms reads the active install manifest. Mirrors the
// install-tokens handler's Platforms() — same five-second timeout, same
// "no distributor configured == empty list" fallback. Lifting the exact
// shape here avoids a cross-handler dependency for this one call.
func (h *EnrollmentPresetsHandler) livePlatforms(ctx context.Context) []storage.SeedPlatform {
	d, ok := core.Ctx.Distributor.(*core.Distributor)
	if !ok || d == nil {
		return nil
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, _, artifacts := d.LivePlatforms(timeoutCtx)
	out := make([]storage.SeedPlatform, 0, len(artifacts))
	for _, a := range artifacts {
		out = append(out, storage.SeedPlatform{OS: a.OS, Arch: a.Arch})
	}
	return out
}

func (h *EnrollmentPresetsHandler) audit(c *gin.Context, action, presetID, projectID string, details interface{}, outcome, errText string) {
	pid := projectID
	RecordActivity(c, ActivityInput{
		ProjectID:   &pid,
		Category:    storage.CategoryAdmin,
		Action:      action,
		TargetType:  "enrollment_preset",
		TargetID:    presetID,
		TargetLabel: presetID,
		Outcome:     outcome,
		Error:       errText,
		Meta:        details,
	})
}

// RegisterV1EnrollmentPresetRoutes mounts the preset CRUD surface at
// the same admin gate the rest of the enrollment-related routes use.
// Project admins (or global admins) can create / edit / delete; viewers
// cannot. The FE seed call piggybacks on the same gate — only admins
// land on the wizard's pick-preset screen anyway.
func RegisterV1EnrollmentPresetRoutes(engine *gin.Engine, h *EnrollmentPresetsHandler, rbac *RBAC) {
	grp := engine.Group("/api/v1/projects/:pid/enrollment-presets")
	grp.Use(rbac.RequireAuth(), rbac.RequireProjectRole("pid", user.RoleAdmin))
	{
		grp.POST("", h.Create)
		grp.GET("", h.List)
		// /seed is a sub-path rather than a colon-segment so it doesn't
		// collide with /:rid for the GET shape — Gin disallows that
		// kind of overlap inside the same group.
		grp.POST("/seed", h.Seed)
		grp.GET("/:rid", h.Get)
		grp.PUT("/:rid", h.Update)
		grp.DELETE("/:rid", h.Delete)
	}
}
