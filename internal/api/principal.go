package api

import (
	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/optoken"
	"github.com/WangYihang/Platypus/internal/user"
)

// PrincipalKind tags the credential the request was authenticated with.
// User-kind principals are humans (browser session, JWT today); AAT-kind
// are AI / programmatic callers carrying a scoped opaque token.
type PrincipalKind int

const (
	PrincipalUser PrincipalKind = iota
	PrincipalAATKind
)

// Principal is the authenticated subject of a request. RBAC and audit
// downstream of RequireAuth read it from the gin context — they don't
// care which credential was presented, only what the principal is
// allowed to do. The shape is identical for human and AAT callers
// except for the AAT-only TokenID and ProjectID-binding fields.
type Principal struct {
	Kind     PrincipalKind
	UserID   string    // human: users.id; AAT: issuer's users.id
	Username string    // human only; AAT leaves empty
	Role     user.Role // for AAT, the issuer-imposed role cap
	Scopes   []string  // human: derived from role; AAT: stored on row
	// ProjectID is empty for human users (their project access is
	// computed via project_members) and for global AATs. A non-empty
	// ProjectID on an AAT means the token is hard-bound to that
	// project — it cannot reach any other.
	ProjectID string
	// TokenID is the opaque token id for AAT principals; empty for
	// humans. Audit code stamps activities.actor_token_id from this.
	TokenID string
}

// IsGlobalAdmin reports whether this principal is allowed to bypass
// project membership checks. The "global admin" bypass is intentionally
// disabled for project-bound AATs: even an admin-role AAT can't reach
// outside its bound project, otherwise the binding would be meaningless.
// Human admins still bypass — their role gate is the ambient one.
func (p *Principal) IsGlobalAdmin() bool {
	if p == nil || p.Role != user.RoleAdmin {
		return false
	}
	if p.Kind == PrincipalAATKind && p.ProjectID != "" {
		return false
	}
	return true
}

// PrincipalFromVerified builds a Principal from a successful optoken
// Verify result. The kind in Verified determines the resulting
// PrincipalKind: AAT rows produce a PrincipalAATKind, user_session
// rows produce a PrincipalUser (sessions are human bearers, just
// authenticated via an opaque token instead of a JWT).
func PrincipalFromVerified(v *optoken.Verified) *Principal {
	p := &Principal{
		UserID:    v.UserID,
		Username:  v.Username,
		Role:      v.Role,
		Scopes:    append([]string(nil), v.Scopes...),
		ProjectID: v.ProjectID,
		TokenID:   v.TokenID,
	}
	switch v.Kind {
	case optoken.KindUserSession:
		p.Kind = PrincipalUser
	default:
		p.Kind = PrincipalAATKind
	}
	return p
}

// principalCtxKey is the gin context slot for the authenticated
// principal. Distinct from claimsCtxKey so handlers that have
// migrated to the new abstraction don't accidentally read a
// half-populated AccessClaims for an AAT-authenticated request.
const principalCtxKey = "platypus.auth.principal"

// SetPrincipal stamps the principal on the gin context. Exported so
// alternate auth middlewares (tests, future SSO) can wire the same
// downstream consumers without going through RequireAuth.
func SetPrincipal(c *gin.Context, p *Principal) {
	c.Set(principalCtxKey, p)
}

// PrincipalFromContext returns the Principal set by RequireAuth (or
// SetPrincipal). Returns ok=false when middleware never ran or when
// the value type doesn't match — handlers that depend on auth must
// treat !ok as a programming error since RequireAuth would have
// aborted earlier on auth failure.
func PrincipalFromContext(c *gin.Context) (*Principal, bool) {
	v, ok := c.Get(principalCtxKey)
	if !ok {
		return nil, false
	}
	p, ok := v.(*Principal)
	return p, ok
}
