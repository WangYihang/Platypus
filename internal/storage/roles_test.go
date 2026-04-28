package storage_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/WangYihang/Platypus/internal/storage"
)

func TestRoles_List_Builtins(t *testing.T) {
	t.Parallel()
	db := newAuthDB(t)
	ctx := context.Background()

	roles, err := db.Roles().List(ctx, storage.RoleFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	bySlug := map[string]*storage.Role{}
	for _, r := range roles {
		bySlug[r.Slug] = r
	}
	for _, want := range []string{"viewer", "operator", "admin"} {
		r, ok := bySlug[want]
		if !ok {
			t.Errorf("List missing builtin role %q", want)
			continue
		}
		if !r.IsBuiltin {
			t.Errorf("role %q IsBuiltin = false, want true", want)
		}
		if !r.IsGlobal || !r.IsProject {
			t.Errorf("builtin role %q must be assignable both globally and per-project", want)
		}
	}
}

func TestRoles_List_FilterGlobal(t *testing.T) {
	t.Parallel()
	db := newAuthDB(t)
	ctx := context.Background()

	yes := true
	roles, err := db.Roles().List(ctx, storage.RoleFilter{IsGlobal: &yes})
	if err != nil {
		t.Fatalf("List filtered: %v", err)
	}
	for _, r := range roles {
		if !r.IsGlobal {
			t.Errorf("filter=is_global true returned %q with IsGlobal=false", r.Slug)
		}
	}
	if len(roles) < 3 {
		t.Errorf("expected at least three global roles (viewer/operator/admin), got %d", len(roles))
	}
}

func TestRoles_Get_PopulatesPermissions(t *testing.T) {
	t.Parallel()
	db := newAuthDB(t)
	ctx := context.Background()

	r, err := db.Roles().Get(ctx, "viewer")
	if err != nil {
		t.Fatalf("Get viewer: %v", err)
	}
	want := []string{"hosts:read", "files:read", "projects:read", "activity:read"}
	if !equalSetsStr(r.Permissions, want) {
		t.Errorf("viewer permissions = %v, want %v", r.Permissions, want)
	}

	if _, err := db.Roles().Get(ctx, "nonexistent"); !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("Get(missing) = %v, want ErrNotFound", err)
	}
}

func TestRoles_HasPermission(t *testing.T) {
	t.Parallel()
	db := newAuthDB(t)
	ctx := context.Background()

	cases := []struct {
		role, perm string
		want       bool
	}{
		{"viewer", "hosts:read", true},
		{"viewer", "hosts:exec", false},
		{"operator", "hosts:exec", true},
		{"admin", "admin:roles", true},
		{"viewer", "admin:roles", false},
	}
	for _, tc := range cases {
		got, err := db.Roles().HasPermission(ctx, tc.role, tc.perm)
		if err != nil {
			t.Errorf("HasPermission(%q, %q): %v", tc.role, tc.perm, err)
			continue
		}
		if got != tc.want {
			t.Errorf("HasPermission(%q, %q) = %v, want %v", tc.role, tc.perm, got, tc.want)
		}
	}
}

func TestRoles_Create_CustomRoleRoundtrip(t *testing.T) {
	t.Parallel()
	db := newAuthDB(t)
	ctx := context.Background()

	r := &storage.Role{
		Slug:        "oncall",
		Name:        "On-call Operator",
		Description: "Time-bound operator role granted during oncall rotation.",
		IsBuiltin:   false,
		IsGlobal:    false,
		IsProject:   true,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	perms := []string{"hosts:read", "hosts:exec", "files:read"}

	if err := db.Roles().Create(ctx, r, perms); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := db.Roles().Get(ctx, "oncall")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.IsBuiltin || got.IsGlobal || !got.IsProject {
		t.Errorf("flag mismatch: %+v", got)
	}
	if !equalSetsStr(got.Permissions, perms) {
		t.Errorf("perms = %v, want %v", got.Permissions, perms)
	}
}

func TestRoles_Create_DuplicateSlugRejected(t *testing.T) {
	t.Parallel()
	db := newAuthDB(t)
	ctx := context.Background()

	now := time.Now().UTC()
	if err := db.Roles().Create(ctx,
		&storage.Role{Slug: "oncall", Name: "x", IsProject: true, CreatedAt: now, UpdatedAt: now},
		[]string{"hosts:read"},
	); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	err := db.Roles().Create(ctx,
		&storage.Role{Slug: "oncall", Name: "x", IsProject: true, CreatedAt: now, UpdatedAt: now},
		[]string{"hosts:read"},
	)
	if err == nil {
		t.Error("second Create with duplicate slug succeeded; want PRIMARY KEY violation")
	}
}

func TestRoles_Update_ReplacesPermissions(t *testing.T) {
	t.Parallel()
	db := newAuthDB(t)
	ctx := context.Background()

	r, _ := db.Roles().Get(ctx, "viewer")
	r.Description = "Edited description"
	newPerms := []string{"hosts:read"} // shrink
	if err := db.Roles().Update(ctx, r, newPerms); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, _ := db.Roles().Get(ctx, "viewer")
	if got.Description != "Edited description" {
		t.Errorf("description not updated: %q", got.Description)
	}
	if !equalSetsStr(got.Permissions, newPerms) {
		t.Errorf("perms not replaced: %v", got.Permissions)
	}
}

func TestRoles_Update_AdminRoleProtect(t *testing.T) {
	t.Parallel()
	db := newAuthDB(t)
	ctx := context.Background()

	r, _ := db.Roles().Get(ctx, "admin")
	// Try to drop admin:roles. The DB trigger should abort the
	// UPDATE — Update is implemented as a permission-set replace,
	// which means deleting the missing rows; the trigger fires.
	stripped := []string{"hosts:read", "hosts:exec"}
	err := db.Roles().Update(ctx, r, stripped)
	if err == nil {
		t.Fatal("Update admin role without admin:* succeeded; want trigger to abort")
	}
}

func TestRoles_Delete_BuiltinRejected(t *testing.T) {
	t.Parallel()
	db := newAuthDB(t)
	ctx := context.Background()

	err := db.Roles().Delete(ctx, "viewer")
	if !errors.Is(err, storage.ErrRoleBuiltin) {
		t.Errorf("Delete(builtin) = %v, want ErrRoleBuiltin", err)
	}
	// Still there.
	if _, err := db.Roles().Get(ctx, "viewer"); err != nil {
		t.Errorf("viewer disappeared: %v", err)
	}
}

func TestRoles_Delete_InUseRejected(t *testing.T) {
	t.Parallel()
	db := newAuthDB(t)
	ctx := context.Background()

	now := time.Now().UTC()
	if err := db.Roles().Create(ctx,
		&storage.Role{Slug: "support", Name: "Support", IsGlobal: true, CreatedAt: now, UpdatedAt: now},
		[]string{"hosts:read"},
	); err != nil {
		t.Fatalf("Create: %v", err)
	}

	makeUser(t, db, "u-support")
	if _, err := db.Exec(`UPDATE users SET role='support' WHERE id='u-support'`); err != nil {
		t.Fatalf("seed user with role: %v", err)
	}

	err := db.Roles().Delete(ctx, "support")
	if !errors.Is(err, storage.ErrRoleInUse) {
		t.Errorf("Delete(in-use) = %v, want ErrRoleInUse", err)
	}
}

func TestRoles_Delete_Custom(t *testing.T) {
	t.Parallel()
	db := newAuthDB(t)
	ctx := context.Background()

	now := time.Now().UTC()
	if err := db.Roles().Create(ctx,
		&storage.Role{Slug: "tmp", Name: "Tmp", IsProject: true, CreatedAt: now, UpdatedAt: now},
		[]string{"hosts:read"},
	); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := db.Roles().Delete(ctx, "tmp"); err != nil {
		t.Errorf("Delete(custom unused): %v", err)
	}
	if _, err := db.Roles().Get(ctx, "tmp"); !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("post-Delete Get = %v, want ErrNotFound", err)
	}
}

// equalSetsStr is a small helper kept local so it doesn't collide
// with the other test files' helpers — order-insensitive set
// equality on string slices.
func equalSetsStr(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	m := make(map[string]int, len(a))
	for _, s := range a {
		m[s]++
	}
	for _, s := range b {
		m[s]--
		if m[s] < 0 {
			return false
		}
	}
	return true
}
