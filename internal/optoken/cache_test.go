package optoken_test

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/WangYihang/Platypus/internal/optoken"
	"github.com/WangYihang/Platypus/internal/user"
)

func sample(id string) *optoken.Verified {
	return &optoken.Verified{
		TokenID:   id,
		Kind:      optoken.KindUserSession,
		Hash:      []byte("hash-of-" + id),
		UserID:    "u-" + id,
		Username:  "user-" + id,
		Role:      user.RoleOperator,
		Scopes:    []string{optoken.ScopeHostsRead},
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}
}

func TestCache_GetMiss(t *testing.T) {
	t.Parallel()
	c := optoken.NewCache(8, time.Minute)
	if v, ok := c.Get("missing"); ok || v != nil {
		t.Errorf("Get on missing key = (%v, %v), want (nil, false)", v, ok)
	}
}

func TestCache_PutGet(t *testing.T) {
	t.Parallel()
	c := optoken.NewCache(8, time.Minute)
	v := sample("tk_abc")
	c.Put("tk_abc", v)

	got, ok := c.Get("tk_abc")
	if !ok {
		t.Fatal("Get after Put = !ok")
	}
	if got != v {
		t.Errorf("Get returned different pointer; got %p want %p", got, v)
	}
}

func TestCache_TTLExpiry(t *testing.T) {
	t.Parallel()
	now := time.Unix(1_700_000_000, 0)
	clock := &fakeClock{now: now}
	c := optoken.NewCache(8, 30*time.Second).WithClock(clock.Now)

	c.Put("tk_x", sample("tk_x"))

	// Just before TTL — still cached.
	clock.advance(29 * time.Second)
	if _, ok := c.Get("tk_x"); !ok {
		t.Error("Get at t+29s = miss, want hit")
	}

	// Advance past TTL — entry should be reported as miss.
	clock.advance(2 * time.Second)
	if v, ok := c.Get("tk_x"); ok {
		t.Errorf("Get at t+31s = (%v, true), want miss", v)
	}
	// Expired entry must also drop from Len.
	if c.Len() != 0 {
		t.Errorf("Len after expired Get = %d, want 0", c.Len())
	}
}

func TestCache_PutResetsTTL(t *testing.T) {
	t.Parallel()
	now := time.Unix(1_700_000_000, 0)
	clock := &fakeClock{now: now}
	c := optoken.NewCache(8, 30*time.Second).WithClock(clock.Now)

	c.Put("tk_x", sample("tk_x"))
	clock.advance(29 * time.Second)
	c.Put("tk_x", sample("tk_x")) // re-Put with same id resets TTL
	clock.advance(29 * time.Second) // 58s since first Put, but only 29s since second
	if _, ok := c.Get("tk_x"); !ok {
		t.Error("Get after re-Put + 29s = miss, want hit (TTL should reset)")
	}
}

func TestCache_Invalidate(t *testing.T) {
	t.Parallel()
	c := optoken.NewCache(8, time.Minute)
	c.Put("tk_a", sample("tk_a"))
	c.Put("tk_b", sample("tk_b"))

	c.Invalidate("tk_a")
	if _, ok := c.Get("tk_a"); ok {
		t.Error("Get after Invalidate = hit")
	}
	if _, ok := c.Get("tk_b"); !ok {
		t.Error("Invalidate(tk_a) removed tk_b too")
	}
	// Invalidating a missing key is a no-op, not an error.
	c.Invalidate("never_existed")
}

func TestCache_InvalidateAll(t *testing.T) {
	t.Parallel()
	c := optoken.NewCache(8, time.Minute)
	c.Put("a", sample("a"))
	c.Put("b", sample("b"))
	c.Put("c", sample("c"))

	c.InvalidateAll()
	if c.Len() != 0 {
		t.Errorf("Len after InvalidateAll = %d, want 0", c.Len())
	}
	if _, ok := c.Get("a"); ok {
		t.Error("Get after InvalidateAll = hit")
	}
}

func TestCache_LRUEviction(t *testing.T) {
	t.Parallel()
	c := optoken.NewCache(3, time.Minute)
	c.Put("a", sample("a"))
	c.Put("b", sample("b"))
	c.Put("c", sample("c"))
	c.Put("d", sample("d")) // capacity exceeded — least-recently-used "a" evicted

	if _, ok := c.Get("a"); ok {
		t.Error("Get(a) = hit, want evicted")
	}
	for _, k := range []string{"b", "c", "d"} {
		if _, ok := c.Get(k); !ok {
			t.Errorf("Get(%s) = miss, want hit", k)
		}
	}
	if c.Len() != 3 {
		t.Errorf("Len = %d, want 3", c.Len())
	}
}

func TestCache_LRUTouchOnGet(t *testing.T) {
	t.Parallel()
	c := optoken.NewCache(3, time.Minute)
	c.Put("a", sample("a"))
	c.Put("b", sample("b"))
	c.Put("c", sample("c"))

	// Touch "a" — it's now MRU, "b" is LRU.
	if _, ok := c.Get("a"); !ok {
		t.Fatal("Get(a) = miss before touch")
	}
	c.Put("d", sample("d")) // should evict "b", not "a"

	if _, ok := c.Get("b"); ok {
		t.Error("Get(b) = hit, want evicted (b was LRU after a was touched)")
	}
	if _, ok := c.Get("a"); !ok {
		t.Error("Get(a) = miss after touch + new Put")
	}
}

func TestCache_Concurrent(t *testing.T) {
	t.Parallel()
	c := optoken.NewCache(64, time.Minute)
	var wg sync.WaitGroup
	var hits, misses int64

	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				key := "k" + string(rune('A'+(w+i)%16))
				if i%3 == 0 {
					c.Put(key, sample(key))
				}
				if _, ok := c.Get(key); ok {
					atomic.AddInt64(&hits, 1)
				} else {
					atomic.AddInt64(&misses, 1)
				}
				if i%50 == 0 {
					c.Invalidate(key)
				}
			}
		}(w)
	}
	wg.Wait()
	// We don't assert specific hit/miss counts — just that the cache
	// survived 8 goroutines hammering it without panic / data race.
	// This test must be run with -race in CI to be meaningful.
}

func TestCache_PutOverNothingPanics(t *testing.T) {
	// Negative-cap construction is a programming bug; surface it loudly.
	defer func() {
		if r := recover(); r == nil {
			t.Error("NewCache(0, ...) did not panic, want panic")
		}
	}()
	_ = optoken.NewCache(0, time.Minute)
}

// fakeClock is a thread-safe injectable clock for TTL tests.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (f *fakeClock) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

func (f *fakeClock) advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = f.now.Add(d)
}
