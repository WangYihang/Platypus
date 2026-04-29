package settings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/WangYihang/Platypus/internal/activity"
	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/utils/config"
)

// Registry is the live source of truth for policy settings. Every
// consumer (TokenIssuer, Distributor, mesh discovery loop) holds a
// *Registry and calls an accessor on each operation. Calls that hit
// the cache are sub-microsecond; uncached reads do one DB round-trip.
//
// Concurrency-safe. Zero value is not usable — always construct via New.
type Registry struct {
	db  *storage.DB
	cfg *config.Options

	// cache holds parsed, typed values keyed by setting key. A nil
	// entry means "we resolved this key to the YAML/default fallback;
	// no DB row exists yet". Distinguishing nil from absent matters
	// so we don't retry the DB lookup on every read when nothing is
	// overridden.
	cache sync.Map // string → *cacheEntry
}

type cacheEntry struct {
	// value is the typed resolved value (time.Duration, bool, string,
	// int). Using `any` keeps the cache generic; accessors type-assert.
	value any
}

// New constructs a Registry backed by db (for DB-level overrides) and
// cfg (for YAML fallbacks). cfg may be nil, in which case the YAML
// layer is skipped and only DB and hardcoded defaults apply.
func New(db *storage.DB, cfg *config.Options) *Registry {
	return &Registry{db: db, cfg: cfg}
}

// ------------------- typed accessors -------------------
//
// Each accessor does: cache → DB → YAML → hardcoded default. On a
// cache miss, a cacheEntry is stored so subsequent reads stay hot.

// AccessTokenTTL is retained for backward compat with the admin
// settings UI. Phase-2 auth uses opaque session tokens whose
// lifetimes are server-side constants (SessionIdleWindow /
// SessionHardTTL) — this getter now just surfaces whatever value
// is in the DB / YAML for display, with no production effect.
func (r *Registry) AccessTokenTTL() time.Duration {
	return r.getDuration(KeyAuthAccessTokenTTL, func() time.Duration {
		return DefaultAccessTokenTTL
	})
}

// RefreshTokenTTL is retained for backward compat with the admin
// settings UI. The session model has no refresh — see AccessTokenTTL.
func (r *Registry) RefreshTokenTTL() time.Duration {
	return r.getDuration(KeyAuthRefreshTokenTTL, func() time.Duration {
		return DefaultRefreshTokenTTL
	})
}

// PATDefaultTTL is the TTL embedded in a newly minted PAT when the
// admin didn't supply one explicitly at install-artifact mint time.
// Enrollment consults this via the SettingsProvider interface.
func (r *Registry) PATDefaultTTL() time.Duration {
	return r.getDuration(KeyAuthPATDefaultTTL, func() time.Duration {
		return DefaultPATDefaultTTL
	})
}

// DistributorChannel is the default release channel shipped in the
// install script. Bootstrap-config side was removed; defaults live in
// code, admin overrides flow through the DB.
func (r *Registry) DistributorChannel() string {
	return r.getString(KeyDistributorChannel, func() string {
		return DefaultDistributorChannel
	})
}

// DistributorPresignedTTL is how long S3 presigned download URLs
// remain valid. Bootstrap-config side was removed; defaults live in
// code, admin overrides flow through the DB.
func (r *Registry) DistributorPresignedTTL() time.Duration {
	return r.getDuration(KeyDistributorPresignedTTL, func() time.Duration {
		return DefaultPresignedTTL
	})
}

// MeshDiscoveryLAN controls whether the mesh mDNS broadcaster runs.
// Defaults to on; admin can disable via DB override.
func (r *Registry) MeshDiscoveryLAN() bool {
	return r.getBool(KeyMeshDiscoveryLAN, func() bool {
		return DefaultMeshDiscoveryLAN
	})
}

// MeshDiscoveryInterval is the period between mDNS discovery cycles.
// Default 30s; admin override via DB.
func (r *Registry) MeshDiscoveryInterval() time.Duration {
	return r.getDuration(KeyMeshDiscoveryIntervalSec, func() time.Duration {
		return time.Duration(DefaultMeshDiscoveryIntSecs) * time.Second
	})
}

// AuditRetentionDays is the number of days the audit log keeps
// entries. Zero = retain forever. Stored as a plain JSON integer
// (not seconds) because the UI edits a "days" number directly.
func (r *Registry) AuditRetentionDays() int {
	return r.getInt(KeyAuditRetentionDays, func() int {
		return DefaultAuditRetentionDays
	})
}

// EnrollmentRequireApproval controls whether fresh agent enrollments
// land in `pending` (true) or auto-promote to `approved` (false). The
// per-PAT auto_approve flag overrides this — a PAT minted with
// auto_approve=true always pre-authorizes its host regardless of the
// global setting. Default true: an out-of-the-box deployment is safe
// against PAT leaks closing the loop without admin attention.
func (r *Registry) EnrollmentRequireApproval() bool {
	return r.getBool(KeyEnrollmentRequireApproval, func() bool {
		return DefaultEnrollmentRequireApproval
	})
}

// MeshPeers is the list of bootstrap peer addresses ("host:port").
// The mesh Node reconciles against this on every tick so admins can
// add / remove peers live from the Web UI without a restart. The
// bootstrap-config side was removed (server's own external_addr is
// used as the implicit bootstrap target); admins seed any cross-LAN
// peers through the Web UI / DB.
func (r *Registry) MeshPeers() []string {
	return r.getStringList(KeyMeshPeers, func() []string {
		return nil
	})
}

// ------------------- resolution helpers -------------------

func (r *Registry) getDuration(key string, fallback func() time.Duration) time.Duration {
	if v, ok := r.cacheLoad(key); ok {
		if d, ok := v.(time.Duration); ok {
			return d
		}
	}
	if raw, ok := r.dbRaw(key); ok {
		// duration is stored as JSON number of seconds
		var secs int64
		if err := json.Unmarshal([]byte(raw), &secs); err == nil && secs > 0 {
			d := time.Duration(secs) * time.Second
			r.cache.Store(key, &cacheEntry{value: d})
			return d
		}
	}
	d := fallback()
	r.cache.Store(key, &cacheEntry{value: d})
	return d
}

func (r *Registry) getBool(key string, fallback func() bool) bool {
	if v, ok := r.cacheLoad(key); ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	if raw, ok := r.dbRaw(key); ok {
		var b bool
		if err := json.Unmarshal([]byte(raw), &b); err == nil {
			r.cache.Store(key, &cacheEntry{value: b})
			return b
		}
	}
	b := fallback()
	r.cache.Store(key, &cacheEntry{value: b})
	return b
}

// getStringList handles JSON string-array settings. The DB value is
// a JSON array e.g. ["host1:9443","host2:9443"]. Returning nil vs an
// empty slice is significant: the fallback returning a non-nil empty
// slice means "admin explicitly overrode to empty", vs nil meaning
// "no value; use caller's backstop default".
func (r *Registry) getStringList(key string, fallback func() []string) []string {
	if v, ok := r.cacheLoad(key); ok {
		if l, ok := v.([]string); ok {
			return l
		}
	}
	if raw, ok := r.dbRaw(key); ok {
		var l []string
		if err := json.Unmarshal([]byte(raw), &l); err == nil {
			// Preserve empty-list-as-override: a non-nil empty slice
			// tells consumers "I meant no peers" rather than
			// "consult YAML".
			if l == nil {
				l = []string{}
			}
			r.cache.Store(key, &cacheEntry{value: l})
			return l
		}
	}
	l := fallback()
	r.cache.Store(key, &cacheEntry{value: l})
	return l
}

// getInt handles plain-integer settings (audit.retention_days is the
// first). The typeInt descriptor tag flows into the same JSON-number
// wire encoding; we just skip the seconds→duration conversion.
func (r *Registry) getInt(key string, fallback func() int) int {
	if v, ok := r.cacheLoad(key); ok {
		if n, ok := v.(int); ok {
			return n
		}
	}
	if raw, ok := r.dbRaw(key); ok {
		var n int64
		if err := json.Unmarshal([]byte(raw), &n); err == nil {
			r.cache.Store(key, &cacheEntry{value: int(n)})
			return int(n)
		}
	}
	n := fallback()
	r.cache.Store(key, &cacheEntry{value: n})
	return n
}

func (r *Registry) getString(key string, fallback func() string) string {
	if v, ok := r.cacheLoad(key); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	if raw, ok := r.dbRaw(key); ok {
		var s string
		if err := json.Unmarshal([]byte(raw), &s); err == nil {
			r.cache.Store(key, &cacheEntry{value: s})
			return s
		}
	}
	s := fallback()
	r.cache.Store(key, &cacheEntry{value: s})
	return s
}

func (r *Registry) cacheLoad(key string) (any, bool) {
	v, ok := r.cache.Load(key)
	if !ok {
		return nil, false
	}
	return v.(*cacheEntry).value, true
}

// dbRaw fetches the JSON-encoded value for key from the DB, if any.
// Swallows DB errors (returns !ok) because the registry must not
// panic a hot path on transient DB faults — the fallback chain
// always produces a sensible answer.
func (r *Registry) dbRaw(key string) (string, bool) {
	if r.db == nil {
		return "", false
	}
	row, err := r.db.AdminSettings().Get(context.Background(), key)
	if err != nil {
		return "", false
	}
	return row.Value, true
}

// ------------------- write path -------------------

// ErrUnknownKey is returned by Set/Reset when the caller passes a key
// that isn't one of the registered Phase-1 settings.
var ErrUnknownKey = errors.New("settings: unknown key")

// ErrBadValue is returned when the JSON payload fails type validation
// (e.g. negative TTL, non-boolean for a bool key).
var ErrBadValue = errors.New("settings: invalid value for key")

// Set validates the raw JSON payload against the key's expected type,
// writes it to the DB, invalidates the cache entry, and records an
// audit row. byUserID is the user making the change (for the audit
// log's updated_by column and activity.ActorUser field).
func (r *Registry) Set(ctx context.Context, key, rawJSON, byUserID string) error {
	if err := validate(key, rawJSON); err != nil {
		return err
	}
	if r.db == nil {
		return errors.New("settings: no db")
	}
	now := time.Now().UTC()
	if err := r.db.AdminSettings().Upsert(ctx, key, rawJSON, byUserID, now); err != nil {
		return err
	}
	r.cache.Delete(key)
	activity.RecordWithContext(ctx, activity.Input{
		ActorType:   storage.ActorTypeUser,
		ActorUser:   byUserID,
		Category:    storage.CategoryAdmin,
		Action:      "settings.set",
		TargetType:  "setting",
		TargetID:    key,
		TargetLabel: key,
		Outcome:     storage.OutcomeSuccess,
		Meta:        map[string]string{"value": rawJSON},
		At:          now,
	})
	return nil
}

// Reset deletes the DB override for key, so subsequent reads fall
// back to YAML / hardcoded default. Cache is invalidated and an
// audit row recorded. No-op (but still audited) if the key had no
// override.
func (r *Registry) Reset(ctx context.Context, key, byUserID string) error {
	if _, ok := descriptor(key); !ok {
		return ErrUnknownKey
	}
	if r.db == nil {
		return errors.New("settings: no db")
	}
	if err := r.db.AdminSettings().Delete(ctx, key); err != nil {
		return err
	}
	r.cache.Delete(key)
	activity.RecordWithContext(ctx, activity.Input{
		ActorType:   storage.ActorTypeUser,
		ActorUser:   byUserID,
		Category:    storage.CategoryAdmin,
		Action:      "settings.reset",
		TargetType:  "setting",
		TargetID:    key,
		TargetLabel: key,
		Outcome:     storage.OutcomeSuccess,
		At:          time.Now().UTC(),
	})
	return nil
}

// validate decodes the raw JSON and checks it's the right shape for
// key's registered type. Returns ErrUnknownKey for unregistered keys,
// ErrBadValue for type mismatches or out-of-range values.
func validate(key, rawJSON string) error {
	d, ok := descriptor(key)
	if !ok {
		return ErrUnknownKey
	}
	switch d.Type {
	case typeDurationSeconds, typeInt:
		var n int64
		if err := json.Unmarshal([]byte(rawJSON), &n); err != nil {
			return fmt.Errorf("%w: expected number: %v", ErrBadValue, err)
		}
		if d.Type == typeDurationSeconds && n <= 0 {
			return fmt.Errorf("%w: duration must be > 0 seconds", ErrBadValue)
		}
	case typeBool:
		var b bool
		if err := json.Unmarshal([]byte(rawJSON), &b); err != nil {
			return fmt.Errorf("%w: expected bool: %v", ErrBadValue, err)
		}
	case typeString:
		var s string
		if err := json.Unmarshal([]byte(rawJSON), &s); err != nil {
			return fmt.Errorf("%w: expected string: %v", ErrBadValue, err)
		}
	case typeStringList:
		var l []string
		if err := json.Unmarshal([]byte(rawJSON), &l); err != nil {
			return fmt.Errorf("%w: expected string array: %v", ErrBadValue, err)
		}
		for i, s := range l {
			if s == "" {
				return fmt.Errorf("%w: list entry %d is empty", ErrBadValue, i)
			}
		}
	default:
		return fmt.Errorf("%w: unknown descriptor type %q", ErrBadValue, d.Type)
	}
	return nil
}
