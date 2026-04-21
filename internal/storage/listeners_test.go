package storage_test

import (
	"context"
	"testing"
	"time"

	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

func seedListener(t *testing.T, db *storage.DB, id string, proj *storage.Project, host string, port uint16) *storage.Listener {
	t.Helper()
	l := &storage.Listener{
		ID:        id,
		ProjectID: proj.ID,
		Host:      host,
		Port:      port,
		CreatedAt: time.Now().UTC(),
	}
	if err := db.Listeners().Create(context.Background(), l); err != nil {
		t.Fatalf("Listeners().Create(%q): %v", id, err)
	}
	return l
}

func TestListenerRepo_CreateAndGet(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	proj := seedProject(t, db, "prod", "Production", admin)

	seedListener(t, db, "lis-1", proj, "0.0.0.0", 13337)
	got, err := db.Listeners().GetByID(ctx, "lis-1")
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.ProjectID != proj.ID || got.Port != 13337 {
		t.Fatalf("mismatch: %+v", got)
	}
}

func TestListenerRepo_ListByProject(t *testing.T) {
	db := newTestDB(t)
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	prod := seedProject(t, db, "prod", "Production", admin)
	stag := seedProject(t, db, "staging", "Staging", admin)

	seedListener(t, db, "lis-1", prod, "0.0.0.0", 13337)
	seedListener(t, db, "lis-2", prod, "0.0.0.0", 13338)
	seedListener(t, db, "lis-3", stag, "0.0.0.0", 22)

	prodList, _ := db.Listeners().ListByProject(context.Background(), prod.ID)
	if len(prodList) != 2 {
		t.Fatalf("prod listeners = %d; want 2", len(prodList))
	}
}

func TestListenerRepo_Delete(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	proj := seedProject(t, db, "prod", "Production", admin)
	l := seedListener(t, db, "lis-1", proj, "0.0.0.0", 13337)

	if err := db.Listeners().Delete(ctx, l.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := db.Listeners().GetByID(ctx, l.ID); err != storage.ErrNotFound {
		t.Fatalf("after Delete GetByID err = %v; want ErrNotFound", err)
	}
}

// Deleting the project cascades to its listeners via the FK.
func TestListenerRepo_ProjectCascade(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	proj := seedProject(t, db, "prod", "Production", admin)
	seedListener(t, db, "lis-1", proj, "0.0.0.0", 13337)

	if err := db.Projects().Delete(ctx, proj.ID); err != nil {
		t.Fatalf("Delete project: %v", err)
	}
	if _, err := db.Listeners().GetByID(ctx, "lis-1"); err != storage.ErrNotFound {
		t.Fatalf("listener row survived project delete: err=%v", err)
	}
}
