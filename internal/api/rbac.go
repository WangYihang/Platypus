package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/user"
)

// RBAC carries the dependencies needed for auth middleware. Today that's
// just the TokenIssuer for parsing access JWTs. When per-project ACLs land
// (P2), a *storage.DB field will be added for project_members lookups.
type RBAC struct {
	tokens *TokenIssuer
}

func NewRBAC(tokens *TokenIssuer) *RBAC {
	return &RBAC{tokens: tokens}
}

// claimsCtxKey is the Gin context key under which RequireAuth stores the
// parsed AccessClaims for downstream handlers.
const claimsCtxKey = "platypus.auth.claims"

// ClaimsFromContext returns the AccessClaims set by RequireAuth on success.
// The second return is false when the middleware hasn't run or the token
// was invalid — in which case the handler should not have been reached.
func ClaimsFromContext(c *gin.Context) (*AccessClaims, bool) {
	v, ok := c.Get(claimsCtxKey)
	if !ok {
		return nil, false
	}
	claims, ok := v.(*AccessClaims)
	return claims, ok
}

// RequireAuth validates the Authorization: Bearer <jwt> header and stores
// the parsed AccessClaims on the Gin context. On any failure — missing
// header, wrong scheme, invalid signature, expired token — it aborts with
// 401 so downstream handlers never see a half-authenticated request.
func (r *RBAC) RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := c.GetHeader("Authorization")
		if raw == "" {
			abortUnauthorized(c, "missing authorization header")
			return
		}
		parts := strings.SplitN(raw, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
			abortUnauthorized(c, "authorization header must be Bearer <token>")
			return
		}
		claims, err := r.tokens.ParseAccess(parts[1])
		if err != nil {
			abortUnauthorized(c, "invalid access token")
			return
		}
		c.Set(claimsCtxKey, &claims)
		c.Next()
	}
}

// RequireGlobalRole gates a route behind a minimum global role. Role
// ordering is admin > operator > viewer; higher roles implicitly satisfy
// lower requirements. Must be used downstream of RequireAuth.
func (r *RBAC) RequireGlobalRole(min user.Role) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, ok := ClaimsFromContext(c)
		if !ok {
			abortUnauthorized(c, "no claims on context — RequireAuth missing?")
			return
		}
		if !roleAtLeast(claims.Role, min) {
			c.AbortWithStatusJSON(http.StatusForbidden,
				gin.H{"error": "insufficient role"})
			return
		}
		c.Next()
	}
}

// roleAtLeast encodes the role ordering. Kept as a switch rather than a map
// so the compiler catches typos on Role renames.
func roleAtLeast(got, min user.Role) bool {
	rank := func(r user.Role) int {
		switch r {
		case user.RoleAdmin:
			return 3
		case user.RoleOperator:
			return 2
		case user.RoleViewer:
			return 1
		default:
			return 0
		}
	}
	return rank(got) >= rank(min)
}

func abortUnauthorized(c *gin.Context, reason string) {
	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": reason})
}
