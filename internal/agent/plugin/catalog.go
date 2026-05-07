package plugin

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"
)

// CatalogEntry is one row of the on-disk installed-plugin index. Fields
// are stable wire-style names so a future tool reading catalog.json
// from outside this package isn't broken by Go-side renames.
type CatalogEntry struct {
	ID                  string    `json:"id"`
	Version             string    `json:"version"`
	Name                string    `json:"name"`
	Author              string    `json:"author"`
	Enabled             bool      `json:"enabled"`
	// GrantedCapabilities uses the typed CapabilityID set so the
	// loader / install path can't accidentally compare a typo'd
	// raw string against the manifest's typed declaration. JSON
	// shape stays as `["fs.read", ...]` since CapabilityID is a
	// string-derived type.
	GrantedCapabilities []CapabilityID `json:"granted_capabilities"`
	InstalledAt         time.Time `json:"installed_at"`
	SourceURL           string    `json:"source_url,omitempty"` // empty for inline installs
	PublisherKeyID      string    `json:"publisher_key_id"`

	// System marks plugins shipped inside the agent binary and
	// auto-installed on startup by the system-plugin bootstrap. Their
	// uninstall path returns plugin_is_system; the bootstrap
	// re-installs them on the next boot anyway, so a forced uninstall
	// would be transient at best.
	System bool `json:"system,omitempty"`

	// ConfigJSON is the operator's resolved (post-secret-substitution)
	// plugin config blob, persisted across agent restarts so a plugin
	// re-instantiated on boot sees the same config it saw at install
	// time. Stored as raw JSON bytes — encoding/json's
	// json.RawMessage type round-trips without re-encoding, which
	// keeps the bytes byte-identical (important if the operator
	// later checksums their preset configurations).
	//
	// Empty when the plugin declares no config block, or when the
	// operator installed it without overrides. The loader treats
	// empty as "no config" and skips the platypus_config Manifest
	// entry; the plugin's GetConfig helper returns "" in that case.
	ConfigJSON json.RawMessage `json:"config_json,omitempty"`

	// ConfigSchemaVersion pins the manifest's config.schema_version
	// the ConfigJSON was authored against. Carried alongside the
	// bytes so a future agent-side validator can refuse to apply
	// a stale config when the on-disk manifest's schema_version
	// has moved on.
	ConfigSchemaVersion int32 `json:"config_schema_version,omitempty"`
}

// Catalog is the in-memory + on-disk view of installed plugins. All
// reads/writes are serialised; the on-disk write is atomic (write to
// .tmp + rename) so a crash mid-install never produces a partial file.
type Catalog struct {
	path    string
	mu      sync.Mutex
	entries map[string]CatalogEntry // keyed by plugin id
}

// LoadCatalog reads catalog.json; a missing file is treated as an empty
// catalog (first-run case). Other errors propagate so the caller can
// decide whether to refuse to boot or to continue with an empty set.
func LoadCatalog(path string) (*Catalog, error) {
	c := &Catalog{path: path, entries: map[string]CatalogEntry{}}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return nil, fmt.Errorf("plugin: read catalog %s: %w", path, err)
	}
	var raw struct {
		Plugins []CatalogEntry `json:"plugins"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("plugin: parse catalog %s: %w", path, err)
	}
	for _, e := range raw.Plugins {
		c.entries[e.ID] = e
	}
	return c, nil
}

// All returns a snapshot of every entry, sorted by id for deterministic
// output (PluginListResponse, log lines, tests).
func (c *Catalog) All() []CatalogEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]CatalogEntry, 0, len(c.entries))
	for _, e := range c.entries {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Get returns one entry by id; the second value is false when missing.
func (c *Catalog) Get(id string) (CatalogEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[id]
	return e, ok
}

// Upsert writes one entry and persists the whole catalog atomically.
// Used by install/enable handlers; the lock is held across the file
// rename so concurrent writers don't lose each other's changes.
func (c *Catalog) Upsert(e CatalogEntry) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[e.ID] = e
	return c.writeLocked()
}

// Remove drops one entry and persists. Returns nil if the id was
// already absent (uninstall is idempotent).
func (c *Catalog) Remove(id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, id)
	return c.writeLocked()
}

// SetEnabled flips the enabled flag for one entry. Returns
// os.ErrNotExist when id is unknown.
func (c *Catalog) SetEnabled(id string, enabled bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[id]
	if !ok {
		return os.ErrNotExist
	}
	e.Enabled = enabled
	c.entries[id] = e
	return c.writeLocked()
}

func (c *Catalog) writeLocked() error {
	out := struct {
		Plugins []CatalogEntry `json:"plugins"`
	}{Plugins: make([]CatalogEntry, 0, len(c.entries))}
	for _, e := range c.entries {
		out.Plugins = append(out.Plugins, e)
	}
	sort.Slice(out.Plugins, func(i, j int) bool { return out.Plugins[i].ID < out.Plugins[j].ID })

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return fmt.Errorf("plugin: marshal catalog: %w", err)
	}
	if err := os.MkdirAll(dirOf(c.path), 0o700); err != nil {
		return fmt.Errorf("plugin: mkdir catalog parent: %w", err)
	}
	tmp := c.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("plugin: write catalog tmp: %w", err)
	}
	if err := os.Rename(tmp, c.path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("plugin: rename catalog: %w", err)
	}
	return nil
}

// dirOf is the parent dir of path. We avoid importing path/filepath
// just for this so the file stays grep-able for "filepath" auditors.
func dirOf(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' {
			return p[:i]
		}
	}
	return "."
}
