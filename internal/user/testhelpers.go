package user

import "testing"

// SetPasswordHashCostForTest lowers PasswordHashCost for the
// duration of a test (or sub-tree) and restores the previous value
// via t.Cleanup. Intended for callers that exercise multiple
// HashPassword paths per test — running cost 12 (production default)
// across an auth-heavy package multiplies wallclock by 100×+ and
// trips race-detector timeouts. Cost 4 (bcrypt minimum) is plenty
// for unit-test correctness.
//
// Lives in a non-_test.go file so test packages OUTSIDE
// internal/user (e.g. internal/api/*_test.go) can call it across
// package boundaries. The testing.TB parameter signals the
// intended call site without forbidding non-test imports — Go does
// not gate `testing` to test code, but a TB-typed argument is
// unobtainable in non-test contexts in practice.
func SetPasswordHashCostForTest(t testing.TB, cost int) {
	t.Helper()
	prev := PasswordHashCost
	PasswordHashCost = cost
	t.Cleanup(func() { PasswordHashCost = prev })
}
