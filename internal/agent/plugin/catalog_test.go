package plugin

import (
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
