package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/user"
)

// rpcThrottle's Allow contract is the authoritative behaviour for
// L3's per-principal rate limit. These tests pin the spec at the
// helper level; the integration test below covers the gin
// middleware path.

func TestRPCThrottle_BurstThenRefill(t *testing.T) {
	t0 := time.Date(2026, 4, 27, 0, 0, 0, 0, time.UTC)
	rt := newRPCThrottle()
	rt.cfg = rpcRateConfig{burst: 5, rate: 100 * time.Millisecond, maxKey: 4096}
	now := t0
	rt.now = func() time.Time { return now }

	for i := 0; i < 5; i++ {
		if !rt.Allow("k") {
			t.Fatalf("Allow #%d returned false; expected to drain the burst budget first", i+1)
		}
	}
	if rt.Allow("k") {
		t.Fatal("Allow returned true on attempt 6 — burst should be exhausted")
	}

	// Refill: advance one rate-tick. We get exactly one token back,
	// allowing one more call before being refused again.
	now = now.Add(rt.cfg.rate)
	if !rt.Allow("k") {
		t.Fatal("after one tick, refill didn't give us a token")
	}
	if rt.Allow("k") {
		t.Fatal("two calls per tick — burst budget should still be empty")
	}
}

func TestRPCThrottle_KeysIndependent(t *testing.T) {
	rt := newRPCThrottle()
	rt.cfg = rpcRateConfig{burst: 3, rate: time.Second, maxKey: 4096}
	for i := 0; i < 3; i++ {
		_ = rt.Allow("alice")
	}
	if rt.Allow("alice") {
		t.Fatal("alice over budget")
	}
	if !rt.Allow("bob") {
		t.Error("alice's exhausted budget locked bob out")
	}
}

func TestPrincipalRateLimitKey_AATPreferred(t *testing.T) {
	p := &Principal{
		Kind:    PrincipalAATKind,
		UserID:  "u-issuer",
		TokenID: "aat_xyz",
	}
	got := principalRateLimitKey(p)
	if got != "aat:aat_xyz" {
		t.Errorf("principalRateLimitKey AAT = %q; want aat:aat_xyz", got)
	}
}

func TestPrincipalRateLimitKey_HumanByUserID(t *testing.T) {
	p := &Principal{
		Kind:   PrincipalUser,
		UserID: "u-1",
	}
	got := principalRateLimitKey(p)
	if got != "user:u-1" {
		t.Errorf("principalRateLimitKey user = %q; want user:u-1", got)
	}
}

// Integration: the gin middleware bucket-checks per Principal and
// returns 429 once the budget is exhausted, but a different
// principal still gets through.
func TestRequireRPCRateLimit_GinMiddleware(t *testing.T) {
	rb, db := rbacTestSetup(t)
	throttle := newRPCThrottle()
	throttle.cfg = rpcRateConfig{burst: 2, rate: time.Hour, maxKey: 4096} // never refills in test

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/probe",
		rb.RequireAuth(),
		rb.requireRPCRateLimit(throttle),
		func(c *gin.Context) { c.String(http.StatusOK, "ok") },
	)

	tokAlice := mintBearerForUserID(t, db, "u-alice", user.RoleOperator)
	tokBob := mintBearerForUserID(t, db, "u-bob", user.RoleOperator)

	hit := func(tok string) int {
		req := httptest.NewRequest("GET", "/probe", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w.Code
	}

	if got := hit(tokAlice); got != 200 {
		t.Fatalf("alice 1st call: %d; want 200", got)
	}
	if got := hit(tokAlice); got != 200 {
		t.Fatalf("alice 2nd call: %d; want 200", got)
	}
	if got := hit(tokAlice); got != http.StatusTooManyRequests {
		t.Fatalf("alice 3rd call: %d; want 429", got)
	}
	// Bob's bucket is independent.
	if got := hit(tokBob); got != 200 {
		t.Errorf("bob 1st call: %d; want 200 (alice's burst shouldn't affect him)", got)
	}
}
