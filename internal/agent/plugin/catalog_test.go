package plugin

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"
)

func TestCatalog_LoadEmpty(t *testing.T) {
	dir := t.TempDir()
	c, err := LoadCatalog(filepath.Join(dir, "catalog.json"))
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if got := c.All(); len(got) != 0 {
		t.Errorf("All() = %v, want empty", got)
	}
}

func TestCatalog_UpsertAndPersist(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "catalog.json")
	c, err := LoadCatalog(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	want := CatalogEntry{
		ID:                  "com.example.foo",
		Version:             "1.0.0",
		Name:                "Foo",
		Author:              "Jane",
		Enabled:             true,
		GrantedCapabilities: []string{"kv", "fs.read"},
		InstalledAt:         time.Unix(1700000000, 0).UTC(),
		PublisherKeyID:      "deadbeefdeadbeef",
	}
	if err := c.Upsert(want); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	// Re-open and round-trip.
	c2, err := LoadCatalog(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	got, ok := c2.Get("com.example.foo")
	if !ok {
		t.Fatalf("missing after reload")
	}
	if got.ID != want.ID || got.Version != want.Version || !got.InstalledAt.Equal(want.InstalledAt) {
		t.Errorf("got %+v, want %+v", got, want)
	}
	if len(got.GrantedCapabilities) != 2 {
		t.Errorf("granted = %v", got.GrantedCapabilities)
	}
}

func TestCatalog_SetEnabledAndRemove(t *testing.T) {
	dir := t.TempDir()
	c, err := LoadCatalog(filepath.Join(dir, "catalog.json"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if err := c.Upsert(CatalogEntry{ID: "a.b.c", Version: "1.0.0", Enabled: true}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := c.SetEnabled("a.b.c", false); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if e, _ := c.Get("a.b.c"); e.Enabled {
		t.Errorf("expected disabled")
	}
	if err := c.SetEnabled("missing", true); err == nil {
		t.Errorf("expected ErrNotExist for missing id")
	}
	if err := c.Remove("a.b.c"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, ok := c.Get("a.b.c"); ok {
		t.Errorf("entry still present after remove")
	}
	// Idempotent
	if err := c.Remove("a.b.c"); err != nil {
		t.Errorf("repeat remove: %v", err)
	}
}

// TestCatalog_ConfigJSON_Roundtrip: PR 3.6 lands the config_json
// field on CatalogEntry so the agent can persist the operator's
// resolved plugin config across restarts. After reload, the
// stored config decodes to the same JSON value the operator
// authored — whitespace / key ordering can differ (catalog.json
// is pretty-printed for human reads), but every value at every
// path matches. Pinning equivalence rather than byte-identity
// keeps the catalog file readable without sacrificing the
// "plugin sees what was authored" guarantee.
func TestCatalog_ConfigJSON_Roundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "catalog.json")
	c, _ := LoadCatalog(path)

	want := CatalogEntry{
		ID:                  "com.example.syslog",
		Version:             "1.4.0",
		Name:                "Syslog",
		Author:              "Jane",
		Enabled:             true,
		GrantedCapabilities: []string{"net.dial"},
		InstalledAt:         time.Unix(1700000000, 0).UTC(),
		PublisherKeyID:      "deadbeefdeadbeef",
		ConfigJSON:          []byte(`{"destination":"udp://10.0.0.1:514","tls":true}`),
		ConfigSchemaVersion: 1,
	}
	if err := c.Upsert(want); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	c2, err := LoadCatalog(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	got, ok := c2.Get("com.example.syslog")
	if !ok {
		t.Fatalf("missing after reload")
	}
	var gotV, wantV any
	if err := json.Unmarshal(got.ConfigJSON, &gotV); err != nil {
		t.Fatalf("decode got: %v (raw=%s)", err, got.ConfigJSON)
	}
	if err := json.Unmarshal(want.ConfigJSON, &wantV); err != nil {
		t.Fatalf("decode want: %v", err)
	}
	gotCanon, _ := json.Marshal(gotV)
	wantCanon, _ := json.Marshal(wantV)
	if string(gotCanon) != string(wantCanon) {
		t.Fatalf("config_json drift after canonicalising:\n  got:  %s\n  want: %s",
			gotCanon, wantCanon)
	}
	if got.ConfigSchemaVersion != 1 {
		t.Fatalf("config_schema_version = %d, want 1", got.ConfigSchemaVersion)
	}
}

// TestCatalog_ConfigJSON_AbsentRoundTripsAsNil: a plugin without
// config (the legacy path) must NOT gain a synthetic empty config
// blob. Pinning the absence is what lets the loader distinguish
// "no config block declared" from "operator authored an empty
// override map" — the two behave differently in the manifest
// validator.
func TestCatalog_ConfigJSON_AbsentRoundTripsAsNil(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "catalog.json")
	c, _ := LoadCatalog(path)

	if err := c.Upsert(CatalogEntry{
		ID: "com.example.legacy", Version: "1.0", Enabled: true,
		InstalledAt: time.Unix(1700000000, 0).UTC(),
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	c2, _ := LoadCatalog(path)
	got, _ := c2.Get("com.example.legacy")
	if len(got.ConfigJSON) != 0 {
		t.Fatalf("legacy entry gained config_json bytes: %s", got.ConfigJSON)
	}
}
