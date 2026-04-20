package api

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

// Auth manages Bearer Token authentication for the API.
type Auth struct {
	mu     sync.RWMutex
	tokens map[string]bool // valid tokens
	secret string          // server secret for obtaining tokens
}

// NewAuth creates an Auth instance with a random server secret.
func NewAuth() *Auth {
	secret := generateRandomHex(32)
	a := &Auth{
		tokens: make(map[string]bool),
		secret: secret,
	}
	return a
}

// GetSecret returns the server secret (printed at startup for operator).
func (a *Auth) GetSecret() string {
	return a.secret
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
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
			return
		}
		if req.Secret != a.secret {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid secret"})
			return
		}
		token := a.CreateToken()
		c.JSON(http.StatusOK, gin.H{"token": token})
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

		if !a.ValidateToken(parts[1]) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}

		c.Next()
	}
}

func generateRandomHex(n int) string {
	b := make([]byte, n)
	// crypto/rand.Read on Linux/macOS reads from getrandom(2) — it only errors
	// if the OS RNG is completely unavailable, which is fatal anyway. Ignore.
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
