package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/optoken"
	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

// RBAC owns the auth + project-membership middleware for the v1 REST
// surface. After the JWT pair was retired in Phase 2, the only
// dependencies left are storage (for project_members lookups) and the
// opaque-token verifier (for session and AAT bearers).
type RBAC struct {
	storage  *storage.DB
	verifier *TokenVerifier
}

// NewRBAC wires the full middleware: opaque-token verifier for both
// session (pst_) and AAT (aat_) bearers, and storage for project gate
// lookups. There is no longer a JWT path — RequireAuth rejects any
// bearer whose prefix doesn't match a registered optoken kind.
func NewRBAC(db *storage.DB, verifier *TokenVerifier) *RBAC {
	return &RBAC{storage: db, verifier: verifier}
}

// AccessClaims is the legacy claim shape kept on gin.Context for
// backward compat with handlers that haven't migrated to
// PrincipalFromContext. It's not a JWT claim set anymore; the auth
// middleware populates it directly from the verified Principal so
// existing `claims.UserID` / `claims.Role` reads keep working without
// each handler being rewritten.
type AccessClaims struct {
	UserID   string
	Username string
	Role     user.Role
}

// claimsCtxKey is the gin context slot for the legacy AccessClaims.
const claimsCtxKey = "platypus.auth.claims"

// ClaimsFromContext returns the AccessClaims stamped by RequireAuth on
// success. Prefer PrincipalFromContext in new code — that carries
// kind, scopes, and project binding too.
func ClaimsFromContext(c *gin.Context) (*AccessClaims, bool) {
	v, ok := c.Get(claimsCtxKey)
	if !ok {
		return nil, false
	}
	claims, ok := v.(*AccessClaims)
	return claims, ok
}

// RequireAuth validates the Authorization: Bearer <token> header and
// stores both the unified *Principal and a legacy AccessClaims on the
// gin context. Bearer must carry a registered optoken prefix (pst_ for
// human sessions, aat_ for AI agent tokens); anything else is 401.
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
		r.authenticate(c, parts[1])
	}
}

// authenticate is the shared body of RequireAuth / RequireAuthWS. On
// success it stamps Principal + AccessClaims and calls c.Next; on
// failure it aborts with 401.
func (r *RBAC) authenticate(c *gin.Context, bearer string) {
	if r.verifier == nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "auth verifier not wired"})
		return
	}
	if _, _, isOpaque := optoken.DetectKind(bearer); !isOpaque {
		abortUnauthorized(c, "unrecognized token format")
		return
	}
	p, reason, err := r.verifier.Verify(c.Request.Context(), bearer)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "auth lookup"})
		return
	}
	if reason != "success" || p == nil {
		abortUnauthorized(c, "invalid token")
		return
	}
	SetPrincipal(c, p)
	c.Set(claimsCtxKey, claimsForPrincipal(p))
	c.Next()
}

// claimsForPrincipal projects a Principal into the legacy AccessClaims
// shape. Human sessions get their real UserID / Username / Role.
// AAT principals get UserID=TokenID + a sentinel username + the
// AAT's role floored to viewer when project-bound — that way any
// pre-Principal handler reading claims.UserID stays distinguishable
// from a real human, and the legacy "claims.Role==admin" bypass
// can't accidentally escape an AAT's project binding.
func claimsForPrincipal(p *Principal) *AccessClaims {
	switch p.Kind {
	case PrincipalAATKind:
		role := p.Role
		if p.ProjectID != "" && role == user.RoleAdmin {
			role = user.RoleViewer
		}
		return &AccessClaims{
			UserID:   p.TokenID,
			Username: "<aat:" + p.TokenID + ">",
			Role:     role,
		}
	default: // PrincipalUser
		return &AccessClaims{
			UserID:   p.UserID,
			Username: p.Username,
			Role:     p.Role,
		}
	}
}

// RequireAuthWS is RequireAuth's WebSocket-friendly cousin. Browsers
// can't set Authorization: Bearer on the WebSocket upgrade, so this
// middleware accepts two credential carriers:
//
//  1. Authorization: Bearer <token>          — native clients.
//  2. Sec-WebSocket-Protocol: ..., Bearer.<token>
//     The browser passes the token as a sentinel subprotocol entry
//     alongside the real one (e.g. ["tty", "Bearer.<token>"]). The
//     handler negotiates only its real subprotocol via
//     websocket.Accept's Subprotocols list, so the auth sentinel is
//     dropped and never reaches the live connection.
//
// A third option (?access_token=<token>) used to exist as a fallback
// for tools that couldn't set custom subprotocols, but security
// audit M3 retired it: query strings end up in nginx / cloudflare
// access logs and HTTP referer headers, and a 30-day session token
// in either is a long-lived credential leak. Tools that can't set
// subprotocols must add Authorization headers instead.
func (r *RBAC) RequireAuthWS() gin.HandlerFunc {
	return func(c *gin.Context) {
		if h := c.GetHeader("Authorization"); h != "" {
			parts := strings.SplitN(h, " ", 2)
			if len(parts) == 2 && strings.EqualFold(parts[0], "bearer") {
				if r.tryAuthenticateWS(c, parts[1]) {
					return
				}
			}
		}
		if h := c.GetHeader("Sec-WebSocket-Protocol"); h != "" {
			for _, p := range strings.Split(h, ",") {
				p = strings.TrimSpace(p)
				if !strings.HasPrefix(p, "Bearer.") {
					continue
				}
				if r.tryAuthenticateWS(c, strings.TrimPrefix(p, "Bearer.")) {
					return
				}
			}
		}
		abortUnauthorized(c, "missing or invalid websocket auth (Bearer header or Sec-WebSocket-Protocol)")
	}
}

func (r *RBAC) tryAuthenticateWS(c *gin.Context, bearer string) bool {
	if r.verifier == nil {
		return false
	}
	if _, _, isOpaque := optoken.DetectKind(bearer); !isOpaque {
		return false
	}
	p, reason, err := r.verifier.Verify(c.Request.Context(), bearer)
	if err != nil || reason != "success" || p == nil {
		return false
	}
	SetPrincipal(c, p)
	c.Set(claimsCtxKey, claimsForPrincipal(p))
	c.Next()
	return true
}

// RequireGlobalRole gates a route behind a minimum global role. Role
// ordering is admin > operator > viewer; higher roles implicitly
// satisfy lower requirements. Must run downstream of RequireAuth.
//
// AAT principals carry the role their issuer set on the row. A global
// (unbound) AAT with role >= min passes the gate exactly as a human
// would. A project-bound AAT NEVER passes — the issuer scoped the
// token out of any global resource by binding it, regardless of the
// role they stamped on it. Without this check a project-bound
// role=admin AAT would have satisfied RequireGlobalRole(admin), since
// roleAtLeast reads p.Role directly and the legacy claims downgrade
// in claimsForPrincipal only protects handlers that read AccessClaims.
func (r *RBAC) RequireGlobalRole(min user.Role) gin.HandlerFunc {
	return func(c *gin.Context) {
		p, ok := PrincipalFromContext(c)
		if !ok {
			abortUnauthorized(c, "no principal on context — RequireAuth missing?")
			return
		}
		if p.Kind == PrincipalAATKind && p.ProjectID != "" {
			c.AbortWithStatusJSON(http.StatusForbidden,
				gin.H{"error": "project-bound token cannot access global resources"})
			return
		}
		if !roleAtLeast(p.Role, min) {
			c.AbortWithStatusJSON(http.StatusForbidden,
				gin.H{"error": "insufficient role"})
			return
		}
		c.Next()
	}
}

// RequireScope gates a route on the principal carrying a specific
// scope. AAT principals expose only the scopes their issuer stamped
// on the row, so a viewer-AAT cannot reach a route that demands
// hosts:exec even if its global role would otherwise pass. Human
// principals derive scopes from their global role
// (optoken.ScopesFromRole) — admin/operator hold every scope and
// pass any RequireScope gate; viewers hold only the read subset.
func (r *RBAC) RequireScope(scope string) gin.HandlerFunc {
	return func(c *gin.Context) {
		p, ok := PrincipalFromContext(c)
		if !ok {
			abortUnauthorized(c, "no principal on context — RequireAuth missing?")
			return
		}
		if !optoken.HasScope(p.Scopes, scope) {
			c.AbortWithStatusJSON(http.StatusForbidden,
				gin.H{"error": "missing scope: " + scope})
			return
		}
		c.Next()
	}
}

// roleAtLeast encodes the role ordering. Kept as a switch rather than
// a map so the compiler catches typos on Role renames.
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

// RequireProjectRole gates a route behind project-level membership.
// The URL param named by `projectParam` is resolved as a project id;
// a global admin (human OR unbound admin AAT) passes regardless of
// membership, otherwise the principal must hold a project_members row
// with role >= min — or, for AAT principals, be bound to that exact
// project.
//
// Not-found vs forbidden: if the project doesn't exist we return 404
// for global admins (the honest answer) and 403 for everyone else (so
// the route doesn't leak project-id existence to unauthorized users).
func (r *RBAC) RequireProjectRole(projectParam string, min user.Role) gin.HandlerFunc {
	if r.storage == nil {
		return func(c *gin.Context) {
			c.AbortWithStatusJSON(http.StatusInternalServerError,
				gin.H{"error": "RBAC missing storage — constructed without it"})
		}
	}
	return func(c *gin.Context) {
		p, ok := PrincipalFromContext(c)
		if !ok {
			abortUnauthorized(c, "no principal on context — RequireAuth missing?")
			return
		}
		projectID := c.Param(projectParam)

		if p.IsGlobalAdmin() {
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

		// AAT principals: enforce row-level project binding directly.
		// project_members never has rows for tokens, so the human-path
		// lookup below would always 403 even on the AAT's own project.
		if p.Kind == PrincipalAATKind {
			if p.ProjectID == "" || p.ProjectID != projectID {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "token not bound to this project"})
				return
			}
			if !roleAtLeast(p.Role, min) {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "insufficient project role"})
				return
			}
			c.Next()
			return
		}

		role, err := r.storage.Projects().MemberRole(c.Request.Context(), projectID, p.UserID)
		if errors.Is(err, storage.ErrNotFound) {
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
// leak which agent ids exist; admins get a 404. Must run downstream
// of RequireAuth + RequireProjectRole so the principal and pid have
// already been validated.
func (r *RBAC) RequireAgentInProject(projectParam, agentParam string) gin.HandlerFunc {
	if r.storage == nil {
		return func(c *gin.Context) {
			c.AbortWithStatusJSON(http.StatusInternalServerError,
				gin.H{"error": "RBAC missing storage — constructed without it"})
		}
	}
	return func(c *gin.Context) {
		p, ok := PrincipalFromContext(c)
		if !ok {
			abortUnauthorized(c, "no principal on context — RequireAuth missing?")
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
			if p.IsGlobalAdmin() {
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
