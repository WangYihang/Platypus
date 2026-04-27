package api

import (
	"testing"
	"time"
)

// loginThrottle's Allow / Record contract is the authoritative
// behaviour for /api/v1/auth/login's rate-limit gate. These tests
// pin the spec at the helper level so the (slower) integration test
// in handler_auth_v1_login_throttle_test.go can stay focused on the
// HTTP path.

func TestLoginThrottle_AllowsWithinBudget(t *testing.T) {
	lt := newLoginThrottle()
	for i := 0; i < loginRateMaxFailures; i++ {
		if !lt.Allow("1.2.3.4", "alice") {
			t.Fatalf("Allow returned false on attempt %d before any failures recorded", i)
		}
		lt.Record("1.2.3.4", "alice", false)
	}
	// Sixth attempt: budget exhausted, must be refused.
	if lt.Allow("1.2.3.4", "alice") {
		t.Fatalf("Allow returned true after %d recorded failures; expected throttle to fire",
			loginRateMaxFailures)
	}
}

func TestLoginThrottle_DifferentKeysIndependent(t *testing.T) {
	lt := newLoginThrottle()
	for i := 0; i < loginRateMaxFailures; i++ {
		_ = lt.Allow("1.2.3.4", "alice")
		lt.Record("1.2.3.4", "alice", false)
	}
	// Same IP, different user → fresh budget.
	if !lt.Allow("1.2.3.4", "bob") {
		t.Errorf("alice's failures locked out bob on the same IP")
	}
	// Same user, different IP → fresh budget.
	if !lt.Allow("9.9.9.9", "alice") {
		t.Errorf("alice's failures from one IP locked her out everywhere")
	}
}

func TestLoginThrottle_SuccessClearsBudget(t *testing.T) {
	lt := newLoginThrottle()
	for i := 0; i < loginRateMaxFailures; i++ {
		_ = lt.Allow("1.2.3.4", "alice")
		lt.Record("1.2.3.4", "alice", false)
	}
	if lt.Allow("1.2.3.4", "alice") {
		t.Fatal("expected to be throttled after 5 failures")
	}
	// A successful auth (e.g. user finally typed the right password
	// from a different session) clears the budget so they aren't
	// punished by the unrelated wave of guesses.
	lt.Record("1.2.3.4", "alice", true)
	if !lt.Allow("1.2.3.4", "alice") {
		t.Error("Record(success) didn't clear the rate-limit budget")
	}
}

func TestLoginThrottle_WindowSlides(t *testing.T) {
	// Drive `now` manually so we can fast-forward past the window
	// without sleeping.
	t0 := time.Date(2026, 4, 27, 0, 0, 0, 0, time.UTC)
	lt := newLoginThrottle()
	now := t0
	lt.now = func() time.Time { return now }

	for i := 0; i < loginRateMaxFailures; i++ {
		_ = lt.Allow("1.2.3.4", "alice")
		lt.Record("1.2.3.4", "alice", false)
	}
	if lt.Allow("1.2.3.4", "alice") {
		t.Fatal("expected throttled at budget")
	}

	// Advance past the window; old failures should age out.
	now = t0.Add(loginRateWindow + time.Second)
	if !lt.Allow("1.2.3.4", "alice") {
		t.Error("Allow still false after window slid past all recorded failures")
	}
}
