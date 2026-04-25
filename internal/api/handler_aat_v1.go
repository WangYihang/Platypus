package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/optoken"
	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

// AATHandler owns the /api/v1/aat surface — minting, listing, getting,
// and revoking AI-agent tokens. It hashes the secret via optoken and
// asks storage to persist the row; the plaintext leaves the process
// only on the issue / rotate response and never appears in any other
// endpoint or audit field.
type AATHandler struct {
	db       *storage.DB
	verifier *TokenVerifier
}

func NewAATHandler(db *storage.DB, verifier *TokenVerifier) *AATHandler {
	return &AATHandler{db: db, verifier: verifier}
}

// AAT lifetime bounds. TTL beyond MaxAATTTL is clamped at issue; TTL
// below MinAATTTL is rejected so a misconfigured client can't mint
// near-expired tokens.
const (
	DefaultAATTTL = 30 * 24 * time.Hour // 30 days
	MinAATTTL     = 1 * time.Minute
	MaxAATTTL     = 365 * 24 * time.Hour // 1 year
)

// --- Wire shapes ----------------------------------------------------------

type issueAATRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Role        string   `json:"role"`
	Scopes      []string `json:"scopes"`
	TTLSeconds  int      `json:"ttl_seconds"`
}

// issueAATResponse is the only place plaintext leaves the server. It
// is returned ONCE on issue / rotate; clients must persist it.
type issueAATResponse struct {
	TokenID     string    `json:"token_id"`
	Token       string    `json:"token"` // aat_<id>.<secret>
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Role        string    `json:"role"`
	Scopes      []string  `json:"scopes"`
	ProjectID   string    `json:"project_id,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// aatListItem is the redacted view safe for any listing endpoint.
// Never carries SecretHash or any plaintext.
type aatListItem struct {
	TokenID     string     `json:"token_id"`
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	UserID      string     `json:"user_id"`
	ProjectID   string     `json:"project_id,omitempty"`
	Role        string     `json:"role"`
	Scopes      []string   `json:"scopes"`
	CreatedAt   time.Time  `json:"created_at"`
	ExpiresAt   time.Time  `json:"expires_at"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
	LastUsedIP  string     `json:"last_used_ip,omitempty"`
	Revoked     bool       `json:"revoked"`
	RevokedAt   *time.Time `json:"revoked_at,omitempty"`
}

func toAATListItem(a *storage.AAT) aatListItem {
	return aatListItem{
		TokenID:     a.TokenID,
		Name:        a.Name,
		Description: a.Description,
		UserID:      a.UserID,
		ProjectID:   a.ProjectID,
		Role:        string(a.Role),
		Scopes:      a.Scopes,
		CreatedAt:   a.CreatedAt,
		ExpiresAt:   a.ExpiresAt,
		LastUsedAt:  a.LastUsedAt,
		LastUsedIP:  a.LastUsedIP,
		Revoked:     a.Revoked,
		RevokedAt:   a.RevokedAt,
	}
}

// --- Authorization helpers ------------------------------------------------

// humanCallerOrAbort returns the *Principal if it's a user-kind, else
// aborts 403. AAT management is always human-only — nothing in the
// request flow needs an AAT to mint another AAT, and allowing it would
// blur the audit chain.
func humanCallerOrAbort(c *gin.Context) (*Principal, bool) {
	p, ok := PrincipalFromContext(c)
	if !ok {
		abortUnauthorized(c, "no principal")
		return nil, false
	}
	if p.Kind != PrincipalUser {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "AAT management is restricted to human users"})
		return nil, false
	}
	return p, true
}

// validateIssue applies the issuance-limit rules: caller's role must
// dominate the requested role, and the requested scopes must be a
// subset of the caller's effective scopes. Returns the parsed Role
// and clamped TTL, or a string error message suitable for a 400.
func validateIssue(p *Principal, in issueAATRequest) (user.Role, []string, time.Duration, string) {
	if in.Name == "" {
		return "", nil, 0, "name is required"
	}
	role, err := user.ParseRole(in.Role)
	if err != nil {
		return "", nil, 0, "invalid role"
	}
	if !roleAtLeast(p.Role, role) {
		return "", nil, 0, "cannot issue AAT with role above your own"
	}
	// Granted scopes must be a subset of the caller's effective scopes.
	callerScopes := optoken.ScopesFromRole(p.Role)
	for _, s := range in.Scopes {
		if !optoken.HasScope(callerScopes, s) {
			return "", nil, 0, "cannot grant scope you do not hold: " + s
		}
	}
	ttl := time.Duration(in.TTLSeconds) * time.Second
	if ttl <= 0 {
		ttl = DefaultAATTTL
	}
	if ttl < MinAATTTL {
		return "", nil, 0, "ttl_seconds too short"
	}
	if ttl > MaxAATTTL {
		ttl = MaxAATTTL
	}
	return role, in.Scopes, ttl, ""
}

// --- Handlers -------------------------------------------------------------

// IssueGlobal handles POST /api/v1/aat — global AAT, no project
// binding. Behind RequireGlobalRole(admin), so only platform-admins
// reach it. Plaintext returned exactly once.
func (h *AATHandler) IssueGlobal(c *gin.Context) {
	p, ok := humanCallerOrAbort(c)
	if !ok {
		return
	}
	h.issue(c, p, "")
}

// IssueProject handles POST /api/v1/projects/:pid/aat. The route is
// gated by RequireProjectRole(admin); inside, we bind the new AAT's
// ProjectID to :pid so the bearer can never reach another project.
func (h *AATHandler) IssueProject(c *gin.Context) {
	p, ok := humanCallerOrAbort(c)
	if !ok {
		return
	}
	pid := c.Param("pid")
	h.issue(c, p, pid)
}

func (h *AATHandler) issue(c *gin.Context, caller *Principal, projectID string) {
	var req issueAATRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	role, scopes, ttl, vErr := validateIssue(caller, req)
	if vErr != "" {
		// 403 for permission errors (role/scope escalation), 400 for
		// shape errors. We can tell them apart by the message family.
		status := http.StatusBadRequest
		switch vErr {
		case "cannot issue AAT with role above your own":
			status = http.StatusForbidden
		}
		if len(vErr) > 25 && vErr[:25] == "cannot grant scope you do" {
			status = http.StatusForbidden
		}
		c.JSON(status, gin.H{"error": vErr})
		return
	}

	id, _, hash, plaintext, err := optoken.Generate(optoken.AATPrefix)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "generate token"})
		return
	}
	now := time.Now().UTC()
	a := &storage.AAT{
		TokenID:     id,
		SecretHash:  hash,
		UserID:      caller.UserID,
		Name:        req.Name,
		Description: req.Description,
		ProjectID:   projectID,
		Role:        role,
		Scopes:      scopes,
		CreatedAt:   now,
		ExpiresAt:   now.Add(ttl),
	}
	if err := h.db.AuthTokens().CreateAAT(c.Request.Context(), a); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "create AAT: " + err.Error()})
		return
	}
	auditAATIssue(c, a)

	c.JSON(http.StatusCreated, issueAATResponse{
		TokenID:     a.TokenID,
		Token:       plaintext,
		Name:        a.Name,
		Description: a.Description,
		Role:        string(a.Role),
		Scopes:      a.Scopes,
		ProjectID:   a.ProjectID,
		CreatedAt:   a.CreatedAt,
		ExpiresAt:   a.ExpiresAt,
	})
}

// Get handles GET /api/v1/aat/:tid. Creator and global admins can
// read; everyone else gets 404 (not 403) so the route doesn't leak
// AAT id existence to outsiders.
func (h *AATHandler) Get(c *gin.Context) {
	p, ok := humanCallerOrAbort(c)
	if !ok {
		return
	}
	tokenID := c.Param("tid")
	a, err := h.db.AuthTokens().GetAAT(c.Request.Context(), tokenID)
	if errors.Is(err, storage.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "get AAT"})
		return
	}
	if !canManageAAT(p, a) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"token": toAATListItem(a)})
}

// canManageAAT decides whether the principal may read / mutate the
// AAT row. Creator always wins; non-creator humans need global admin
// (project admins do NOT — they should mint per-project AATs and
// manage those; cross-creator visibility is a global concern).
func canManageAAT(p *Principal, a *storage.AAT) bool {
	if p.UserID == a.UserID {
		return true
	}
	return p.IsGlobalAdmin()
}

// List handles GET /api/v1/aat — caller's own AATs by default, all
// AATs when ?all=true (admin-only). Project-scoped listing has its
// own endpoint (ListByProject) so the URL communicates the binding.
func (h *AATHandler) List(c *gin.Context) {
	p, ok := humanCallerOrAbort(c)
	if !ok {
		return
	}
	includeRevoked := c.Query("include_revoked") == "true"

	if c.Query("all") == "true" {
		if !p.IsGlobalAdmin() {
			c.JSON(http.StatusForbidden, gin.H{"error": "?all=true requires global admin"})
			return
		}
		// Admin-all view: by-creator with no filter = caller's own.
		// We expose admin-all via project listing + per-creator
		// queries; a single ?all isn't worth its own SELECT *.
		c.JSON(http.StatusBadRequest, gin.H{"error": "list-all not implemented; query by creator or project"})
		return
	}

	tokens, err := h.db.AuthTokens().ListAATsByCreator(c.Request.Context(), p.UserID, includeRevoked)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list AATs"})
		return
	}
	out := make([]aatListItem, 0, len(tokens))
	for _, a := range tokens {
		out = append(out, toAATListItem(a))
	}
	c.JSON(http.StatusOK, gin.H{"tokens": out})
}

// ListByProject handles GET /api/v1/projects/:pid/aat. Behind
// RequireProjectRole(admin); returns AATs scoped to this project.
func (h *AATHandler) ListByProject(c *gin.Context) {
	if _, ok := humanCallerOrAbort(c); !ok {
		return
	}
	pid := c.Param("pid")
	includeRevoked := c.Query("include_revoked") == "true"
	tokens, err := h.db.AuthTokens().ListAATsByProject(c.Request.Context(), pid, includeRevoked)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list project AATs"})
		return
	}
	out := make([]aatListItem, 0, len(tokens))
	for _, a := range tokens {
		out = append(out, toAATListItem(a))
	}
	c.JSON(http.StatusOK, gin.H{"tokens": out})
}

// Revoke handles DELETE /api/v1/aat/:tid. Idempotent. After the DB
// commit succeeds, the verifier cache is invalidated synchronously so
// the next request observes the change without waiting for the TTL.
func (h *AATHandler) Revoke(c *gin.Context) {
	p, ok := humanCallerOrAbort(c)
	if !ok {
		return
	}
	tokenID := c.Param("tid")

	a, err := h.db.AuthTokens().GetAAT(c.Request.Context(), tokenID)
	if errors.Is(err, storage.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "get AAT"})
		return
	}
	if !canManageAAT(p, a) {
		// Same 404 as Get for unauthorised callers — don't leak id
		// existence.
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	reason := c.Query("reason")
	if err := h.db.AuthTokens().Revoke(c.Request.Context(), tokenID, p.UserID, reason, time.Now().UTC()); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "revoke"})
		return
	}
	if h.verifier != nil {
		h.verifier.Invalidate(tokenID)
	}
	auditAATRevoke(c, a, reason)
	c.Status(http.StatusNoContent)
}

// auditAATIssue / auditAATRevoke write to the activities log. Field
// names mirror what handler_pat_tokens does so dashboards can union
// across credential kinds.
func auditAATIssue(c *gin.Context, a *storage.AAT) {
	pid := a.ProjectID
	var pidPtr *string
	if pid != "" {
		pidPtr = &pid
	}
	RecordActivity(c, ActivityInput{
		ProjectID:   pidPtr,
		Category:    storage.CategoryAdmin,
		Action:      "aat.issue",
		TargetType:  "aat",
		TargetID:    a.TokenID,
		TargetLabel: a.Name,
		Outcome:     "success",
		Meta: map[string]any{
			"role":       string(a.Role),
			"scopes":     a.Scopes,
			"expires_at": a.ExpiresAt,
		},
	})
}

func auditAATRevoke(c *gin.Context, a *storage.AAT, reason string) {
	pid := a.ProjectID
	var pidPtr *string
	if pid != "" {
		pidPtr = &pid
	}
	RecordActivity(c, ActivityInput{
		ProjectID:   pidPtr,
		Category:    storage.CategoryAdmin,
		Action:      "aat.revoke",
		TargetType:  "aat",
		TargetID:    a.TokenID,
		TargetLabel: a.Name,
		Outcome:     "success",
		Meta:        map[string]any{"reason": reason},
	})
}

// --- Routing --------------------------------------------------------------

// RegisterV1AATRoutes wires every AAT endpoint. Global routes need a
// global admin; project routes go through RequireProjectRole(admin).
// Per-token Get/Revoke run only RequireAuth — the handler itself
// enforces creator-or-admin so the routes stay accessible to any
// caller for their own tokens.
func RegisterV1AATRoutes(engine *gin.Engine, h *AATHandler, rbac *RBAC) {
	g := engine.Group("/api/v1/aat")
	g.Use(rbac.RequireAuth())
	{
		g.POST("", rbac.RequireGlobalRole(user.RoleAdmin), h.IssueGlobal)
		g.GET("", h.List)
		g.GET("/:tid", h.Get)
		g.DELETE("/:tid", h.Revoke)
	}
	gp := engine.Group("/api/v1/projects/:pid/aat")
	gp.Use(rbac.RequireAuth(), rbac.RequireProjectRole("pid", user.RoleAdmin))
	{
		gp.POST("", h.IssueProject)
		gp.GET("", h.ListByProject)
	}
}
