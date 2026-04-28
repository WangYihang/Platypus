package storage_test

import (
	"context"
	"testing"

	"github.com/WangYihang/Platypus/internal/storage"
)

func TestPermissions_List(t *testing.T) {
	t.Parallel()
	db := newAuthDB(t)
	ctx := context.Background()

	all, err := db.Permissions().List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	// At minimum the seeded catalogue is present.
	want := []string{
		"hosts:read", "hosts:exec",
		"admin:users", "admin:roles", "admin:settings",
	}
	got := map[string]struct{}{}
	for _, p := range all {
		got[p.Slug] = struct{}{}
		if p.Resource == "" || p.Description == "" {
			t.Errorf("permission %q has empty resource/description", p.Slug)
		}
	}
	for _, w := range want {
		if _, ok := got[w]; !ok {
			t.Errorf("List missing %q", w)
		}
	}
}

func TestPermissions_Get(t *testing.T) {
	t.Parallel()
	db := newAuthDB(t)
	ctx := context.Background()

	p, err := db.Permissions().Get(ctx, "hosts:exec")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if p.Slug != "hosts:exec" || p.Resource != "hosts" {
		t.Errorf("got %+v", p)
	}

	if _, err := db.Permissions().Get(ctx, "does:not:exist"); err != storage.ErrNotFound {
		t.Errorf("Get(missing) = %v, want ErrNotFound", err)
	}
}
