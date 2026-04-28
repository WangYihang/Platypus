package optoken

import (
	"strings"

	"github.com/WangYihang/Platypus/internal/user"
)

// Kind classifies an opaque token by its issuance purpose. The wire
// format is identical across kinds — only the prefix differs — so this
// is the value that storage, verifier, and audit code branch on.
type Kind string

const (
	KindAAT             Kind = "aat"
	KindUserSession     Kind = "user_session"
	KindEnrollmentToken Kind = "enrollment"
	KindInstall         Kind = "install"
)

// Wire-format prefixes. Visible in logs / git / Slack so secret
// scanners can match each kind the way they do GitHub's `ghp_`. The
// trailing underscore is part of the prefix — included so a substring
// like "aat_" never appears mid-id.
//
// EnrollmentTokenPrefix keeps the historical literal "plt_" so that
// already-deployed agents and operator scripts (which pasted the prefix
// into config files) continue to work unchanged.
const (
	AATPrefix             = "aat_"
	UserSessionPrefix     = "pst_"
	EnrollmentTokenPrefix = "plt_"
	InstallPrefix         = "dl_"
)

// kindByPrefix is the single source of truth for prefix↔kind mapping.
// Adding a new kind = one entry here + one Kind constant + one prefix
// constant; nothing else in this package needs touching.
var kindByPrefix = map[string]Kind{
	AATPrefix:             KindAAT,
	UserSessionPrefix:     KindUserSession,
	EnrollmentTokenPrefix: KindEnrollmentToken,
	InstallPrefix:         KindInstall,
}

// KindForPrefix resolves an exact prefix string (including the trailing
// underscore) to its Kind. Returns ("", false) for unknown prefixes.
func KindForPrefix(prefix string) (Kind, bool) {
	k, ok := kindByPrefix[prefix]
	return k, ok
}

// DetectKind inspects a raw token string and returns the matching kind
// plus its prefix. Used by verifiers to dispatch to the right code path
// before any storage lookup. Returns ok=false for empty strings, JWTs,
// or any token whose prefix isn't registered — caller should treat
// those as 401 unrecognized.
func DetectKind(raw string) (kind Kind, prefix string, ok bool) {
	for p, k := range kindByPrefix {
		if strings.HasPrefix(raw, p) {
			return k, p, true
		}
	}
	return "", "", false
}

// Scope strings follow "<resource>:<verb>". Lowercase, no spaces — the
// storage column is space-delimited, so a scope containing whitespace
// would silently corrupt the row.
const (
	ScopeHostsRead    = "hosts:read"
	ScopeHostsExec    = "hosts:exec"
	ScopeFilesRead    = "files:read"
	ScopeFilesWrite   = "files:write"
	ScopeRPCInvoke    = "rpc:invoke"
	ScopeProjectsRead = "projects:read"
	ScopeActivityRead = "activity:read"
)

// allScopes is the canonical full set, ordered to match the role
// hierarchy: read scopes first (viewer), then write/exec (operator).
// AllScopes returns a defensive copy so callers can't mutate it.
var allScopes = []string{
	ScopeHostsRead,
	ScopeFilesRead,
	ScopeProjectsRead,
	ScopeActivityRead,
	ScopeHostsExec,
	ScopeFilesWrite,
	ScopeRPCInvoke,
}

// AllScopes returns every scope known to the server. Useful for admin
// AAT issuance and for the "ScopesFromRole(admin)" implementation.
func AllScopes() []string {
	out := make([]string, len(allScopes))
	copy(out, allScopes)
	return out
}

// readScopes are the strict-read subset granted to viewers and any
// stricter principal that needs only to observe state.
var readScopes = []string{
	ScopeHostsRead,
	ScopeFilesRead,
	ScopeProjectsRead,
	ScopeActivityRead,
}

// ScopesFromRole returns the scope set a human user implicitly holds
// based on their global role. Used so RequireScope on a route is a
// no-op for humans (they're already gated by RequireGlobalRole /
// RequireProjectRole) while still functioning for AATs, which carry
// an explicit scope set on the row.
//
// Unknown / empty roles return nil, which makes RequireScope deny
// every check — desirable for a misconfigured user record.
func ScopesFromRole(r user.Role) []string {
	switch r {
	case user.RoleAdmin, user.RoleOperator:
		return AllScopes()
	case user.RoleViewer:
		out := make([]string, len(readScopes))
		copy(out, readScopes)
		return out
	default:
		return nil
	}
}

// HasScope reports whether granted contains want. Case-sensitive — the
// canonical form is lowercase and we never want a typo'd "Hosts:Read"
// to silently accept an "hosts:read" check.
func HasScope(granted []string, want string) bool {
	if want == "" {
		return false
	}
	for _, s := range granted {
		if s == want {
			return true
		}
	}
	return false
}

// ParseList splits a stored scope string into individual scopes,
// tolerant of any whitespace (space, tab, newline) and collapsing
// runs. Returns nil for an empty / whitespace-only input so callers
// can range over the result without a length check.
func ParseList(s string) []string {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return nil
	}
	return fields
}

// FormatList joins scopes with single spaces — the on-disk
// representation. Empty input yields the empty string so storage rows
// stay clean.
func FormatList(scopes []string) string {
	return strings.Join(scopes, " ")
}
