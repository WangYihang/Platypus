package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/enrollment"
	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

// EnrollmentTokensHandler owns the /projects/:pid/pat-tokens admin
// surface. The on-the-wire URL still says "pat-tokens" so already
// shipped operator scripts and the agent install bootstrap keep
// working; the Go-side names reflect what the rows really are: one-shot
// agent-enrollment credentials. Listing and revocation hit storage
// directly; issuance delegates to the enrollment service so the
// (id, secret) pair stays in one place.
type EnrollmentTokensHandler struct {
	db     *storage.DB
	enroll *enrollment.Service
}

func NewEnrollmentTokensHandler(db *storage.DB, enroll *enrollment.Service) *EnrollmentTokensHandler {
	return &EnrollmentTokensHandler{db: db, enroll: enroll}
}

// --- Request / Response shapes ---------------------------------------------

type issueEnrollmentTokenRequest struct {
	Description      string `json:"description"`
	TTLSeconds       int    `json:"ttl_seconds"`
	MaxUses          int    `json:"max_uses"`
	BindingMachineID string `json:"binding_machine_id"`
	BindingHostAlias string `json:"binding_host_alias"`
}

// issueEnrollmentTokenResponse is the ONLY response that carries the
// plaintext token. Clients must persist it immediately — no other
// endpoint will return it.
type issueEnrollmentTokenResponse struct {
	TokenID     string    `json:"token_id"`
	Token       string    `json:"token"` // plt_<id>.<secret>
	ExpiresAt   time.Time `json:"expires_at"`
	IssuedAt    time.Time `json:"issued_at"`
	MaxUses     int       `json:"max_uses"`
	Description string    `json:"description,omitempty"`
}

// enrollmentTokenListItem is the redacted view surfaced to anyone
// listing tokens. Never contains the secret or its hash.
type enrollmentTokenListItem struct {
	TokenID          string     `json:"token_id"`
	Description      string     `json:"description,omitempty"`
	IssuedByUser     string     `json:"issued_by_user"`
	IssuedAt         time.Time  `json:"issued_at"`
	ExpiresAt        time.Time  `json:"expires_at"`
	MaxUses          int        `json:"max_uses"`
	Uses             int        `json:"uses"`
	BindingMachineID string     `json:"binding_machine_id,omitempty"`
	BindingHostAlias string     `json:"binding_host_alias,omitempty"`
	Revoked          bool       `json:"revoked"`
	RevokedAt        *time.Time `json:"revoked_at,omitempty"`
	RevokedReason    string     `json:"revoked_reason,omitempty"`
	Status           string     `json:"status"` // derived
}

func toEnrollmentListItem(p *storage.EnrollmentToken, now time.Time) enrollmentTokenListItem {
	return enrollmentTokenListItem{
		TokenID:          p.TokenID,
		Description:      p.Description,
		IssuedByUser:     p.IssuedByUser,
		IssuedAt:         p.IssuedAt,
		ExpiresAt:        p.ExpiresAt,
		MaxUses:          p.MaxUses,
		Uses:             p.Uses,
		BindingMachineID: p.BindingMachineID,
		BindingHostAlias: p.BindingHostAlias,
		Revoked:          p.Revoked,
		RevokedAt:        p.RevokedAt,
		RevokedReason:    p.RevokedReason,
		Status:           string(p.Status(now)),
	}
}

// --- Handlers --------------------------------------------------------------

// Issue handles POST /projects/:pid/pat-tokens. Returns the plaintext
// enrollment token exactly once. Operators are expected to copy it into
// whatever bootstrap channel they're using.
func (h *EnrollmentTokensHandler) Issue(c *gin.Context) {
	projectID := c.Param("pid")
	claims, _ := ClaimsFromContext(c)

	var req issueEnrollmentTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// Body is optional — tolerate bad JSON by falling back to defaults.
		req = issueEnrollmentTokenRequest{}
	}

	ttl := time.Duration(req.TTLSeconds) * time.Second
	res, err := h.enroll.MintEnrollmentToken(c.Request.Context(), enrollment.MintEnrollmentTokenInput{
		ProjectID:        projectID,
		IssuedByUser:     claims.UserID,
		Description:      req.Description,
		TTL:              ttl,
		MaxUses:          req.MaxUses,
		BindingMachineID: req.BindingMachineID,
		BindingHostAlias: req.BindingHostAlias,
	})
	if err != nil {
		h.audit(c, "pat.issue", "pat_token", "", projectID, req, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "issue pat"})
		return
	}
	h.audit(c, "pat.issue", "pat_token", res.TokenID, projectID, req, "success", "")

	c.JSON(http.StatusCreated, issueEnrollmentTokenResponse{
		TokenID:     res.TokenID,
		Token:       res.PlaintextToken,
		ExpiresAt:   res.ExpiresAt,
		IssuedAt:    res.Token.IssuedAt,
		MaxUses:     res.Token.MaxUses,
		Description: res.Token.Description,
	})
}

// List handles GET /projects/:pid/pat-tokens.
// Query: ?include_inactive=true to include revoked / consumed / expired.
func (h *EnrollmentTokensHandler) List(c *gin.Context) {
	projectID := c.Param("pid")
	includeInactive := c.Query("include_inactive") == "true"

	toks, err := h.db.EnrollmentTokens().ListByProject(c.Request.Context(), projectID, includeInactive)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list pat tokens"})
		return
	}
	now := time.Now().UTC()
	out := make([]enrollmentTokenListItem, 0, len(toks))
	for _, p := range toks {
		out = append(out, toEnrollmentListItem(p, now))
	}
	c.JSON(http.StatusOK, gin.H{"tokens": out})
}

// Get handles GET /projects/:pid/pat-tokens/:tid. Includes the full
// redemption event history so auditors can trace activity.
func (h *EnrollmentTokensHandler) Get(c *gin.Context) {
	tokenID := c.Param("tid")
	ctx := c.Request.Context()
	tok, err := h.db.EnrollmentTokens().Get(ctx, tokenID)
	if errors.Is(err, storage.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "pat not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "get pat"})
		return
	}
	// Redemption events now live in the unified activities log. Filter
	// to this specific token id and the auth category so we surface both
	// success and failure attempts alongside each other. Action / target
	// strings are kept on the historical "pat.*" / "pat_token" names so
	// pre-rename audit rows stay queryable.
	events, _, err := h.db.Activities().List(ctx, storage.ActivityFilter{
		Actions:    []string{"pat.redeem", "pat.redeem_failed"},
		TargetType: "pat_token",
		TargetID:   tokenID,
		Limit:      200,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list redemption events"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"token":  toEnrollmentListItem(tok, time.Now().UTC()),
		"events": events,
	})
}

// Revoke handles DELETE /projects/:pid/pat-tokens/:tid. Idempotent for
// convenience — revoking an already-revoked token is a 204.
func (h *EnrollmentTokensHandler) Revoke(c *gin.Context) {
	projectID := c.Param("pid")
	tokenID := c.Param("tid")
	claims, _ := ClaimsFromContext(c)

	reason := c.Query("reason")
	err := h.enroll.RevokeEnrollmentToken(c.Request.Context(), tokenID, claims.UserID, reason)
	if errors.Is(err, storage.ErrNotFound) {
		h.audit(c, "pat.revoke", "pat_token", tokenID, projectID, map[string]string{"reason": reason}, "denied", "not found")
		c.Status(http.StatusNotFound)
		return
	}
	if err != nil {
		h.audit(c, "pat.revoke", "pat_token", tokenID, projectID, map[string]string{"reason": reason}, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "revoke pat"})
		return
	}
	h.audit(c, "pat.revoke", "pat_token", tokenID, projectID, map[string]string{"reason": reason}, "success", "")
	c.Status(http.StatusNoContent)
}

// audit writes one row into the unified activities log. Errors during
// audit do NOT fail the main flow; the recorder logs them itself.
func (h *EnrollmentTokensHandler) audit(c *gin.Context, action, targetType, targetID, projectID string, details interface{}, outcome, errText string) {
	pid := projectID
	RecordActivity(c, ActivityInput{
		ProjectID:   &pid,
		Category:    storage.CategoryAdmin,
		Action:      action,
		TargetType:  targetType,
		TargetID:    targetID,
		TargetLabel: targetID,
		Outcome:     outcome,
		Error:       errText,
		Meta:        details,
	})
}

// RegisterV1EnrollmentTokenRoutes mounts the enrollment-token admin
// surface. Wire URL keeps the historical "pat-tokens" segment so
// existing operator scripts and CI flows don't break. RequireProjectRole
// (admin) — project admins (or global admins) can issue tokens; viewers
// cannot.
func RegisterV1EnrollmentTokenRoutes(engine *gin.Engine, h *EnrollmentTokensHandler, rbac *RBAC) {
	grp := engine.Group("/api/v1/projects/:pid/pat-tokens")
	grp.Use(rbac.RequireAuth(), rbac.RequireProjectRole("pid", user.RoleAdmin))
	{
		grp.POST("", h.Issue)
		grp.GET("", h.List)
		grp.GET("/:tid", h.Get)
		grp.DELETE("/:tid", h.Revoke)
	}
}
