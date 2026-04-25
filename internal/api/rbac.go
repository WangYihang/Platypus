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

// RBAC carries the dependencies needed for auth middleware: the token
// issuer for parsing access JWTs (legacy human auth), the storage layer
// for project_members lookups, and the opaque-token verifier that
// handles AAT (and later user-session) credentials.
type RBAC struct {
	tokens   *TokenIssuer
	storage  *storage.DB
	verifier *TokenVerifier
}

// NewRBAC builds an RBAC that can only enforce RequireAuth and
// RequireGlobalRole. RequireProjectRole requires storage and will panic
// at middleware call time if used with this constructor.
func NewRBAC(tokens *TokenIssuer) *RBAC {
	return &RBAC{tokens: tokens}
}

// NewRBACWithStorage additionally enables RequireProjectRole by providing
// the DB handle used for project_members lookups. AAT verification is
// disabled — bearers with the aat_ prefix fall through to the JWT
// parser which rejects them as invalid signatures.
func NewRBACWithStorage(tokens *TokenIssuer, db *storage.DB) *RBAC {
	return &RBAC{tokens: tokens, storage: db}
}

// NewRBACWithVerifier wires the full RBAC: JWT for humans, opaque-token
// verifier for AAT (and Phase 2 user sessions). The verifier is the only
// path that accepts AAT bearers; without it, RequireAuth treats every
// bearer as a JWT.
func NewRBACWithVerifier(tokens *TokenIssuer, db *storage.DB, verifier *TokenVerifier) *RBAC {
	return &RBAC{tokens: tokens, storage: db, verifier: verifier}
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

// RequireAuth validates the Authorization: Bearer <token> header and
// stores both the parsed AccessClaims (legacy compat) and the unified
// *Principal on the gin context.
//
// Bearer dispatch is by token format: any registered opaque prefix
// (aat_, ...) goes through the TokenVerifier; everything else falls
// through to JWT parsing. On any failure — missing header, wrong
// scheme, invalid signature, revoked / expired opaque token — the
// middleware aborts with 401.
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

// authenticate is the shared body of RequireAuth / RequireAuthWS. It
// receives the bearer payload (already past Bearer scheme parsing) and
// either stamps Principal+Claims and calls c.Next, or aborts with 401.
func (r *RBAC) authenticate(c *gin.Context, bearer string) {
	if r.verifier != nil {
		if _, _, isOpaque := optoken.DetectKind(bearer); isOpaque {
			p, reason, err := r.verifier.Verify(c.Request.Context(), bearer)
			if err != nil {
				// Internal storage error — distinct from "token bad".
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "auth lookup"})
				return
			}
			if reason != "success" || p == nil {
				abortUnauthorized(c, "invalid token")
				return
			}
			SetPrincipal(c, p)
			c.Set(claimsCtxKey, syntheticClaimsForAAT(p))
			c.Next()
			return
		}
	}
	// JWT path (humans).
	claims, err := r.tokens.ParseAccess(bearer)
	if err != nil {
		abortUnauthorized(c, "invalid access token")
		return
	}
	c.Set(claimsCtxKey, &claims)
	SetPrincipal(c, PrincipalFromClaims(claims))
	c.Next()
}

// syntheticClaimsForAAT lets pre-Principal handlers continue to read
// ClaimsFromContext on AAT-authenticated requests. UserID is set to
// the TokenID — never the issuer's user_id — so any handler that
// uses it in an audit field or "owns this row" check stays
// distinguishable from a real human user.
//
// Role mirrors the AAT's role for global tokens; project-bound AATs
// have their role floored to viewer here so the legacy admin-bypass
// in any handler still reading claims directly cannot accidentally
// escape the project binding. Handlers migrated to PrincipalFromContext
// see the real Role and the binding via Principal.IsGlobalAdmin().
func syntheticClaimsForAAT(p *Principal) *AccessClaims {
	role := p.Role
	if p.ProjectID != "" && role == user.RoleAdmin {
		role = user.RoleViewer
	}
	return &AccessClaims{
		UserID:   p.TokenID,
		Username: "<aat:" + p.TokenID + ">",
		Role:     role,
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
				if r.tryAuthenticateWS(c, parts[1]) {
					return
				}
			}
		}
		// 2) Sec-WebSocket-Protocol "Bearer.<token>" sentinel.
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
		// 3) Query-param fallback.
		if t := c.Query("access_token"); t != "" {
			if r.tryAuthenticateWS(c, t) {
				return
			}
		}
		abortUnauthorized(c, "missing or invalid websocket auth (Bearer header, Sec-WebSocket-Protocol, or ?access_token=)")
	}
}

// tryAuthenticateWS attempts the same dispatch as authenticate but
// returns ok=true only on success so the WS middleware can fall
// through to other carriers on failure. On success it stamps the
// context and calls c.Next.
func (r *RBAC) tryAuthenticateWS(c *gin.Context, bearer string) bool {
	if r.verifier != nil {
		if _, _, isOpaque := optoken.DetectKind(bearer); isOpaque {
			p, reason, err := r.verifier.Verify(c.Request.Context(), bearer)
			if err != nil || reason != "success" || p == nil {
				return false
			}
			SetPrincipal(c, p)
			c.Set(claimsCtxKey, syntheticClaimsForAAT(p))
			c.Next()
			return true
		}
	}
	claims, err := r.tokens.ParseAccess(bearer)
	if err != nil {
		return false
	}
	c.Set(claimsCtxKey, &claims)
	SetPrincipal(c, PrincipalFromClaims(claims))
	c.Next()
	return true
}

// RequireGlobalRole gates a route behind a minimum global role. Role
// ordering is admin > operator > viewer; higher roles implicitly satisfy
// lower requirements. Must be used downstream of RequireAuth.
//
// AAT principals carry the role their issuer set on the row; project-
// bound AATs are NOT lowered here (the project gate, not the global
// gate, enforces binding). A global-admin AAT therefore passes a
// RequireGlobalRole(admin) gate exactly as a human admin would.
func (r *RBAC) RequireGlobalRole(min user.Role) gin.HandlerFunc {
	return func(c *gin.Context) {
		p, ok := PrincipalFromContext(c)
		if !ok {
			abortUnauthorized(c, "no principal on context — RequireAuth missing?")
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
// (optoken.ScopesFromRole) — admin/operator hold every scope and pass
// any RequireScope gate; viewers hold only the read subset.
//
// Always run downstream of RequireAuth — and typically downstream of
// RequireProjectRole too, so the project gate runs first and emits a
// 403 with project-binding language before any scope-language 403.
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
		p, ok := PrincipalFromContext(c)
		if !ok {
			abortUnauthorized(c, "no principal on context — RequireAuth missing?")
			return
		}
		projectID := c.Param(projectParam)

		// Global admin (human or unbound admin AAT) bypasses
		// project membership but still needs to confirm the project
		// exists so 404 vs 200 matches admin reality.
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

		// AAT principals: enforce row-level project binding
		// directly. project_members never has rows for tokens, so
		// the human-path lookup below would always 403 even on the
		// AAT's own project.
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

		// Human user: project_members lookup decides membership.
		role, err := r.storage.Projects().MemberRole(c.Request.Context(), projectID, p.UserID)
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
