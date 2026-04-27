package api

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/optoken"
	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

// M1 part 1 — rate limit. After loginRateMaxFailures wrong password
// attempts in 60s, the next attempt from the same (ip, username)
// must return 429, regardless of whether that next attempt would
// have been correct. The bcrypt path is skipped before that, so
// flooders can't drive load even when every response is a denial.
func TestLogin_RateLimitedAfterFailures(t *testing.T) {
	r, _ := authTestSetup(t)
	jsonReq(t, r, "POST", "/api/v1/auth/bootstrap", map[string]string{
		"secret": bootstrapSecret, "username": "alice", "password": "hunter2",
	})

	for i := 0; i < loginRateMaxFailures; i++ {
		w := jsonReq(t, r, "POST", "/api/v1/auth/login", map[string]string{
			"username": "alice", "password": "wrong",
		})
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: status=%d; want 401", i+1, w.Code)
		}
	}
	w := jsonReq(t, r, "POST", "/api/v1/auth/login", map[string]string{
		"username": "alice", "password": "hunter2", // even the right password
	})
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("over-budget attempt with correct password: status=%d body=%s; want 429",
			w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "too many login attempts") {
		t.Errorf("429 body should hint at the throttle: %s", w.Body.String())
	}
}

// M1 part 1 (cont.) — different usernames on the same IP have their
// own budgets. A user mistyping their own password shouldn't be
// locked out by an unrelated brute-force burst on a different
// account from the same NAT'd source.
func TestLogin_RateLimitIsolatesUsernames(t *testing.T) {
	r, _ := authTestSetup(t)
	jsonReq(t, r, "POST", "/api/v1/auth/bootstrap", map[string]string{
		"secret": bootstrapSecret, "username": "alice", "password": "hunter2",
	})

	for i := 0; i < loginRateMaxFailures; i++ {
		_ = jsonReq(t, r, "POST", "/api/v1/auth/login", map[string]string{
			"username": "bob", "password": "wrong",
		})
	}
	// alice still has her full budget.
	w := jsonReq(t, r, "POST", "/api/v1/auth/login", map[string]string{
		"username": "alice", "password": "hunter2",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("alice locked out by bob's failures: status=%d body=%s",
			w.Code, w.Body.String())
	}
}

// M1 part 2 — timing flatten. The unknown-user path must run a
// real-shape bcrypt compare against the dummy hash so its CPU work
// is in the same bucket as the bad-password path. We can't easily
// pin "<5ms vs >250ms" without flake, so we assert the unknown-user
// path takes at least 30ms — well above the no-bcrypt baseline (DB
// lookup + JSON parse < 5ms) and well below the cost-12 bcrypt time
// (~250ms) so a slow CI box can't flake it the wrong way.
//
// authTestSetup defaults bcrypt to MinCost (4) so the rest of the
// auth tests fit inside the per-package timeout under -race. For
// THIS test specifically we need the dummy bcrypt at production
// cost 12 — the 30ms threshold means nothing if bcrypt itself takes
// 1-3ms — and the dummy is computed AT HANDLER-CONSTRUCTION TIME,
// so bumping cost after authTestSetup wouldn't help: the cached
// dummy hash is already cost-4. Build the handler from scratch
// here with cost 12 set first.
func TestLogin_UnknownUser_TimingFlat(t *testing.T) {
	user.SetPasswordHashCostForTest(t, 12)

	gin.SetMode(gin.TestMode)
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	cache := optoken.NewCache(64, 30*time.Second)
	verifier := NewTokenVerifier(db, cache)
	h := NewAuthHandler(db, verifier, bootstrapSecret)
	r := gin.New()
	r.POST("/api/v1/auth/login", h.Login)

	// No bootstrap — users table is empty; every username is unknown.
	start := time.Now()
	w := jsonReq(t, r, "POST", "/api/v1/auth/login", map[string]string{
		"username": "nobody", "password": "x",
	})
	elapsed := time.Since(start)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d; want 401", w.Code)
	}
	if elapsed < 30*time.Millisecond {
		t.Errorf(
			"unknown-user login returned in %v; that's a username-existence "+
				"timing oracle (the dummy bcrypt isn't running)",
			elapsed,
		)
	}
}
