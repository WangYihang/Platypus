package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/optoken"
	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

// RBAC owns the auth + project-membership middleware for the v1 REST
// surface. After the JWT pair was retired in Phase 2, the only
// dependencies left are storage (for project_members lookups) and the
// opaque-token verifier (currently the user-session bearer).
type RBAC struct {
	storage  *storage.DB
	verifier *TokenVerifier
	// rpcThrottle is the per-principal token-bucket rate limiter for
	// the high-impact agent RPC surface. Lazily allocated by
	// RPCThrottle() so RBAC zero-value still works in tests that
	// don't exercise rate-limited routes.
	rpcThrottle *rpcThrottle
}

// NewRBAC wires the full middleware: opaque-token verifier for both
// session (pst_) bearers, and storage for project gate
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
// gin context. Bearer must carry a registered optoken prefix (pst_
// for human sessions today; future scoped-token prefixes mount the
// same path); anything else is 401.
func (r *RBAC) RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if r.bearerCheck(c) {
			c.Next()
		}
	}
}

// bearerCheck is the inline form of RequireAuth: returns true when the
// principal is set and false when 401 has already been written. Used
// by middlewares that need to compose Bearer auth with an alternative
// (RequireFsReadAuth's preview-token branch) without re-entering the
// middleware chain via c.Next.
func (r *RBAC) bearerCheck(c *gin.Context) bool {
	raw := c.GetHeader("Authorization")
	if raw == "" {
		abortUnauthorized(c, "missing authorization header")
		return false
	}
	parts := strings.SplitN(raw, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		abortUnauthorized(c, "authorization header must be Bearer <token>")
		return false
	}
	if r.verifier == nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "auth verifier not wired"})
		return false
	}
	if _, _, isOpaque := optoken.DetectKind(parts[1]); !isOpaque {
		abortUnauthorized(c, "unrecognized token format")
		return false
	}
	p, reason, err := r.verifier.Verify(c.Request.Context(), parts[1])
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "auth lookup"})
		return false
	}
	if reason != "success" || p == nil {
		abortUnauthorized(c, "invalid token")
		return false
	}
	SetPrincipal(c, p)
	c.Set(claimsCtxKey, claimsForPrincipal(p))
	return true
}

// claimsForPrincipal projects a Principal into the legacy AccessClaims
// shape so handlers that haven't migrated off the JWT-era helpers
// still see consistent UserID / Username / Role values.
func claimsForPrincipal(p *Principal) *AccessClaims {
	return &AccessClaims{
		UserID:   p.UserID,
		Username: p.Username,
		Role:     p.Role,
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

// RequireGlobalRole gates a route behind a "minimum" global role.
// "Minimum" used to mean enum hierarchy (admin > operator > viewer);
// post-RBAC it means permission superset — the caller's role must
// hold every permission the named min role holds. For builtin roles
// the two definitions coincide, so existing routes don't shift
// behaviour. Custom roles work too: a role "support" with operator's
// full permission set passes RequireGlobalRole(operator); one missing
// any of operator's perms is rejected. Must run downstream of
// RequireAuth.
func (r *RBAC) RequireGlobalRole(min user.Role) gin.HandlerFunc {
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
		ok, err := r.roleHasAllOf(c.Request.Context(), string(p.Role), string(min))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError,
				gin.H{"error": "role lookup"})
			return
		}
		if !ok {
			c.AbortWithStatusJSON(http.StatusForbidden,
				gin.H{"error": "insufficient role"})
			return
		}
		c.Next()
	}
}

// RequireScope gates a route on the principal carrying a specific
// scope. Scoped opaque tokens (when re-introduced) expose only the
// scopes their issuer stamped on the row, so a viewer-scoped token
// cannot reach a route that demands hosts:exec even if its role would
// otherwise pass. Human principals derive scopes from their global
// role (optoken.ScopesFromRole) — admin/operator hold every scope
// and pass any RequireScope gate; viewers hold only the read subset.
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

// roleHasAllOf reports whether the role at userRoleSlug holds every
// permission of the role at minRoleSlug. The "permission superset"
// rule — a custom role passes a builtin-min gate iff it has all the
// builtin's perms. Returns (false, nil) when the user role is
// unknown (e.g. dropped after a migration); (false, err) only on a
// real DB failure so the caller can distinguish 403 from 500.
func (r *RBAC) roleHasAllOf(ctx context.Context, userRoleSlug, minRoleSlug string) (bool, error) {
	if userRoleSlug == "" {
		return false, nil
	}
	minRole, err := r.storage.Roles().Get(ctx, minRoleSlug)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			// A min role removed at runtime is a misconfiguration —
			// fail closed (deny everyone) rather than fail open.
			return false, nil
		}
		return false, err
	}
	for _, perm := range minRole.Permissions {
		ok, err := r.storage.Roles().HasPermission(ctx, userRoleSlug, perm)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
	}
	return true, nil
}

func abortUnauthorized(c *gin.Context, reason string) {
	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": reason})
}

// RequireProjectRole gates a route behind project-level membership.
// The URL param named by `projectParam` is resolved as a project id;
// a global admin passes regardless of membership, otherwise the
// principal must hold a project_members row with role >= min.
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
		if r.projectRoleCheck(c, projectParam, min) {
			c.Next()
		}
	}
}

// projectRoleCheck is the inline form of RequireProjectRole: same
// behaviour, but composable with custom auth chains. Returns true on
// pass; on fail the response has already been written.
func (r *RBAC) projectRoleCheck(c *gin.Context, projectParam string, min user.Role) bool {
	if r.storage == nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError,
			gin.H{"error": "RBAC missing storage — constructed without it"})
		return false
	}
	p, ok := PrincipalFromContext(c)
	if !ok {
		abortUnauthorized(c, "no principal on context — RequireAuth missing?")
		return false
	}
	projectID := c.Param(projectParam)

	if p.IsGlobalAdmin() {
		if _, err := r.storage.Projects().GetByID(c.Request.Context(), projectID); err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "project not found"})
				return false
			}
			c.AbortWithStatusJSON(http.StatusInternalServerError,
				gin.H{"error": "project lookup"})
			return false
		}
		return true
	}

	role, err := r.storage.Projects().MemberRole(c.Request.Context(), projectID, p.UserID)
	if errors.Is(err, storage.ErrNotFound) {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "insufficient project role"})
		return false
	}
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError,
			gin.H{"error": "project member lookup"})
		return false
	}
	ok2, err := r.roleHasAllOf(c.Request.Context(), string(role), string(min))
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError,
			gin.H{"error": "role lookup"})
		return false
	}
	if !ok2 {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "insufficient project role"})
		return false
	}
	return true
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
		if r.agentInProjectCheck(c, projectParam, agentParam) {
			c.Next()
		}
	}
}

// RequireFsReadAuth gates GET /fs/read with a dual-path auth scheme:
//
//  1. Standard Authorization: Bearer ... (everything else uses this).
//     Falls through the same RequireAuth + RequireProjectRole(viewer)
//     + RequireAgentInProject chain the rest of the agent routes use,
//     just inlined so they compose with branch (2) inside one MW.
//  2. Short-lived signed-URL preview token (?preview_token=, ?exp=).
//     Used by browser-native <video>/<audio>/pdf.js elements that
//     can't add a custom Authorization header. The signer is
//     process-local and rotates on every restart, so leaked URLs
//     can't outlive their 5-minute TTL or a server bounce.
//
// signer may be nil — that disables branch (2) entirely, which keeps
// the route bearer-only for tests / deployments that don't want to
// expose the signed-URL surface.
func (r *RBAC) RequireFsReadAuth(signer *PreviewSigner) gin.HandlerFunc {
	return func(c *gin.Context) {
		if signer != nil && c.Query("preview_token") != "" {
			pid := c.Param("pid")
			aid := c.Param("agent_id")
			path := c.Query("path")
			exp, err := strconv.ParseInt(c.Query("exp"), 10, 64)
			if err != nil {
				abortUnauthorized(c, "invalid preview-token exp")
				return
			}
			if !signer.Verify(pid, aid, path, exp, c.Query("preview_token")) {
				abortUnauthorized(c, "invalid preview token")
				return
			}
			// Token already encodes (pid, aid, path) and is bound to a
			// caller who passed RequireAuth + project/agent gates at
			// mint time — re-running them here would just produce the
			// same answer without a useful diagnostic, so skip and let
			// the handler run.
			c.Next()
			return
		}
		if !r.bearerCheck(c) {
			return
		}
		if !r.projectRoleCheck(c, "pid", user.RoleViewer) {
			return
		}
		if !r.agentInProjectCheck(c, "pid", "agent_id") {
			return
		}
		c.Next()
	}
}

// agentInProjectCheck is the inline form of RequireAgentInProject.
// Returns true on pass; on fail the response is already written.
func (r *RBAC) agentInProjectCheck(c *gin.Context, projectParam, agentParam string) bool {
	if r.storage == nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError,
			gin.H{"error": "RBAC missing storage — constructed without it"})
		return false
	}
	p, ok := PrincipalFromContext(c)
	if !ok {
		abortUnauthorized(c, "no principal on context — RequireAuth missing?")
		return false
	}
	pid := c.Param(projectParam)
	agentID := c.Param(agentParam)
	if agentID == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "agent_id required"})
		return false
	}
	host, err := r.storage.Hosts().GetByAgentID(c.Request.Context(), agentID)
	if errors.Is(err, storage.ErrNotFound) {
		if p.IsGlobalAdmin() {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "agent not found"})
			return false
		}
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "agent not in project"})
		return false
	}
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError,
			gin.H{"error": "agent lookup"})
		return false
	}
	if host.ProjectID != pid {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "agent not in project"})
		return false
	}
	return true
}
