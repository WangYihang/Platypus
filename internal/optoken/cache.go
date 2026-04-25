package optoken

import (
	"container/list"
	"fmt"
	"sync"
	"time"

	"github.com/WangYihang/Platypus/internal/user"
)

// Verified is the cache-friendly form of an authenticated token: the
// data the verifier needs both to short-circuit a future request
// (matching the presented secret against Hash) and to reconstruct an
// api.Principal without re-reading storage. Pure data, no methods —
// the api package owns Principal semantics; optoken just stores the
// fields.
type Verified struct {
	TokenID  string
	Kind     Kind
	Hash     []byte // sha256 of the secret half — for cache-hit secret check
	UserID   string
	Username string // empty for AAT
	Role     user.Role
	Scopes   []string
	// ProjectID is empty for global AATs and for user sessions.
	ProjectID string
	// ExpiresAt is the hard expiry; both kinds carry it.
	ExpiresAt time.Time
	// IdleExpiresAt is the sliding-window expiry — populated for user
	// sessions, zero value for AATs.
	IdleExpiresAt time.Time
}

// Cache is a bounded LRU + TTL store keyed by token id. The verifier
// inserts on every successful DB Verify and Invalidates on Revoke. TTL
// is the same for every entry — short enough (default 30s) that a
// missed Invalidate (different process, dropped signal) limits damage
// without making the hot path constantly hit the DB.
//
// All operations are safe under concurrent access. Eviction order is
// strict LRU: a Get on a non-expired entry promotes it to most-recent.
type Cache struct {
	mu    sync.Mutex
	cap   int
	ttl   time.Duration
	now   func() time.Time
	items map[string]*list.Element
	order *list.List // *cacheEntry; Front = most-recently-used
}

type cacheEntry struct {
	id        string
	val       *Verified
	expiresAt time.Time
}

// NewCache returns a cache with the given capacity and per-entry TTL.
// Capacity ≤0 panics — that's a config bug, not a runtime condition.
func NewCache(capacity int, ttl time.Duration) *Cache {
	if capacity <= 0 {
		panic(fmt.Sprintf("optoken.NewCache: capacity must be > 0, got %d", capacity))
	}
	if ttl <= 0 {
		panic(fmt.Sprintf("optoken.NewCache: ttl must be > 0, got %v", ttl))
	}
	return &Cache{
		cap:   capacity,
		ttl:   ttl,
		now:   time.Now,
		items: make(map[string]*list.Element, capacity),
		order: list.New(),
	}
}

// WithClock injects a clock for deterministic TTL tests. Returns the
// receiver so it composes with NewCache. Production code never calls
// this — the default time.Now is what we want.
func (c *Cache) WithClock(now func() time.Time) *Cache {
	c.mu.Lock()
	c.now = now
	c.mu.Unlock()
	return c
}

// Get returns the cached Verified for id if present and not yet
// expired. A hit promotes the entry to most-recently-used. A
// TTL-expired entry is treated as a miss AND removed from the cache so
// Len() reflects only live entries.
func (c *Cache) Get(id string) (*Verified, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[id]
	if !ok {
		return nil, false
	}
	ent := el.Value.(*cacheEntry)
	if !c.now().Before(ent.expiresAt) {
		c.removeElement(el)
		return nil, false
	}
	c.order.MoveToFront(el)
	return ent.val, true
}

// Put inserts or replaces the entry for id with the given Verified.
// TTL is reset on each Put — a re-put behaves like a fresh insert. If
// the cache is at capacity, the least-recently-used entry is evicted.
func (c *Cache) Put(id string, v *Verified) {
	c.mu.Lock()
	defer c.mu.Unlock()
	exp := c.now().Add(c.ttl)
	if el, ok := c.items[id]; ok {
		ent := el.Value.(*cacheEntry)
		ent.val = v
		ent.expiresAt = exp
		c.order.MoveToFront(el)
		return
	}
	ent := &cacheEntry{id: id, val: v, expiresAt: exp}
	el := c.order.PushFront(ent)
	c.items[id] = el
	if c.order.Len() > c.cap {
		c.removeElement(c.order.Back())
	}
}

// Invalidate drops the entry for id. Safe to call on missing keys.
// Synchronous — when this returns, no subsequent Get for id will hit.
func (c *Cache) Invalidate(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[id]; ok {
		c.removeElement(el)
	}
}

// InvalidateAll drops every entry. Used in tests and on signing-key
// rotation (every cached principal must be re-verified against the new
// state).
func (c *Cache) InvalidateAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[string]*list.Element, c.cap)
	c.order.Init()
}

// Len returns the number of live entries. Only meaningful for
// metrics / tests; not part of the verifier hot path.
func (c *Cache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.order.Len()
}

// removeElement assumes the caller holds c.mu.
func (c *Cache) removeElement(el *list.Element) {
	ent := el.Value.(*cacheEntry)
	c.order.Remove(el)
	delete(c.items, ent.id)
}
