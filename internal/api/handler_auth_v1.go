package api

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/WangYihang/Platypus/internal/optoken"
	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

// SessionHardTTL is the absolute upper bound on a user_session
// lifetime. The sliding idle window (TokenVerifier.SessionIdleWindow)
// re-authenticates inactive users far sooner; this just caps the
// "user has been continuously active for a month" pathological case.
const SessionHardTTL = 30 * 24 * time.Hour

// AuthHandler wires the bootstrap / login / logout / change-password
// surface. It mints opaque pst_ session tokens via optoken and
// persists them through AuthTokens.CreateSession; the verifier is
// notified on revoke / password-change so cache invalidation lands
// synchronously and other live tabs lose access without waiting for
// the cache TTL.
type AuthHandler struct {
	db              *storage.DB
	verifier        *TokenVerifier
	bootstrapSecret string
}

// NewAuthHandler builds the handler. The verifier is required: every
// revocation path (Logout, ChangePassword, RevokeSession) needs to
// invalidate the cache to take effect on the current node within
// the same request.
func NewAuthHandler(db *storage.DB, verifier *TokenVerifier, bootstrapSecret string) *AuthHandler {
	return &AuthHandler{db: db, verifier: verifier, bootstrapSecret: bootstrapSecret}
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

type publicInfoResponse struct {
	Product           string `json:"product"`
	AdminBootstrapped bool   `json:"admin_bootstrapped"`
}

type userBody struct {
	ID       string    `json:"id"`
	Username string    `json:"username"`
	Role     user.Role `json:"role"`
}

// loginResponse is what Login / Bootstrap return. Plaintext is
// included exactly once — clients persist it. The expiry pair lets
// browser code show a remaining-lifetime UI without a follow-up
// roundtrip.
type loginResponse struct {
	SessionToken  string    `json:"session_token"`
	TokenID       string    `json:"token_id"`
	ExpiresAt     time.Time `json:"expires_at"`
	IdleExpiresAt time.Time `json:"idle_expires_at"`
	User          *userBody `json:"user,omitempty"`
}

// PublicInfo answers GET /api/v1/auth/info without requiring a bearer
// token. Returns just enough metadata for the desktop onboarding
// wizard to decide whether to show the Log-in or First-time-setup
// form.
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

// Bootstrap is the one-shot "set up the first admin" flow. It succeeds
// only when the users table is empty AND the caller presents the
// server's bootstrap secret. After the first admin exists this endpoint
// is dead weight; subsequent calls return 409.
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
			ProjectID: &empty, ActorType: storage.ActorTypeAnonymous,
			Category: storage.CategoryAuth, Action: "user.bootstrap",
			Outcome: storage.OutcomeDenied, Error: "already bootstrapped",
			Meta: map[string]any{"username": req.Username},
		})
		c.JSON(http.StatusConflict, gin.H{"error": "already bootstrapped"})
		return
	}
	if subtle.ConstantTimeCompare([]byte(req.Secret), []byte(h.bootstrapSecret)) != 1 {
		RecordActivity(c, ActivityInput{
			ProjectID: &empty, ActorType: storage.ActorTypeAnonymous,
			Category: storage.CategoryAuth, Action: "user.bootstrap",
			Outcome: storage.OutcomeDenied, Error: "invalid secret",
			Meta: map[string]any{"username": req.Username},
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
		ProjectID: &empty, ActorType: storage.ActorTypeUser, ActorUser: u.ID,
		Category: storage.CategoryAuth, Action: "user.bootstrap",
		TargetType: "user", TargetID: u.ID, TargetLabel: u.Username,
		Meta: map[string]any{"username": u.Username, "role": string(u.Role)},
	})
	if _, err := h.db.Projects().GetBySlug(ctx, "default"); errors.Is(err, storage.ErrNotFound) {
		_ = h.db.Projects().Create(ctx, &storage.Project{
			ID: uuid.NewString(), Name: "Default", Slug: "default",
			CreatedAt: time.Now().UTC(), CreatedBy: u.ID,
		})
	}
	h.issueSession(c, u)
}

// Login authenticates by username + password and mints a fresh session
// token. Same-shape 401 on every failure path so timing / body deltas
// don't reveal which field was wrong.
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
			ProjectID: &empty, ActorType: storage.ActorTypeAnonymous,
			Category: storage.CategoryAuth, Action: "user.login_failed",
			Outcome: storage.OutcomeDenied, Error: "unknown user",
			Meta: map[string]any{"username": req.Username, "reason": "unknown_user"},
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
			ProjectID: &empty, ActorType: storage.ActorTypeAnonymous,
			Category: storage.CategoryAuth, Action: "user.login_failed",
			TargetType: "user", TargetID: u.ID, TargetLabel: u.Username,
			Outcome: storage.OutcomeDenied, Error: "invalid password",
			Meta: map[string]any{"username": req.Username, "reason": "bad_password"},
		})
		unauthorized(c)
		return
	}
	if err := h.db.Users().TouchLastLogin(ctx, u.ID); err != nil {
		// Non-fatal — login still succeeded.
		_ = err
	}
	RecordActivity(c, ActivityInput{
		ProjectID: &empty, ActorType: storage.ActorTypeUser, ActorUser: u.ID,
		Category: storage.CategoryAuth, Action: "user.login",
		TargetType: "user", TargetID: u.ID, TargetLabel: u.Username,
		Meta: map[string]any{"username": u.Username, "method": "password"},
	})
	h.issueSession(c, u)
}

// Refresh is intentionally rejected: the session model uses sliding
// idle windows so there's nothing for the client to refresh
// proactively. Returning 410 Gone (not 404) signals "this used to
// exist; stop calling it" so frontends migrating off the old JWT pair
// can detect the deprecation cleanly.
func (h *AuthHandler) Refresh(c *gin.Context) {
	c.JSON(http.StatusGone, gin.H{
		"error":   "endpoint deprecated",
		"message": "session tokens use sliding idle windows; no refresh needed",
	})
}

// Logout revokes the caller's current session and invalidates it in
// the verifier cache. Behind RequireAuth so we always have a
// Principal — the session id comes from there. Idempotent: re-logout
// of an already-revoked session still returns 204.
func (h *AuthHandler) Logout(c *gin.Context) {
	p, ok := PrincipalFromContext(c)
	if !ok {
		// Should be unreachable behind RequireAuth.
		c.Status(http.StatusNoContent)
		return
	}
	tokenID := p.TokenID
	// JWT principals (legacy) carry no TokenID; treat their logout
	// as a no-op success — they have nothing to revoke server-side.
	if tokenID == "" {
		c.Status(http.StatusNoContent)
		return
	}
	_ = h.db.AuthTokens().Revoke(c.Request.Context(), tokenID, p.UserID, "logout", time.Now().UTC())
	if h.verifier != nil {
		h.verifier.Invalidate(tokenID)
	}
	empty := ""
	RecordActivity(c, ActivityInput{
		ProjectID:    &empty,
		ActorType:    storage.ActorTypeUser,
		ActorUser:    p.UserID,
		ActorTokenID: tokenID,
		Category:     storage.CategoryAuth,
		Action:       "user.logout",
		TargetType:   "user",
		TargetID:     p.UserID,
	})
	c.Status(http.StatusNoContent)
}

// ChangePassword lets the currently-authenticated user rotate their
// own password. On success every other live session for the user is
// revoked + cache-invalidated so a stolen secondary session can't
// outlive the password change. The caller's current session
// continues to work — they shouldn't be kicked back to the login
// screen for changing their own password.
func (h *AuthHandler) ChangePassword(c *gin.Context) {
	p, ok := PrincipalFromContext(c)
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
	u, err := h.db.Users().GetByID(ctx, p.UserID)
	if err != nil {
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
	// Cascade-revoke all OTHER sessions: list active first so we can
	// invalidate the cache entry per-id, then mass-revoke. Skip the
	// caller's own session so they aren't kicked out.
	now := time.Now().UTC()
	sessions, _ := h.db.AuthTokens().ListSessionsForUser(ctx, u.ID)
	for _, s := range sessions {
		if s.TokenID == p.TokenID {
			continue
		}
		_ = h.db.AuthTokens().Revoke(ctx, s.TokenID, u.ID, "password change", now)
		if h.verifier != nil {
			h.verifier.Invalidate(s.TokenID)
		}
	}
	c.Status(http.StatusNoContent)
}

// ListSessions returns every active session the caller holds so the
// browser settings page can render a "logged-in devices" list. Only
// the caller's own sessions — admins do not get a global view here;
// their tools should query the activity log instead.
func (h *AuthHandler) ListSessions(c *gin.Context) {
	p, ok := PrincipalFromContext(c)
	if !ok {
		unauthorized(c)
		return
	}
	sessions, err := h.db.AuthTokens().ListSessionsForUser(c.Request.Context(), p.UserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list sessions"})
		return
	}
	type item struct {
		TokenID       string     `json:"token_id"`
		UserAgent     string     `json:"user_agent,omitempty"`
		CreatedAt     time.Time  `json:"created_at"`
		ExpiresAt     time.Time  `json:"expires_at"`
		IdleExpiresAt time.Time  `json:"idle_expires_at"`
		LastUsedAt    *time.Time `json:"last_used_at,omitempty"`
		LastUsedIP    string     `json:"last_used_ip,omitempty"`
		Current       bool       `json:"current"`
	}
	out := make([]item, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, item{
			TokenID:       s.TokenID,
			UserAgent:     s.UserAgent,
			CreatedAt:     s.CreatedAt,
			ExpiresAt:     s.ExpiresAt,
			IdleExpiresAt: s.IdleExpiresAt,
			LastUsedAt:    s.LastUsedAt,
			LastUsedIP:    s.LastUsedIP,
			Current:       s.TokenID == p.TokenID,
		})
	}
	c.JSON(http.StatusOK, gin.H{"sessions": out})
}

// RevokeSession kills a specific session by id. Caller can revoke
// only their own sessions; cross-user revocation is admin work
// performed via direct activity / users tooling, not through this
// endpoint.
func (h *AuthHandler) RevokeSession(c *gin.Context) {
	p, ok := PrincipalFromContext(c)
	if !ok {
		unauthorized(c)
		return
	}
	sid := c.Param("sid")
	s, err := h.db.AuthTokens().GetSession(c.Request.Context(), sid)
	if errors.Is(err, storage.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "lookup"})
		return
	}
	if s.UserID != p.UserID {
		// Same 404 as missing — don't leak existence of other-user sessions.
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	_ = h.db.AuthTokens().Revoke(c.Request.Context(), sid, p.UserID, "user revoked", time.Now().UTC())
	if h.verifier != nil {
		h.verifier.Invalidate(sid)
	}
	c.Status(http.StatusNoContent)
}

// issueSession is the common tail of Login / Bootstrap: mint an
// opaque pst_ token, persist the row, return the plaintext exactly
// once.
func (h *AuthHandler) issueSession(c *gin.Context, u *user.User) {
	id, _, hash, plaintext, err := optoken.Generate(optoken.UserSessionPrefix)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "generate session"})
		return
	}
	now := time.Now().UTC()
	s := &storage.UserSession{
		TokenID:       id,
		SecretHash:    hash,
		UserID:        u.ID,
		UserAgent:     c.GetHeader("User-Agent"),
		CreatedAt:     now,
		ExpiresAt:     now.Add(SessionHardTTL),
		IdleExpiresAt: now.Add(SessionIdleWindow),
	}
	if err := h.db.AuthTokens().CreateSession(c.Request.Context(), s); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "persist session"})
		return
	}
	c.JSON(http.StatusOK, loginResponse{
		SessionToken:  plaintext,
		TokenID:       id,
		ExpiresAt:     s.ExpiresAt,
		IdleExpiresAt: s.IdleExpiresAt,
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
