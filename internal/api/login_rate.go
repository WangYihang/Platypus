package api

import (
	"sync"
	"time"
)

// loginRateWindow is how far back the throttle counts failures. A
// rolling-window approach is simpler than a token bucket here: we
// already need the per-key failure log for audit, and "5 failures in
// the last minute" matches what we'd write in incident docs.
const (
	loginRateWindow      = 60 * time.Second
	loginRateMaxFailures = 5
)

// loginThrottle is an in-memory per-(ip, username) rolling-window
// counter for /api/v1/auth/login. Built from scratch (no
// golang.org/x/time/rate dependency) because:
//
//   - Token-bucket semantics don't quite match what we want — we care
//     about *failure* counts, not all requests, so a successful login
//     shouldn't drain the bucket.
//   - The bucket needs to clear instantly on success (a real user
//     finally typing the right password) without that drain.
//
// Concurrent access is gated by a single mutex. The map can grow if
// an attacker fans across many usernames; we cap it at maxKeys and
// evict the oldest entry when it exceeds (best-effort, the cap is a
// memory bound, not a security guarantee).
type loginThrottle struct {
	mu      sync.Mutex
	now     func() time.Time
	window  time.Duration
	max     int
	maxKeys int
	state   map[loginRateKey]*loginRateEntry
}

type loginRateKey struct {
	ip       string
	username string
}

type loginRateEntry struct {
	failures []time.Time
	updated  time.Time
}

// newLoginThrottle wires defaults. now is overridable for tests so
// the window can be advanced without sleeping.
func newLoginThrottle() *loginThrottle {
	return &loginThrottle{
		now:     time.Now,
		window:  loginRateWindow,
		max:     loginRateMaxFailures,
		maxKeys: 4096,
		state:   make(map[loginRateKey]*loginRateEntry),
	}
}

// Allow reports whether a fresh login attempt for (ip, username) may
// proceed. Returns false if the caller has already exceeded
// max failures inside the window — the handler should respond with
// 429 immediately, without invoking bcrypt.
func (lt *loginThrottle) Allow(ip, username string) bool {
	lt.mu.Lock()
	defer lt.mu.Unlock()

	key := loginRateKey{ip: ip, username: username}
	e, ok := lt.state[key]
	if !ok {
		return true
	}
	cutoff := lt.now().Add(-lt.window)
	live := e.failures[:0]
	for _, t := range e.failures {
		if t.After(cutoff) {
			live = append(live, t)
		}
	}
	e.failures = live
	return len(live) < lt.max
}

// Record updates the throttle with the outcome of a login attempt.
// On success, the (ip, username) entry is cleared so a real user
// regaining access immediately has a fresh budget. On failure, the
// timestamp is appended; the next Allow check will weigh it against
// the rolling window.
func (lt *loginThrottle) Record(ip, username string, success bool) {
	lt.mu.Lock()
	defer lt.mu.Unlock()

	key := loginRateKey{ip: ip, username: username}
	if success {
		delete(lt.state, key)
		return
	}

	e, ok := lt.state[key]
	if !ok {
		e = &loginRateEntry{}
		// Best-effort cap: if we're at the limit, evict the oldest
		// entry by wall-clock update time. Linear scan keeps the
		// implementation tiny; at maxKeys=4096 this is still well
		// under a millisecond.
		if len(lt.state) >= lt.maxKeys {
			var oldestK loginRateKey
			var oldestT time.Time
			first := true
			for k, v := range lt.state {
				if first || v.updated.Before(oldestT) {
					oldestK = k
					oldestT = v.updated
					first = false
				}
			}
			delete(lt.state, oldestK)
		}
		lt.state[key] = e
	}
	now := lt.now()
	cutoff := now.Add(-lt.window)
	live := e.failures[:0]
	for _, t := range e.failures {
		if t.After(cutoff) {
			live = append(live, t)
		}
	}
	e.failures = append(live, now)
	e.updated = now
}
