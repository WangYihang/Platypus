package api

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

// AuthHandler wires the login / refresh / logout / bootstrap endpoints. It
// holds everything it needs as plain fields — no global state — so each test
// can instantiate an isolated handler against an in-memory DB.
type AuthHandler struct {
	db              *storage.DB
	tokens          *TokenIssuer
	bootstrapSecret string
}

func NewAuthHandler(db *storage.DB, tokens *TokenIssuer, bootstrapSecret string) *AuthHandler {
	return &AuthHandler{db: db, tokens: tokens, bootstrapSecret: bootstrapSecret}
}

type bootstrapRequest struct {
	Secret   string `json:"secret" binding:"required"`
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type loginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// publicInfoResponse is the shape of GET /api/v1/auth/info. Unlike
// /api/v1/info (which is auth-gated), this endpoint is reachable
// without a bearer token so the onboarding wizard can probe a URL
// the user just typed and tell them whether to log in or bootstrap.
// Keep the payload tiny — no live counters, no build sha that an
// attacker could correlate against CVEs; just "is this a Platypus
// server, and is it ready for first-time setup?".
type publicInfoResponse struct {
	Product           string `json:"product"`
	AdminBootstrapped bool   `json:"admin_bootstrapped"`
}

type userBody struct {
	ID       string    `json:"id"`
	Username string    `json:"username"`
	Role     user.Role `json:"role"`
}

type tokenPair struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	User         *userBody `json:"user,omitempty"`
}

// PublicInfo answers GET /api/v1/auth/info without requiring a bearer
// token. Returns just enough metadata for the desktop onboarding
// wizard to decide whether to show the Log-in or First-time-setup
// form. No build sha, no session counts, no endpoint enumeration —
// callers that are already authenticated can hit /api/v1/info for
// that.
func (h *AuthHandler) PublicInfo(c *gin.Context) {
	ctx := c.Request.Context()
	n, err := h.db.Users().Count(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "count users"})
		return
	}
	c.JSON(http.StatusOK, publicInfoResponse{
		Product:           "platypus",
		AdminBootstrapped: n > 0,
	})
}

// Bootstrap is the one-shot "set up the first admin" flow. It succeeds only
// when the users table is empty AND the caller presents the server's
// bootstrap secret. After the first admin exists this endpoint is dead
// weight; subsequent calls return 409.
func (h *AuthHandler) Bootstrap(c *gin.Context) {
	empty := ""
	var req bootstrapRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()
	n, err := h.db.Users().Count(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "count users"})
		return
	}
	if n > 0 {
		RecordActivity(c, ActivityInput{
			ProjectID: &empty,
			ActorType: storage.ActorTypeAnonymous,
			Category:  storage.CategoryAuth,
			Action:    "user.bootstrap",
			Outcome:   storage.OutcomeDenied,
			Error:     "already bootstrapped",
			Meta:      map[string]any{"username": req.Username},
		})
		c.JSON(http.StatusConflict, gin.H{"error": "already bootstrapped"})
		return
	}
	// subtle.ConstantTimeCompare keeps the bootstrap window slightly harder
	// to time-side-channel, though the single-shot nature already limits
	// exploitation.
	if subtle.ConstantTimeCompare([]byte(req.Secret), []byte(h.bootstrapSecret)) != 1 {
		RecordActivity(c, ActivityInput{
			ProjectID: &empty,
			ActorType: storage.ActorTypeAnonymous,
			Category:  storage.CategoryAuth,
			Action:    "user.bootstrap",
			Outcome:   storage.OutcomeDenied,
			Error:     "invalid secret",
			Meta:      map[string]any{"username": req.Username},
		})
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid secret"})
		return
	}

	hashed, err := user.HashPassword(req.Password)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	u := &user.User{
		ID:           uuid.NewString(),
		Username:     req.Username,
		PasswordHash: hashed,
		Role:         user.RoleAdmin,
		CreatedAt:    time.Now().UTC(),
	}
	if err := h.db.Users().Create(ctx, u); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "create user"})
		return
	}
	RecordActivity(c, ActivityInput{
		ProjectID:   &empty,
		ActorType:   storage.ActorTypeUser,
		ActorUser:   u.ID,
		Category:    storage.CategoryAuth,
		Action:      "user.bootstrap",
		TargetType:  "user",
		TargetID:    u.ID,
		TargetLabel: u.Username,
		Meta:        map[string]any{"username": u.Username, "role": string(u.Role)},
	})
	// Seed a "default" project so the legacy listener flows (which know
	// nothing about projects) still have somewhere to write. Only attempt
	// when no project exists yet — idempotent if someone renamed the seed.
	if _, err := h.db.Projects().GetBySlug(ctx, "default"); errors.Is(err, storage.ErrNotFound) {
		_ = h.db.Projects().Create(ctx, &storage.Project{
			ID:        uuid.NewString(),
			Name:      "Default",
			Slug:      "default",
			CreatedAt: time.Now().UTC(),
			CreatedBy: u.ID,
		})
	}
	h.issueTokensTo(c, u)
}

// Login authenticates by username + password and returns a fresh token pair.
// On invalid credentials we always return 401 with the same body so
// attackers can't distinguish "wrong username" from "wrong password".
func (h *AuthHandler) Login(c *gin.Context) {
	empty := ""
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()
	u, err := h.db.Users().GetByUsername(ctx, req.Username)
	if errors.Is(err, storage.ErrNotFound) {
		RecordActivity(c, ActivityInput{
			ProjectID: &empty,
			ActorType: storage.ActorTypeAnonymous,
			Category:  storage.CategoryAuth,
			Action:    "user.login_failed",
			Outcome:   storage.OutcomeDenied,
			Error:     "unknown user",
			Meta:      map[string]any{"username": req.Username, "reason": "unknown_user"},
		})
		unauthorized(c)
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "lookup user"})
		return
	}
	if !user.VerifyPassword(u.PasswordHash, req.Password) {
		RecordActivity(c, ActivityInput{
			ProjectID:   &empty,
			ActorType:   storage.ActorTypeAnonymous,
			Category:    storage.CategoryAuth,
			Action:      "user.login_failed",
			TargetType:  "user",
			TargetID:    u.ID,
			TargetLabel: u.Username,
			Outcome:     storage.OutcomeDenied,
			Error:       "invalid password",
			Meta:        map[string]any{"username": req.Username, "reason": "bad_password"},
		})
		unauthorized(c)
		return
	}
	if err := h.db.Users().TouchLastLogin(ctx, u.ID); err != nil {
		// Non-fatal — the login itself still succeeded.
		_ = err
	}
	RecordActivity(c, ActivityInput{
		ProjectID:   &empty,
		ActorType:   storage.ActorTypeUser,
		ActorUser:   u.ID,
		Category:    storage.CategoryAuth,
		Action:      "user.login",
		TargetType:  "user",
		TargetID:    u.ID,
		TargetLabel: u.Username,
		Meta:        map[string]any{"username": u.Username, "method": "password"},
	})
	h.issueTokensTo(c, u)
}

// Refresh exchanges a valid, non-revoked refresh token for a new pair and
// revokes the old refresh_tokens row. Strict rotation: one refresh token is
// single-use so a leaked token is only useful for one follow-up refresh.
func (h *AuthHandler) Refresh(c *gin.Context) {
	var req refreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	claims, err := h.tokens.ParseRefresh(req.RefreshToken)
	if err != nil {
		unauthorized(c)
		return
	}

	ctx := c.Request.Context()
	rt, err := h.db.RefreshTokens().Get(ctx, claims.TokenID)
	if err != nil || rt.RevokedAt != nil || time.Now().After(rt.ExpiresAt) {
		unauthorized(c)
		return
	}
	u, err := h.db.Users().GetByID(ctx, rt.UserID)
	if err != nil {
		unauthorized(c)
		return
	}

	// Rotate: revoke the old row, issue a fresh pair.
	if err := h.db.RefreshTokens().Revoke(ctx, rt.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "revoke old refresh"})
		return
	}
	h.issueTokensTo(c, u)
}

// ChangePassword lets the currently-authenticated user rotate their own
// password without admin intervention. On success every outstanding
// refresh token for the user is revoked (matches the admin-reset
// semantics), forcing other live sessions to re-login.
//
// Must be mounted behind RequireAuth; without that the body's
// old_password check is the only gate and anyone with a dictionary
// could brute-force via this endpoint.
func (h *AuthHandler) ChangePassword(c *gin.Context) {
	claims, ok := ClaimsFromContext(c)
	if !ok {
		unauthorized(c)
		return
	}

	var req struct {
		OldPassword string `json:"old_password" binding:"required"`
		NewPassword string `json:"new_password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.NewPassword == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "new_password must not be empty"})
		return
	}

	ctx := c.Request.Context()
	u, err := h.db.Users().GetByID(ctx, claims.UserID)
	if err != nil {
		// User deleted mid-session — their access token is technically
		// still valid until it expires, but we shouldn't honour it for
		// state-changing ops.
		unauthorized(c)
		return
	}
	if !user.VerifyPassword(u.PasswordHash, req.OldPassword) {
		unauthorized(c)
		return
	}

	hashed, err := user.HashPassword(req.NewPassword)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.db.Users().UpdatePasswordHash(ctx, u.ID, hashed); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "update password"})
		return
	}
	// Best-effort — if revocation errors, the password change still
	// succeeded and the next refresh will fail anyway once the TTL ends.
	_ = h.db.RefreshTokens().RevokeAllForUser(ctx, u.ID)
	c.Status(http.StatusNoContent)
}

// Logout revokes a single refresh token. Returns 204 even if the token is
// already revoked or unknown — logging out "harder than necessary" is not
// an error.
func (h *AuthHandler) Logout(c *gin.Context) {
	empty := ""
	var req refreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	claims, err := h.tokens.ParseRefresh(req.RefreshToken)
	if err != nil {
		// Unparseable token → nothing to revoke, but a client's "please
		// invalidate this" request still succeeds semantically.
		c.Status(http.StatusNoContent)
		return
	}
	_ = h.db.RefreshTokens().Revoke(c.Request.Context(), claims.TokenID)
	RecordActivity(c, ActivityInput{
		ProjectID:  &empty,
		ActorType:  storage.ActorTypeUser,
		ActorUser:  claims.UserID,
		Category:   storage.CategoryAuth,
		Action:     "user.logout",
		TargetType: "user",
		TargetID:   claims.UserID,
	})
	c.Status(http.StatusNoContent)
}

// issueTokensTo is the common tail of Login / Refresh / Bootstrap: mint a
// new access + refresh pair, persist the refresh row, and return the pair.
func (h *AuthHandler) issueTokensTo(c *gin.Context, u *user.User) {
	access, err := h.tokens.IssueAccess(AccessClaims{
		UserID:   u.ID,
		Username: u.Username,
		Role:     u.Role,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "issue access"})
		return
	}
	tokenID := uuid.NewString()
	refresh, err := h.tokens.IssueRefresh(RefreshClaims{
		UserID:  u.ID,
		TokenID: tokenID,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "issue refresh"})
		return
	}
	rt := &storage.RefreshToken{
		ID:        tokenID,
		UserID:    u.ID,
		ExpiresAt: time.Now().Add(h.tokens.refreshTTL).UTC(),
	}
	if err := h.db.RefreshTokens().Create(c.Request.Context(), rt); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "persist refresh"})
		return
	}
	c.JSON(http.StatusOK, tokenPair{
		AccessToken:  access,
		RefreshToken: refresh,
		User: &userBody{
			ID:       u.ID,
			Username: u.Username,
			Role:     u.Role,
		},
	})
}

func unauthorized(c *gin.Context) {
	c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
}
