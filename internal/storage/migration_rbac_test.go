package storage

import (
	"context"
	"sort"
	"testing"
)

// TestMigration_RBAC_Tables locks in migration 000018: the three
// permission/role tables exist after Open() runs every pending
// migration.
func TestMigration_RBAC_Tables(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	for _, want := range []string{"permissions", "roles", "role_permissions"} {
		var name string
		err := db.QueryRow(
			`SELECT name FROM sqlite_master WHERE type='table' AND name=?`,
			want,
		).Scan(&name)
		if err != nil {
			t.Errorf("expected table %q after migration 18: %v", want, err)
		}
	}
}

// TestMigration_RBAC_BuiltinRoles asserts the three builtin roles
// (viewer / operator / admin) are present, marked is_builtin=1, and
// have the expected slug → permissions mapping. The mapping is the
// behaviour contract callers in optoken / api will rely on; pinning
// it here keeps an unintended seed edit from silently shrinking
// every existing user's effective permission set.
func TestMigration_RBAC_BuiltinRoles(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	cases := map[string][]string{
		"viewer": {
			"hosts:read", "files:read", "projects:read", "activity:read",
		},
		"operator": {
			"hosts:read", "files:read", "projects:read", "activity:read",
			"hosts:exec", "files:write", "files:delete",
			"rpc:invoke", "enrollment:issue", "enrollment:revoke",
		},
		"admin": {
			"hosts:read", "hosts:exec",
			"files:read", "files:write", "files:delete",
			"projects:read", "projects:create", "projects:settings", "projects:delete",
			"activity:read", "activity:export",
			"enrollment:issue", "enrollment:revoke",
			"rpc:invoke",
			"admin:users", "admin:roles", "admin:settings",
		},
	}

	for slug, wantPerms := range cases {
		// Role row exists, is_builtin=1.
		var isBuiltin int
		err := db.QueryRow(
			`SELECT is_builtin FROM roles WHERE slug = ?`, slug,
		).Scan(&isBuiltin)
		if err != nil {
			t.Errorf("role %q missing: %v", slug, err)
			continue
		}
		if isBuiltin != 1 {
			t.Errorf("role %q is_builtin = %d, want 1", slug, isBuiltin)
		}

		// Permissions match exactly (set equality).
		rows, err := db.Query(
			`SELECT perm_slug FROM role_permissions WHERE role_slug = ? ORDER BY perm_slug`,
			slug,
		)
		if err != nil {
			t.Fatalf("query role_permissions for %q: %v", slug, err)
		}
		var got []string
		for rows.Next() {
			var p string
			if err := rows.Scan(&p); err != nil {
				t.Fatalf("scan: %v", err)
			}
			got = append(got, p)
		}
		_ = rows.Close()

		sort.Strings(wantPerms)
		if !equalSlices(got, wantPerms) {
			t.Errorf("role %q permissions:\n  got  %v\n  want %v", slug, got, wantPerms)
		}
	}
}

// TestMigration_RBAC_AdminRoleProtect asserts the DB-side guarantee
// that the admin role's admin:* permissions cannot be removed, even
// by a direct DELETE against role_permissions. UI-side prevention is
// also in place, but the trigger is the safety net for raw SQL or a
// future bug in the admin UI.
func TestMigration_RBAC_AdminRoleProtect(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	_, err = db.ExecContext(ctx,
		`DELETE FROM role_permissions WHERE role_slug = 'admin' AND perm_slug = 'admin:roles'`,
	)
	if err == nil {
		t.Fatal("DELETE admin:roles from admin role succeeded; expected protective trigger to abort")
	}

	// Verify the row is still there.
	var count int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM role_permissions WHERE role_slug='admin' AND perm_slug='admin:roles'`,
	).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("admin:roles row count = %d after attempted delete; want 1", count)
	}
}

// TestMigration_RBAC_PermissionsCatalog locks in the canonical
// permission catalogue: every slug ever referenced in role_permissions
// must exist in permissions, every permission has a non-empty
// resource group + description for the admin UI.
func TestMigration_RBAC_PermissionsCatalog(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	rows, err := db.Query(`SELECT slug, resource, description FROM permissions ORDER BY slug`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer func() { _ = rows.Close() }()

	got := map[string]struct{ resource, desc string }{}
	for rows.Next() {
		var slug, resource, desc string
		if err := rows.Scan(&slug, &resource, &desc); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if resource == "" {
			t.Errorf("permission %q has empty resource", slug)
		}
		if desc == "" {
			t.Errorf("permission %q has empty description", slug)
		}
		got[slug] = struct{ resource, desc string }{resource, desc}
	}

	want := []string{
		"hosts:read", "hosts:exec",
		"files:read", "files:write", "files:delete",
		"projects:read", "projects:create", "projects:settings", "projects:delete",
		"activity:read", "activity:export",
		"enrollment:issue", "enrollment:revoke",
		"rpc:invoke",
		"admin:users", "admin:roles", "admin:settings",
	}
	for _, slug := range want {
		if _, ok := got[slug]; !ok {
			t.Errorf("permission %q missing from catalogue", slug)
		}
	}
}

// TestMigration_RBAC_UsersRoleFK pins the rebuild of users and
// project_members in migration 18: the role column now FKs to
// roles.slug, so an INSERT with an unknown slug fails and a known
// slug succeeds. PRAGMA foreign_key_check stays clean — the rebuild
// preserved every cross-table FK pointing at users(id) /
// project_members(project_id, user_id).
func TestMigration_RBAC_UsersRoleFK(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `
		INSERT INTO users (id, username, password_hash, role, created_at)
		VALUES ('u-fk', 'u-fk', 'x', 'viewer', CURRENT_TIMESTAMP)`,
	); err != nil {
		t.Fatalf("INSERT with valid role slug failed: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO users (id, username, password_hash, role, created_at)
		VALUES ('u-bad', 'u-bad', 'x', 'no-such-role', CURRENT_TIMESTAMP)`,
	)
	if err == nil {
		t.Fatal("INSERT with unknown role slug succeeded; FK should reject it")
	}

	rows, err := db.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatalf("foreign_key_check: %v", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var table, parent string
		var rowid int64
		var fkid int
		if err := rows.Scan(&table, &rowid, &parent, &fkid); err != nil {
			t.Fatalf("scan: %v", err)
		}
		t.Errorf("foreign_key_check violation: table=%s rowid=%d parent=%s fkid=%d",
			table, rowid, parent, fkid)
	}
}

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	sort.Strings(a)
	sort.Strings(b)
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
