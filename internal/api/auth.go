package api

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/optoken"
)

// Auth manages Bearer Token authentication for the API.
type Auth struct {
	mu        sync.RWMutex
	tokens    map[string]bool // valid tokens (legacy single-secret path)
	secret    string          // server secret for obtaining tokens
	wsTickets *wsTicketStore  // short-lived one-shot tickets for WS auth
	// opaqueVerifier, if set, lets Middleware also accept opaque
	// session tokens alongside the legacy single-secret path.
	// Wired by main.go after the verifier is constructed so that
	// browser clients (which carry pst_ session tokens) can hit
	// endpoints still mounted under the legacy middleware —
	// notably /api/v1/ws/ticket.
	opaqueVerifier *TokenVerifier
}

// NewAuth creates an Auth instance with a random server secret.
func NewAuth() *Auth {
	secret := generateRandomHex(32)
	a := &Auth{
		tokens:    make(map[string]bool),
		secret:    secret,
		wsTickets: newWSTicketStore(),
	}
	return a
}

// IssueWSTicket mints a short-lived, one-shot ticket for WebSocket auth.
// Browsers can't set Bearer headers on a WS upgrade, so clients trade a
// Bearer token for a ticket and pass ?ticket=<value> on the WS URL.
func (a *Auth) IssueWSTicket() string {
	return a.wsTickets.Issue()
}

// ConsumeWSTicket validates and consumes a ticket. Returns true on success.
func (a *Auth) ConsumeWSTicket(t string) bool {
	return a.wsTickets.Consume(t)
}

// GetSecret returns the server secret (printed at startup for operator).
func (a *Auth) GetSecret() string {
	return a.secret
}

// SetOpaqueVerifier enables opaque-token (session) acceptance in
// Middleware(). Pass the same TokenVerifier used by RBAC. Optional —
// without it Middleware() only accepts legacy single-secret tokens.
func (a *Auth) SetOpaqueVerifier(v *TokenVerifier) {
	a.opaqueVerifier = v
}

// CreateToken generates a new bearer token and registers it.
func (a *Auth) CreateToken() string {
	token := generateRandomHex(32)
	a.mu.Lock()
	a.tokens[token] = true
	a.mu.Unlock()
	return token
}

// ValidateToken checks if a token is valid.
func (a *Auth) ValidateToken(token string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.tokens[token]
}

// tokenRequest is the POST body for /api/v1/auth/token.
type tokenRequest struct {
	// Secret is the value printed at server startup ("API secret: …").
	Secret string `json:"secret" binding:"required"`
}

// tokenResponse is the successful response body.
type tokenResponse struct {
	Token string `json:"token"`
}

// tokenError is the error body when auth fails.
type tokenError struct {
	Error string `json:"error"`
}

// TokenEndpoint exchanges the server secret for a Bearer token.
//
// @Summary     Exchange secret for token
// @Description Bootstraps a session. The returned token must be sent as `Authorization: Bearer <token>` on every other endpoint.
// @Tags        auth
// @Accept      json
// @Produce     json
// @Param       body body      tokenRequest   true "Server secret"
// @Success     200  {object}  tokenResponse
// @Failure     400  {object}  tokenError
// @Failure     401  {object}  tokenError
// @Router      /api/v1/auth/token [post]
func (a *Auth) TokenEndpoint() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Secret string `json:"secret"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, tokenError{Error: "invalid request"})
			return
		}
		if req.Secret != a.secret {
			c.JSON(http.StatusUnauthorized, tokenError{Error: "invalid secret"})
			return
		}
		token := a.CreateToken()
		c.JSON(http.StatusOK, tokenResponse{Token: token})
	}
}

// Middleware returns a gin middleware that validates Bearer tokens.
// Requests without valid tokens get 401 Unauthorized.
func (a *Auth) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing authorization header"})
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid authorization format"})
			return
		}

		if a.ValidateToken(parts[1]) {
			c.Next()
			return
		}
		if a.opaqueVerifier != nil {
			if _, _, isOpaque := optoken.DetectKind(parts[1]); isOpaque {
				p, reason, err := a.opaqueVerifier.Verify(c.Request.Context(), parts[1])
				if err == nil && reason == "success" && p != nil {
					SetPrincipal(c, p)
					c.Next()
					return
				}
			}
		}
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
	}
}

func generateRandomHex(n int) string {
	b := make([]byte, n)
	// crypto/rand.Read on Linux/macOS reads from getrandom(2) — it only errors
	// if the OS RNG is completely unavailable, which is fatal anyway. Ignore.
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
