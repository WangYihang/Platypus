package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

// RBAC carries the dependencies needed for auth middleware: the token
// issuer for parsing access JWTs, and the storage layer for project_members
// lookups used by RequireProjectRole.
type RBAC struct {
	tokens  *TokenIssuer
	storage *storage.DB
}

// NewRBAC builds an RBAC that can only enforce RequireAuth and
// RequireGlobalRole. RequireProjectRole requires storage and will panic
// at middleware call time if used with this constructor.
func NewRBAC(tokens *TokenIssuer) *RBAC {
	return &RBAC{tokens: tokens}
}

// NewRBACWithStorage additionally enables RequireProjectRole by providing
// the DB handle used for project_members lookups.
func NewRBACWithStorage(tokens *TokenIssuer, db *storage.DB) *RBAC {
	return &RBAC{tokens: tokens, storage: db}
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

// RequireAuthWS is RequireAuth's WebSocket-friendly cousin. Browsers
// can't set Authorization: Bearer on the WebSocket upgrade, so this
// middleware accepts three credential carriers (in order):
//
//  1. Authorization: Bearer <jwt>           — native clients.
//  2. Sec-WebSocket-Protocol: ..., Bearer.<jwt>
//     The browser passes the JWT as a sentinel subprotocol entry
//     alongside the real one (e.g. ["tty", "Bearer.<jwt>"]). The
//     handler still negotiates only its real subprotocol via
//     websocket.Accept's Subprotocols list, so the auth sentinel is
//     dropped and never reaches the live connection.
//  3. ?access_token=<jwt>                    — last-resort fallback
//     for tools that can't set custom subprotocols. Use sparingly;
//     query strings are easier to leak via referer / logs.
//
// On success the parsed AccessClaims are stamped on the gin context
// just like RequireAuth, so downstream RequireProjectRole etc. work
// transparently.
func (r *RBAC) RequireAuthWS() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1) Bearer header (native clients).
		if h := c.GetHeader("Authorization"); h != "" {
			parts := strings.SplitN(h, " ", 2)
			if len(parts) == 2 && strings.EqualFold(parts[0], "bearer") {
				if claims, err := r.tokens.ParseAccess(parts[1]); err == nil {
					c.Set(claimsCtxKey, &claims)
					c.Next()
					return
				}
			}
		}
		// 2) Sec-WebSocket-Protocol "Bearer.<jwt>" sentinel.
		if h := c.GetHeader("Sec-WebSocket-Protocol"); h != "" {
			for _, p := range strings.Split(h, ",") {
				p = strings.TrimSpace(p)
				if !strings.HasPrefix(p, "Bearer.") {
					continue
				}
				jwt := strings.TrimPrefix(p, "Bearer.")
				if claims, err := r.tokens.ParseAccess(jwt); err == nil {
					c.Set(claimsCtxKey, &claims)
					c.Next()
					return
				}
			}
		}
		// 3) Query-param fallback.
		if t := c.Query("access_token"); t != "" {
			if claims, err := r.tokens.ParseAccess(t); err == nil {
				c.Set(claimsCtxKey, &claims)
				c.Next()
				return
			}
		}
		abortUnauthorized(c, "missing or invalid websocket auth (Bearer header, Sec-WebSocket-Protocol, or ?access_token=)")
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

// RequireProjectRole gates a route behind project-level membership. The
// URL param named by `projectParam` is resolved as a project id; a global
// admin passes regardless of membership, otherwise the user must hold a
// project_members row for that project with role >= min.
//
// Not-found vs forbidden: if the project doesn't exist we return 404 so
// the route is indistinguishable from a missing record for users without
// access (it already 403s before we reach the NotFound codepath for
// non-admins). Admins get the honest 404.
func (r *RBAC) RequireProjectRole(projectParam string, min user.Role) gin.HandlerFunc {
	if r.storage == nil {
		return func(c *gin.Context) {
			c.AbortWithStatusJSON(http.StatusInternalServerError,
				gin.H{"error": "RBAC missing storage — constructed without it"})
		}
	}
	return func(c *gin.Context) {
		claims, ok := ClaimsFromContext(c)
		if !ok {
			abortUnauthorized(c, "no claims on context — RequireAuth missing?")
			return
		}
		projectID := c.Param(projectParam)

		if claims.Role == user.RoleAdmin {
			if _, err := r.storage.Projects().GetByID(c.Request.Context(), projectID); err != nil {
				if errors.Is(err, storage.ErrNotFound) {
					c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "project not found"})
					return
				}
				c.AbortWithStatusJSON(http.StatusInternalServerError,
					gin.H{"error": "project lookup"})
				return
			}
			c.Next()
			return
		}

		role, err := r.storage.Projects().MemberRole(c.Request.Context(), projectID, claims.UserID)
		if errors.Is(err, storage.ErrNotFound) {
			// No membership row: pretend the project doesn't exist for
			// non-admins. Avoids disclosing project existence to
			// unauthorized users.
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "insufficient project role"})
			return
		}
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError,
				gin.H{"error": "project member lookup"})
			return
		}
		if !roleAtLeast(role, min) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "insufficient project role"})
			return
		}
		c.Next()
	}
}

// RequireAgentInProject closes the gap between the project-membership
// gate (RequireProjectRole, which only checks the *user* against pid)
// and per-agent endpoints. Without it, a viewer of project A could
// pass `:pid=A&:agent_id=<agent in project B>` and reach project B's
// agent. The middleware looks up the host row for :agent_id and 403s
// when its project_id doesn't match the URL :pid.
//
// Status codes mirror RequireProjectRole's discretion: a non-admin who
// asks about an unknown agent gets 403 (not 404) so the route doesn't
// leak which agent ids exist; admins get a 404. Must run downstream of
// RequireAuth + RequireProjectRole so claims and pid have already been
// validated.
func (r *RBAC) RequireAgentInProject(projectParam, agentParam string) gin.HandlerFunc {
	if r.storage == nil {
		return func(c *gin.Context) {
			c.AbortWithStatusJSON(http.StatusInternalServerError,
				gin.H{"error": "RBAC missing storage — constructed without it"})
		}
	}
	return func(c *gin.Context) {
		claims, ok := ClaimsFromContext(c)
		if !ok {
			abortUnauthorized(c, "no claims on context — RequireAuth missing?")
			return
		}
		pid := c.Param(projectParam)
		agentID := c.Param(agentParam)
		if agentID == "" {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "agent_id required"})
			return
		}
		host, err := r.storage.Hosts().GetByAgentID(c.Request.Context(), agentID)
		if errors.Is(err, storage.ErrNotFound) {
			if claims.Role == user.RoleAdmin {
				c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "agent not found"})
				return
			}
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "agent not in project"})
			return
		}
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError,
				gin.H{"error": "agent lookup"})
			return
		}
		if host.ProjectID != pid {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "agent not in project"})
			return
		}
		c.Next()
	}
}
