package storage_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

func TestHostRepo_Archive_HidesFromFleetList(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	proj := seedProject(t, db, "prod", "Production", admin)

	keep := seedHost(t, db, proj.ID, "keep")
	drop := seedHost(t, db, proj.ID, "drop")

	before, err := db.Hosts().ListByProject(ctx, proj.ID)
	if err != nil {
		t.Fatalf("ListByProject: %v", err)
	}
	if len(before) != 2 {
		t.Fatalf("expected 2 live hosts before archiving, got %d", len(before))
	}

	at := time.Now().UTC()
	if err := db.Hosts().Archive(ctx, drop.ID, admin.ID, "decommissioned", at); err != nil {
		t.Fatalf("Archive: %v", err)
	}

	after, err := db.Hosts().ListByProject(ctx, proj.ID)
	if err != nil {
		t.Fatalf("ListByProject after archive: %v", err)
	}
	if len(after) != 1 || after[0].ID != keep.ID {
		t.Fatalf("expected only %s to remain live, got %d rows", keep.Hostname, len(after))
	}

	// The row is hidden, not gone: a direct Get still resolves it, and
	// carries who archived it and why.
	got, err := db.Hosts().GetByID(ctx, drop.ID)
	if err != nil {
		t.Fatalf("GetByID on archived host: %v", err)
	}
	if got.ArchivedAt == nil {
		t.Error("ArchivedAt should be set on an archived host")
	}
	if got.ArchivedBy != admin.ID {
		t.Errorf("ArchivedBy = %q, want %q", got.ArchivedBy, admin.ID)
	}
	if got.ArchivedReason != "decommissioned" {
		t.Errorf("ArchivedReason = %q, want %q", got.ArchivedReason, "decommissioned")
	}
}

func TestHostRepo_Archive_DropsOutOfApprovalQueue(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	proj := seedProject(t, db, "prod", "Production", admin)

	h := seedHost(t, db, proj.ID, "pending")
	if h.ApprovalStatus != storage.HostApprovalPending {
		t.Fatalf("precondition: want a pending host, got %q", h.ApprovalStatus)
	}

	n, err := db.Hosts().CountPendingByProject(ctx, proj.ID)
	if err != nil {
		t.Fatalf("CountPendingByProject: %v", err)
	}
	if n != 1 {
		t.Fatalf("pending count = %d, want 1", n)
	}

	if err := db.Hosts().Archive(ctx, h.ID, admin.ID, "", time.Now().UTC()); err != nil {
		t.Fatalf("Archive: %v", err)
	}

	// An archived host must not keep nagging the operator through the
	// approval badge — that was half the reason to archive it.
	n, err = db.Hosts().CountPendingByProject(ctx, proj.ID)
	if err != nil {
		t.Fatalf("CountPendingByProject after archive: %v", err)
	}
	if n != 0 {
		t.Errorf("pending count = %d after archiving the only pending host, want 0", n)
	}
	pending, err := db.Hosts().ListPendingByProject(ctx, proj.ID)
	if err != nil {
		t.Fatalf("ListPendingByProject: %v", err)
	}
	if len(pending) != 0 {
		t.Errorf("approval queue still lists %d archived host(s)", len(pending))
	}
}

func TestHostRepo_Restore(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	proj := seedProject(t, db, "prod", "Production", admin)
	h := seedHost(t, db, proj.ID, "back")

	if err := db.Hosts().Archive(ctx, h.ID, admin.ID, "oops", time.Now().UTC()); err != nil {
		t.Fatalf("Archive: %v", err)
	}
	if err := db.Hosts().Restore(ctx, h.ID); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	live, err := db.Hosts().ListByProject(ctx, proj.ID)
	if err != nil {
		t.Fatalf("ListByProject: %v", err)
	}
	if len(live) != 1 {
		t.Fatalf("restored host missing from fleet list (%d rows)", len(live))
	}
	// Metadata is cleared, so a later archive records its own actor
	// and reason rather than inheriting this one's.
	if live[0].ArchivedAt != nil || live[0].ArchivedBy != "" || live[0].ArchivedReason != "" {
		t.Errorf("archive metadata survived Restore: at=%v by=%q reason=%q",
			live[0].ArchivedAt, live[0].ArchivedBy, live[0].ArchivedReason)
	}
}

func TestHostRepo_Archive_Idempotence(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	proj := seedProject(t, db, "prod", "Production", admin)
	h := seedHost(t, db, proj.ID, "twice")

	at := time.Now().UTC()
	if err := db.Hosts().Archive(ctx, h.ID, admin.ID, "first", at); err != nil {
		t.Fatalf("first Archive: %v", err)
	}
	// Archiving again must not overwrite the original actor/reason —
	// the first decision is the one worth keeping.
	if err := db.Hosts().Archive(ctx, h.ID, "someone-else", "second", at); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("second Archive err = %v, want ErrNotFound", err)
	}
	got, err := db.Hosts().GetByID(ctx, h.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.ArchivedReason != "first" || got.ArchivedBy != admin.ID {
		t.Errorf("archive metadata was overwritten: by=%q reason=%q", got.ArchivedBy, got.ArchivedReason)
	}

	// Restoring something already live is likewise a no-op error.
	if err := db.Hosts().Restore(ctx, h.ID); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if err := db.Hosts().Restore(ctx, h.ID); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("second Restore err = %v, want ErrNotFound", err)
	}
}

// An archived host whose agent reconnects comes back rather than
// enrolling as a second row — otherwise the fleet gains a duplicate
// with none of the original's history.
func TestHostRepo_Upsert_ReenrolmentUnarchives(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	proj := seedProject(t, db, "prod", "Production", admin)
	h := seedHost(t, db, proj.ID, "returning")

	if err := db.Hosts().Archive(ctx, h.ID, admin.ID, "thought it was gone", time.Now().UTC()); err != nil {
		t.Fatalf("Archive: %v", err)
	}

	again, err := db.Hosts().Upsert(ctx, &storage.HostIdentity{
		ProjectID:   proj.ID,
		MachineID:   "returning",
		Fingerprint: "fp-returning",
		Hostname:    "host-returning",
		SeenAt:      time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("re-Upsert: %v", err)
	}
	if again.ID != h.ID {
		t.Fatalf("re-enrolment created a new row %s, want the existing %s", again.ID, h.ID)
	}
	if again.ArchivedAt != nil {
		t.Error("re-enrolled host is still archived; a live agent would stay invisible")
	}

	live, err := db.Hosts().ListByProject(ctx, proj.ID)
	if err != nil {
		t.Fatalf("ListByProject: %v", err)
	}
	if len(live) != 1 {
		t.Fatalf("fleet list has %d rows after re-enrolment, want exactly 1", len(live))
	}
}

func TestHostRepo_ListArchivedByProject(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	proj := seedProject(t, db, "prod", "Production", admin)

	live := seedHost(t, db, proj.ID, "live")
	gone := seedHost(t, db, proj.ID, "gone")
	if err := db.Hosts().Archive(ctx, gone.ID, admin.ID, "retired", time.Now().UTC()); err != nil {
		t.Fatalf("Archive: %v", err)
	}

	archived, err := db.Hosts().ListArchivedByProject(ctx, proj.ID)
	if err != nil {
		t.Fatalf("ListArchivedByProject: %v", err)
	}
	if len(archived) != 1 || archived[0].ID != gone.ID {
		t.Fatalf("archived listing = %d rows, want just %s", len(archived), gone.Hostname)
	}
	if archived[0].ArchivedReason != "retired" {
		t.Errorf("ArchivedReason = %q, want %q", archived[0].ArchivedReason, "retired")
	}
	_ = live
}
