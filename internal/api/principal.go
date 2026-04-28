package api

import (
	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/optoken"
	"github.com/WangYihang/Platypus/internal/user"
)

// PrincipalKind tags the credential the request was authenticated with.
// User-kind principals are humans (browser session today). PrincipalAATKind
// is reserved for any non-session opaque token that lands in this slot —
// today it's unreachable because no scoped-token surface is wired up,
// and the constant is preserved so a follow-up rename to PrincipalPATKind
// stays a single-token edit.
type PrincipalKind int

const (
	PrincipalUser PrincipalKind = iota
	PrincipalAATKind
)

// Principal is the authenticated subject of a request. RBAC and audit
// downstream of RequireAuth read it from the gin context — they don't
// care which credential was presented, only what the principal is
// allowed to do.
type Principal struct {
	Kind     PrincipalKind
	UserID   string    // users.id of the human (or, for scoped tokens, the issuer)
	Username string    // populated for human sessions; empty for non-session kinds
	Role     user.Role // for scoped tokens, the issuer-imposed role cap
	Scopes   []string  // human: derived from role; scoped tokens: stored on row
	// ProjectID is empty for human users (their project access is
	// computed via project_members) and for any future global
	// scoped-token. A non-empty value would mean the token is
	// hard-bound to that project — it cannot reach any other.
	ProjectID string
	// TokenID is the opaque token id; empty for humans authenticated
	// with no opaque carrier. Audit code stamps activities.actor_token_id
	// from this.
	TokenID string
}

// IsGlobalAdmin reports whether this principal is allowed to bypass
// project membership checks. Today only humans hold global admin —
// scoped opaque tokens that bind to a project will reintroduce a
// project-binding caveat here when they land.
func (p *Principal) IsGlobalAdmin() bool {
	if p == nil || p.Role != user.RoleAdmin {
		return false
	}
	return true
}

// PrincipalFromVerified builds a Principal from a successful optoken
// Verify result. user_session rows produce a PrincipalUser; any other
// kind defaults to PrincipalAATKind so a future scoped-token wiring
// flows through the same codepath.
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
// principal. Distinct from claimsCtxKey so handlers that haven't been
// migrated off AccessClaims still see consistent values.
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
