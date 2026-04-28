package api

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// L3: a token-bucket rate limiter for the high-impact agent RPC
// surface (/api/v1/projects/:pid/agents/:agent_id/exec, /fs/*).
// Without it, a compromised session or future scoped token can drive an unlimited
// rate of shell commands / file writes against every host in the
// project — DoS by design. The default bucket (30 burst, 10/s
// refill) lets normal interactive usage and small loops through
// while capping sustained churn.
//
// Keying is per-principal so a noisy CI script on its own token can't
// starve a human admin sharing the project. Per-(principal, agent)
// would be tighter but creates a 2D map; one-dimensional keying
// matches the practical attack model (a single compromised
// credential).

type rpcRateConfig struct {
	burst  int           // max tokens that can accumulate
	rate   time.Duration // duration per added token (10/s ⇒ 100ms)
	maxKey int           // memory cap; LRU-by-update beyond this
}

var defaultRPCRateConfig = rpcRateConfig{
	burst:  30,
	rate:   100 * time.Millisecond,
	maxKey: 4096,
}

type rpcThrottle struct {
	mu   sync.Mutex
	cfg  rpcRateConfig
	now  func() time.Time
	keys map[string]*rpcBucket
}

type rpcBucket struct {
	tokens    float64
	lastFill  time.Time
	updatedAt time.Time
}

func newRPCThrottle() *rpcThrottle {
	return &rpcThrottle{
		cfg:  defaultRPCRateConfig,
		now:  time.Now,
		keys: make(map[string]*rpcBucket),
	}
}

// Allow consumes one token from the bucket for `key`, refilling
// based on wall-clock since the last call. Returns false when the
// bucket is empty — the caller should respond with 429 and not
// invoke the underlying RPC.
func (rt *rpcThrottle) Allow(key string) bool {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	now := rt.now()
	b, ok := rt.keys[key]
	if !ok {
		// Cap memory: best-effort LRU-by-update eviction.
		if len(rt.keys) >= rt.cfg.maxKey {
			var oldestK string
			var oldestT time.Time
			first := true
			for k, v := range rt.keys {
				if first || v.updatedAt.Before(oldestT) {
					oldestK, oldestT = k, v.updatedAt
					first = false
				}
			}
			delete(rt.keys, oldestK)
		}
		b = &rpcBucket{tokens: float64(rt.cfg.burst), lastFill: now}
		rt.keys[key] = b
	}
	// Refill since lastFill.
	elapsed := now.Sub(b.lastFill)
	if elapsed > 0 && rt.cfg.rate > 0 {
		add := float64(elapsed) / float64(rt.cfg.rate)
		b.tokens += add
		if b.tokens > float64(rt.cfg.burst) {
			b.tokens = float64(rt.cfg.burst)
		}
		b.lastFill = now
	}
	b.updatedAt = now

	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// principalRateLimitKey returns a stable string identifier for the
// authenticated subject of the current request. Human principals are
// keyed by UserID; future scoped-token principals will key on TokenID
// so a stolen token can't share budget with its issuer's session.
func principalRateLimitKey(p *Principal) string {
	if p == nil {
		return ""
	}
	if p.UserID != "" {
		return "user:" + p.UserID
	}
	return ""
}

// requireRPCRateLimit returns a gin middleware that token-buckets
// per Principal. Mount AFTER RequireAuth so the middleware can read
// the principal off the context. Handlers that fall outside the
// budget receive 429 with a "slow down" body and an audit trail in
// the activities log (caller's responsibility to record;
// the gin context already carries enough state for the recorder).
func (r *RBAC) requireRPCRateLimit(throttle *rpcThrottle) gin.HandlerFunc {
	return func(c *gin.Context) {
		p, ok := PrincipalFromContext(c)
		if !ok {
			abortUnauthorized(c, "no principal on context — RequireAuth missing?")
			return
		}
		key := principalRateLimitKey(p)
		if key == "" {
			// Defense-in-depth: a principal with neither TokenID nor
			// UserID isn't a valid auth state, but we still gate
			// rather than letting it pass.
			abortUnauthorized(c, "principal missing identity for rate-limit keying")
			return
		}
		if !throttle.Allow(key) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": "rpc rate limit exceeded — slow down",
			})
			return
		}
		c.Next()
	}
}

// RPCThrottle is the shared throttle instance used by the routes
// that mount this middleware. Lives on RBAC so handlers and tests
// don't construct fresh ones per call (which would leak state /
// be useless).
func (r *RBAC) RPCThrottle() *rpcThrottle {
	if r.rpcThrottle == nil {
		r.rpcThrottle = newRPCThrottle()
	}
	return r.rpcThrottle
}

// RequireRPCRateLimit is the public entrypoint: a single middleware
// that token-buckets by principal key. Use it on the operator group
// of agent RPC routes (exec / file mutations) so a future scoped token can't drive
// unbounded churn against the host it's bound to.
func (r *RBAC) RequireRPCRateLimit() gin.HandlerFunc {
	return r.requireRPCRateLimit(r.RPCThrottle())
}
