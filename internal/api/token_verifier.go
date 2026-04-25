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

	v.cache.Put(id, verified)

	// last_used_* update runs detached so it never blocks the
	// request hot path. context.WithoutCancel preserves any
	// values (request id, etc.) but lets the goroutine outlive
	// the request without picking up its cancellation.
	go func(id, ip, ua string, t time.Time) {
		bg := context.WithoutCancel(ctx)
		ctx2, cancel := context.WithTimeout(bg, 2*time.Second)
		defer cancel()
		// Errors are best-effort; a missed touch is a stale
		// last_used, not a security issue.
		_ = v.db.AuthTokens().TouchLastUsed(ctx2, id, ip, ua, nil, t)
	}(id, "", "", now)

	return PrincipalFromVerified(verified), "success", nil
}

// Invalidate drops the cache entry for tokenID. Called by the AAT
// revoke / rotate handlers after the storage write commits, so the
// next request observes the change immediately on this node. (Cross-
// node invalidation is out of scope until Platypus runs multi-node.)
func (v *TokenVerifier) Invalidate(tokenID string) {
	v.cache.Invalidate(tokenID)
}
