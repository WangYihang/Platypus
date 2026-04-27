package api

import (
	"net/http"
	"strings"
	"testing"
	"time"
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
func TestLogin_UnknownUser_TimingFlat(t *testing.T) {
	r, _ := authTestSetup(t)
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
