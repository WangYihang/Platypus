package storage_test

import (
	"context"
	"testing"
	"time"

	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

func seedProject(t *testing.T, db *storage.DB, slug, name string, creator *user.User) *storage.Project {
	t.Helper()
	p := &storage.Project{
		ID:        "prj-" + slug,
		Name:      name,
		Slug:      slug,
		CreatedAt: time.Now().UTC(),
		CreatedBy: creator.ID,
	}
	if err := db.Projects().Create(context.Background(), p); err != nil {
		t.Fatalf("Projects().Create(%q): %v", slug, err)
	}
	return p
}

func TestProjectRepo_CreateAndGet(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	u := seedUser(t, db, "alice", user.RoleAdmin)

	p := seedProject(t, db, "prod", "Production", u)

	got, err := db.Projects().GetByID(ctx, p.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Slug != "prod" || got.Name != "Production" || got.CreatedBy != u.ID {
		t.Fatalf("mismatch: %+v", got)
	}

	bySlug, err := db.Projects().GetBySlug(ctx, "prod")
	if err != nil {
		t.Fatalf("GetBySlug: %v", err)
	}
	if bySlug.ID != p.ID {
		t.Fatalf("GetBySlug ID mismatch: %s vs %s", bySlug.ID, p.ID)
	}
}

func TestProjectRepo_DuplicateSlug(t *testing.T) {
	db := newTestDB(t)
	u := seedUser(t, db, "alice", user.RoleAdmin)
	seedProject(t, db, "prod", "Production", u)

	err := db.Projects().Create(context.Background(), &storage.Project{
		ID: "prj-other", Name: "Other", Slug: "prod",
		CreatedAt: time.Now().UTC(), CreatedBy: u.ID,
	})
	if err == nil {
		t.Fatal("expected UNIQUE violation on slug")
	}
}

func TestProjectRepo_ListOrderedBySlug(t *testing.T) {
	db := newTestDB(t)
	u := seedUser(t, db, "alice", user.RoleAdmin)
	seedProject(t, db, "prod", "Production", u)
	seedProject(t, db, "dev", "Development", u)
	seedProject(t, db, "staging", "Staging", u)

	projects, err := db.Projects().List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	want := []string{"dev", "prod", "staging"}
	if len(projects) != len(want) {
		t.Fatalf("len=%d; want %d", len(projects), len(want))
	}
	for i, p := range projects {
		if p.Slug != want[i] {
			t.Errorf("projects[%d].Slug = %q; want %q", i, p.Slug, want[i])
		}
	}
}

func TestProjectRepo_Delete(t *testing.T) {
	db := newTestDB(t)
	u := seedUser(t, db, "alice", user.RoleAdmin)
	p := seedProject(t, db, "prod", "Production", u)

	if err := db.Projects().Delete(context.Background(), p.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err := db.Projects().GetByID(context.Background(), p.ID)
	if err != storage.ErrNotFound {
		t.Fatalf("err = %v; want ErrNotFound", err)
	}
}

func TestProjectMembers_AddAndGetRole(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	bob := seedUser(t, db, "bob", user.RoleViewer)
	p := seedProject(t, db, "prod", "Production", admin)

	if err := db.Projects().AddMember(ctx, p.ID, bob.ID, user.RoleOperator); err != nil {
		t.Fatalf("AddMember: %v", err)
	}

	got, err := db.Projects().MemberRole(ctx, p.ID, bob.ID)
	if err != nil {
		t.Fatalf("MemberRole: %v", err)
	}
	if got != user.RoleOperator {
		t.Fatalf("role = %q; want operator", got)
	}
}

func TestProjectMembers_MissingMember(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	p := seedProject(t, db, "prod", "Production", admin)

	_, err := db.Projects().MemberRole(ctx, p.ID, "nonexistent")
	if err != storage.ErrNotFound {
		t.Fatalf("err = %v; want ErrNotFound", err)
	}
}

func TestProjectMembers_RemoveMember(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	bob := seedUser(t, db, "bob", user.RoleViewer)
	p := seedProject(t, db, "prod", "Production", admin)

	_ = db.Projects().AddMember(ctx, p.ID, bob.ID, user.RoleOperator)
	if err := db.Projects().RemoveMember(ctx, p.ID, bob.ID); err != nil {
		t.Fatalf("RemoveMember: %v", err)
	}
	_, err := db.Projects().MemberRole(ctx, p.ID, bob.ID)
	if err != storage.ErrNotFound {
		t.Fatalf("err = %v; want ErrNotFound after Remove", err)
	}
}

// ListForUser returns every project the given user can see. Global
// admins see all; other users only see projects where a project_members
// row exists.
func TestProjectRepo_ListForUser(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	bob := seedUser(t, db, "bob", user.RoleOperator)

	prod := seedProject(t, db, "prod", "Production", admin)
	seedProject(t, db, "staging", "Staging", admin)

	_ = db.Projects().AddMember(ctx, prod.ID, bob.ID, user.RoleOperator)

	forBob, err := db.Projects().ListForUser(ctx, bob.ID, user.RoleOperator)
	if err != nil {
		t.Fatalf("ListForUser(bob): %v", err)
	}
	if len(forBob) != 1 || forBob[0].Slug != "prod" {
		t.Fatalf("bob sees: %+v; want only [prod]", forBob)
	}

	forAdmin, err := db.Projects().ListForUser(ctx, admin.ID, user.RoleAdmin)
	if err != nil {
		t.Fatalf("ListForUser(admin): %v", err)
	}
	if len(forAdmin) != 2 {
		t.Fatalf("admin sees %d projects; want 2 (all)", len(forAdmin))
	}
}

// Cascading: deleting a project removes its project_members rows.
func TestProjectRepo_DeleteCascadesMembers(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	bob := seedUser(t, db, "bob", user.RoleViewer)
	p := seedProject(t, db, "prod", "Production", admin)
	_ = db.Projects().AddMember(ctx, p.ID, bob.ID, user.RoleViewer)

	if err := db.Projects().Delete(ctx, p.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if _, err := db.Projects().MemberRole(ctx, p.ID, bob.ID); err != storage.ErrNotFound {
		t.Fatalf("member row still present after project delete: err=%v", err)
	}
}
