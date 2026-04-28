package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/optoken"
	"github.com/WangYihang/Platypus/internal/storage"
)

// AccountPATHandler exposes the user-self personal-access-token surface
// at /api/v1/account/pat. Tokens are issued by the holder (no admin
// gating), bind to the user's account (project access is re-evaluated
// per request against project_members), and inherit the user's live
// role at verify time so a demoted user immediately loses any
// write-scoped capability that was granted at issue time.
//
// The plaintext `pat_<id>.<secret>` only ever leaves the server in the
// POST response. List / Get strip it.
type AccountPATHandler struct {
	db       *storage.DB
	verifier *TokenVerifier
}

func NewAccountPATHandler(db *storage.DB, verifier *TokenVerifier) *AccountPATHandler {
	return &AccountPATHandler{db: db, verifier: verifier}
}

// PAT lifetime bounds. TTLs above MaxPATTTL clamp; below MinPATTTL
// reject so a misconfigured client can't mint near-expired tokens.
const (
	DefaultPATTTL = 90 * 24 * time.Hour
	MinPATTTL     = 1 * time.Minute
	MaxPATTTL     = 365 * 24 * time.Hour
)

// --- Wire shapes ----------------------------------------------------------

type issueAccountPATRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Scopes      []string `json:"scopes"`
	TTLSeconds  int      `json:"ttl_seconds"`
}

// issueAccountPATResponse is the only place the PAT plaintext leaves
// the server. Returned ONCE on issue; clients must persist it
// immediately (mirrors the `git remote add ... https://ghp_...`
// experience).
type issueAccountPATResponse struct {
	TokenID   string    `json:"token_id"`
	Token     string    `json:"token"` // pat_<id>.<secret>
	Name      string    `json:"name"`
	Scopes    []string  `json:"scopes"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// accountPATListItem is the redacted shape returned by List / Get.
// SecretHash never leaves the storage layer; this struct couldn't
// surface it even by accident.
type accountPATListItem struct {
	TokenID     string     `json:"token_id"`
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	Scopes      []string   `json:"scopes"`
	CreatedAt   time.Time  `json:"created_at"`
	ExpiresAt   time.Time  `json:"expires_at"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
	LastUsedIP  string     `json:"last_used_ip,omitempty"`
	Revoked     bool       `json:"revoked"`
	RevokedAt   *time.Time `json:"revoked_at,omitempty"`
}

func toAccountPATListItem(p *storage.PAT) accountPATListItem {
	return accountPATListItem{
		TokenID:     p.TokenID,
		Name:        p.Name,
		Description: p.Description,
		Scopes:      p.Scopes,
		CreatedAt:   p.CreatedAt,
		ExpiresAt:   p.ExpiresAt,
		LastUsedAt:  p.LastUsedAt,
		LastUsedIP:  p.LastUsedIP,
		Revoked:     p.Revoked,
		RevokedAt:   p.RevokedAt,
	}
}

// --- Handlers --------------------------------------------------------------

// Issue handles POST /api/v1/account/pat. The caller must be a human
// (session-authenticated) — PATs cannot mint other PATs, so any future
// AI / scripted automation cannot bootstrap further credentials by
// presenting the one it was handed.
func (h *AccountPATHandler) Issue(c *gin.Context) {
	p, ok := PrincipalFromContext(c)
	if !ok || p.Kind != PrincipalUser || p.UserID == "" {
		c.AbortWithStatusJSON(http.StatusForbidden,
			gin.H{"error": "PAT issuance requires a human session"})
		return
	}

	var req issueAccountPATRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name required"})
		return
	}

	// Scope check: requested scopes must be a subset of what the
	// caller's current role grants. Empty scopes default to that full
	// ceiling (the UI also defaults that way). The ceiling comes from
	// the LIVE roles table — if an admin has shrunk the caller's role
	// since their session started, they can only mint a token with
	// the new (smaller) permission set.
	role, err := h.db.Roles().Get(c.Request.Context(), string(p.Role))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "lookup role"})
		return
	}
	roleCeiling := role.Permissions
	scopes := req.Scopes
	if len(scopes) == 0 {
		scopes = roleCeiling
	} else {
		for _, s := range scopes {
			if !optoken.HasScope(roleCeiling, s) {
				c.JSON(http.StatusForbidden,
					gin.H{"error": "scope not held by caller: " + s})
				return
			}
		}
	}
	if len(scopes) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no scopes available for caller"})
		return
	}

	ttl := time.Duration(req.TTLSeconds) * time.Second
	switch {
	case req.TTLSeconds == 0:
		ttl = DefaultPATTTL
	case ttl < MinPATTTL:
		c.JSON(http.StatusBadRequest, gin.H{"error": "ttl below minimum"})
		return
	case ttl > MaxPATTTL:
		ttl = MaxPATTTL
	}

	id, _, hash, plaintext, err := optoken.Generate(optoken.PATPrefix)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "generate pat"})
		return
	}
	now := time.Now().UTC()
	pat := &storage.PAT{
		TokenID:     id,
		SecretHash:  hash,
		UserID:      p.UserID,
		Name:        req.Name,
		Description: req.Description,
		Scopes:      scopes,
		CreatedAt:   now,
		ExpiresAt:   now.Add(ttl),
	}
	if err := h.db.AuthTokens().CreatePAT(c.Request.Context(), pat); err != nil {
		h.audit(c, "pat.issue", id, req, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "store pat"})
		return
	}
	h.audit(c, "pat.issue", id, gin.H{"name": req.Name, "scopes": scopes}, "success", "")

	c.JSON(http.StatusCreated, issueAccountPATResponse{
		TokenID:   id,
		Token:     plaintext,
		Name:      req.Name,
		Scopes:    scopes,
		CreatedAt: now,
		ExpiresAt: pat.ExpiresAt,
	})
}

// List handles GET /api/v1/account/pat?include_revoked=bool. Returns
// only the caller's own tokens — there's no admin-wide "list all PATs"
// surface today (audit needs go through the activities log).
func (h *AccountPATHandler) List(c *gin.Context) {
	p, ok := PrincipalFromContext(c)
	if !ok || p.UserID == "" {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "no principal"})
		return
	}
	includeRevoked := c.Query("include_revoked") == "true"
	pats, err := h.db.AuthTokens().ListPATsForUser(c.Request.Context(), p.UserID, includeRevoked)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list pats"})
		return
	}
	out := make([]accountPATListItem, 0, len(pats))
	for _, pt := range pats {
		out = append(out, toAccountPATListItem(pt))
	}
	c.JSON(http.StatusOK, gin.H{"tokens": out})
}

// Get handles GET /api/v1/account/pat/:tid. Owner-or-deny — PATs are
// per-user and there is no admin override path here (admins listing
// foreign PATs would amount to a parallel admin surface that we
// deliberately don't ship).
func (h *AccountPATHandler) Get(c *gin.Context) {
	p, ok := PrincipalFromContext(c)
	if !ok || p.UserID == "" {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "no principal"})
		return
	}
	tid := c.Param("tid")
	pat, err := h.db.AuthTokens().GetPAT(c.Request.Context(), tid)
	if errors.Is(err, storage.ErrNotFound) || (pat != nil && pat.UserID != p.UserID) {
		c.JSON(http.StatusNotFound, gin.H{"error": "pat not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "get pat"})
		return
	}
	c.JSON(http.StatusOK, toAccountPATListItem(pat))
}

// ListMyPermissions handles GET /api/v1/account/permissions. Returns
// the calling user's effective permission set — the live permissions
// of their global role. The PAT issue dialog reads this so its scope
// checkboxes match exactly what the server will accept; viewers can
// see the (smaller) set their role offers without trying to mint a
// scope they don't hold and getting a 403.
func (h *AccountPATHandler) ListMyPermissions(c *gin.Context) {
	p, ok := PrincipalFromContext(c)
	if !ok || p.UserID == "" {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "no principal"})
		return
	}
	role, err := h.db.Roles().Get(c.Request.Context(), string(p.Role))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "lookup role"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"permissions": role.Permissions})
}

// Revoke handles DELETE /api/v1/account/pat/:tid. Idempotent: revoking
// an already-revoked or non-existent owned token is still 204 to avoid
// leaking row existence to the caller.
func (h *AccountPATHandler) Revoke(c *gin.Context) {
	p, ok := PrincipalFromContext(c)
	if !ok || p.UserID == "" {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "no principal"})
		return
	}
	tid := c.Param("tid")

	// Check ownership first — a user must not be able to revoke a
	// PAT they don't own. ErrNotFound from GetPAT also returns 204
	// so we don't leak existence ("the id you tried doesn't belong
	// to you OR doesn't exist" looks identical).
	pat, err := h.db.AuthTokens().GetPAT(c.Request.Context(), tid)
	if errors.Is(err, storage.ErrNotFound) || (pat != nil && pat.UserID != p.UserID) {
		c.Status(http.StatusNoContent)
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "lookup pat"})
		return
	}

	if err := h.db.AuthTokens().Revoke(c.Request.Context(), tid, p.UserID, "user-revoked", time.Now().UTC()); err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			h.audit(c, "pat.revoke", tid, nil, "error", err.Error())
			c.JSON(http.StatusInternalServerError, gin.H{"error": "revoke pat"})
			return
		}
	}
	if h.verifier != nil {
		h.verifier.Invalidate(tid)
	}
	h.audit(c, "pat.revoke", tid, nil, "success", "")
	c.Status(http.StatusNoContent)
}

// audit writes one row into the unified activities log. Distinct
// target_type ("user_pat") so historical "pat_token" enrollment
// activity stays cleanly partitioned from user-PAT lifecycle events.
func (h *AccountPATHandler) audit(c *gin.Context, action, tid string, details interface{}, outcome, errText string) {
	RecordActivity(c, ActivityInput{
		Category:    storage.CategoryAdmin,
		Action:      action,
		TargetType:  "user_pat",
		TargetID:    tid,
		TargetLabel: tid,
		Outcome:     outcome,
		Error:       errText,
		Meta:        details,
	})
}

// RegisterV1AccountPATRoutes mounts the user-self PAT routes and the
// /api/v1/account/permissions reflector behind RequireAuth — anyone
// with a valid bearer can manage their own PATs and see their own
// effective permissions. The handler itself enforces "human session
// only" on POST and per-row owner gating on GET / DELETE.
func RegisterV1AccountPATRoutes(engine *gin.Engine, h *AccountPATHandler, rbac *RBAC) {
	auth := engine.Group("/api/v1/account").Use(rbac.RequireAuth())
	auth.GET("/permissions", h.ListMyPermissions)
	g := engine.Group("/api/v1/account/pat")
	g.Use(rbac.RequireAuth())
	{
		g.POST("", h.Issue)
		g.GET("", h.List)
		g.GET("/:tid", h.Get)
		g.DELETE("/:tid", h.Revoke)
	}
}
