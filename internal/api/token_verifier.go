package api

import (
	"context"
	"time"

	"github.com/WangYihang/Platypus/internal/optoken"
	"github.com/WangYihang/Platypus/internal/storage"
)

// TokenVerifier turns a raw opaque token off the wire into an
// authenticated *Principal. It owns the cache + DB lookup decision and
// the prefix-to-kind dispatch — RBAC's RequireAuth delegates here once
// it has stripped the Bearer scheme. JWT verification stays in
// TokenIssuer; this verifier is exclusively for opaque kinds.
type TokenVerifier struct {
	db    *storage.DB
	cache *optoken.Cache
	clock func() time.Time
}

// NewTokenVerifier constructs a verifier sharing the storage handle
// with the rest of the API. Cache is required: zero-cache deployments
// would push every authenticated request into a DB read on each call.
func NewTokenVerifier(db *storage.DB, cache *optoken.Cache) *TokenVerifier {
	return &TokenVerifier{db: db, cache: cache, clock: time.Now}
}

// WithClock substitutes the clock used for cache-hit expiry comparison.
// Tests use it; production keeps time.Now.
func (v *TokenVerifier) WithClock(now func() time.Time) *TokenVerifier {
	v.clock = now
	return v
}

// SessionIdleWindow is how far ahead each successful Verify pushes a
// user_session's idle_expires_at. 24h matches the documented session
// behaviour: a user idle longer than this re-authenticates regardless
// of the hard ExpiresAt.
const SessionIdleWindow = 24 * time.Hour

// Verify resolves the raw token and returns the authenticated principal
// alongside a reason string callers log to the audit trail. The reason
// is one of:
//
//	"success"      — *Principal populated
//	"unrecognized" — prefix doesn't match any known kind (incl. JWTs)
//	"malformed"    — prefix matches but body is invalid
//	"unknown" / "invalid_secret" / "expired" / "idle_expired" / "revoked"
//	               — storage.AuthTokens.Verify reasons
//
// On any non-success reason the principal is nil; the caller maps the
// reason to a 401 / 403 with a generic body.
func (v *TokenVerifier) Verify(ctx context.Context, raw string) (*Principal, string, error) {
	kind, prefix, ok := optoken.DetectKind(raw)
	if !ok {
		return nil, "unrecognized", nil
	}
	id, secret, err := optoken.Parse(raw, prefix)
	if err != nil {
		return nil, "malformed", nil
	}
	now := v.clock()

	// Cache hit short-circuits the DB. We still verify the presented
	// secret against the cached hash so a stolen token id (e.g. from
	// logs) doesn't trivially impersonate the holder. We also check
	// for hard expiry / idle expiry against the cached deadlines —
	// the cache TTL is a separate bound that limits the "missed
	// invalidate" damage window, not a substitute for the per-row
	// expiries.
	if cached, hit := v.cache.Get(id); hit {
		if cached.Kind == kind &&
			optoken.Equal(optoken.Hash(secret), cached.Hash) &&
			cached.ExpiresAt.After(now) &&
			(cached.IdleExpiresAt.IsZero() || cached.IdleExpiresAt.After(now)) {
			return PrincipalFromVerified(cached), "success", nil
		}
		// Anything off → drop the entry and fall through to a fresh
		// DB Verify so the reason is authoritative.
		v.cache.Invalidate(id)
	}

	verified, reason, err := v.db.AuthTokens().Verify(ctx, id, secret, kind, now)
	if err != nil {
		return nil, "", err
	}
	if reason != "success" {
		return nil, reason, nil
	}

	// Sessions inherit role/username from the live users row so a
	// demote takes effect within one cache TTL (≤30s). AATs keep
	// their on-row role/scope — issuer-time intent is the source of
	// truth for tokens. A users row that has been deleted out from
	// under a session yields auth failure (treated as revoked).
	if kind == optoken.KindUserSession {
		u, err := v.db.Users().GetByID(ctx, verified.UserID)
		if err != nil {
			return nil, "revoked", nil
		}
		verified.Role = u.Role
		verified.Username = u.Username
		verified.Scopes = optoken.ScopesFromRole(u.Role)
	}

	v.cache.Put(id, verified)

	// last_used_* update runs detached so it never blocks the
	// request hot path. Sessions also need their idle window
	// pushed forward — that's the sliding-window mechanism. AATs
	// pass nil so idle_expires_at stays NULL on their rows.
	var newIdle *time.Time
	if kind == optoken.KindUserSession {
		bumped := now.Add(SessionIdleWindow)
		newIdle = &bumped
	}
	go func(id string, idle *time.Time, t time.Time) {
		bg := context.WithoutCancel(ctx)
		ctx2, cancel := context.WithTimeout(bg, 2*time.Second)
		defer cancel()
		// Errors are best-effort; a missed touch is a stale
		// last_used, not a security issue.
		_ = v.db.AuthTokens().TouchLastUsed(ctx2, id, "", "", idle, t)
	}(id, newIdle, now)

	return PrincipalFromVerified(verified), "success", nil
}

// Invalidate drops the cache entry for tokenID. Called by the AAT
// revoke / rotate handlers after the storage write commits, so the
// next request observes the change immediately on this node. (Cross-
// node invalidation is out of scope until Platypus runs multi-node.)
func (v *TokenVerifier) Invalidate(tokenID string) {
	v.cache.Invalidate(tokenID)
}
