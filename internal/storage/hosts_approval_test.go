package storage_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

// TestHostRepo_Upsert_DefaultApprovalIsPending pins the safe default:
// a fresh enrollment that doesn't explicitly set InitialApproval lands
// in `pending`. This is the default that protects against PAT leaks
// closing the loop without admin attention.
func TestHostRepo_Upsert_DefaultApprovalIsPending(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	proj := seedProject(t, db, "prod", "Production", admin)

	h, err := db.Hosts().Upsert(ctx, &storage.HostIdentity{
		ProjectID:   proj.ID,
		MachineID:   "m-1",
		Fingerprint: "fp-1",
		Hostname:    "host-1",
		SeenAt:      time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if h.ApprovalStatus != storage.HostApprovalPending {
		t.Fatalf("ApprovalStatus = %q, want %q", h.ApprovalStatus, storage.HostApprovalPending)
	}
	if h.ApprovalDecidedAt != nil {
		t.Fatalf("ApprovalDecidedAt should be nil before any decision, got %v", h.ApprovalDecidedAt)
	}
}

// TestHostRepo_Upsert_AutoApproveStampsDecision verifies the
// auto_approve PAT path: caller passes InitialApproval=Approved, the
// row lands in `approved` with `system:auto-approve` as the decided_by
// so the audit trail still shows who-when even when the decision was
// automatic.
func TestHostRepo_Upsert_AutoApproveStampsDecision(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	proj := seedProject(t, db, "prod", "Production", admin)

	now := time.Now().UTC()
	h, err := db.Hosts().Upsert(ctx, &storage.HostIdentity{
		ProjectID:       proj.ID,
		MachineID:       "m-2",
		Fingerprint:     "fp-2",
		Hostname:        "host-2",
		SeenAt:          now,
		InitialApproval: storage.HostApprovalApproved,
	})
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if h.ApprovalStatus != storage.HostApprovalApproved {
		t.Fatalf("ApprovalStatus = %q, want approved", h.ApprovalStatus)
	}
	if h.ApprovalDecidedAt == nil {
		t.Fatalf("ApprovalDecidedAt should be stamped on auto-approve")
	}
	if h.ApprovalDecidedBy != "system:auto-approve" {
		t.Fatalf("ApprovalDecidedBy = %q, want system:auto-approve", h.ApprovalDecidedBy)
	}
}

// TestHostRepo_Approve_FlipsStatusAndStampsDecision exercises the
// happy path: pending → approved with reason persisted.
func TestHostRepo_Approve_FlipsStatusAndStampsDecision(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	proj := seedProject(t, db, "prod", "Production", admin)

	h, err := db.Hosts().Upsert(ctx, &storage.HostIdentity{
		ProjectID: proj.ID, MachineID: "m-3", Fingerprint: "fp-3",
		Hostname: "host-3", SeenAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	now := time.Now().UTC()
	if err := db.Hosts().Approve(ctx, h.ID, admin.ID, "expected", now); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	got, err := db.Hosts().GetByID(ctx, h.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.ApprovalStatus != storage.HostApprovalApproved {
		t.Fatalf("status = %q, want approved", got.ApprovalStatus)
	}
	if got.ApprovalDecidedBy != admin.ID {
		t.Fatalf("decided_by = %q, want %q", got.ApprovalDecidedBy, admin.ID)
	}
	if got.ApprovalReason != "expected" {
		t.Fatalf("reason = %q, want 'expected'", got.ApprovalReason)
	}
}

// TestHostRepo_Reject_FlipsStatus mirrors Approve. The `pending →
// rejected → approved` loop is supported (admin changes their mind).
func TestHostRepo_Reject_FlipsStatus(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	proj := seedProject(t, db, "prod", "Production", admin)

	h, err := db.Hosts().Upsert(ctx, &storage.HostIdentity{
		ProjectID: proj.ID, MachineID: "m-4", Fingerprint: "fp-4",
		Hostname: "host-4", SeenAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	now := time.Now().UTC()
	if err := db.Hosts().Reject(ctx, h.ID, admin.ID, "wrong host", now); err != nil {
		t.Fatalf("Reject: %v", err)
	}
	got, err := db.Hosts().GetByID(ctx, h.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.ApprovalStatus != storage.HostApprovalRejected {
		t.Fatalf("status = %q, want rejected", got.ApprovalStatus)
	}

	// Admin can flip back to approved.
	if err := db.Hosts().Approve(ctx, h.ID, admin.ID, "actually fine", time.Now().UTC()); err != nil {
		t.Fatalf("Approve after Reject: %v", err)
	}
	got, _ = db.Hosts().GetByID(ctx, h.ID)
	if got.ApprovalStatus != storage.HostApprovalApproved {
		t.Fatalf("status = %q after re-approve, want approved", got.ApprovalStatus)
	}
}

// TestHostRepo_Approve_NotFound — calling Approve on a missing id
// returns ErrNotFound so the API handler can map to 404.
func TestHostRepo_Approve_NotFound(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	if err := db.Hosts().Approve(ctx, "no-such-host", "u", "", time.Now().UTC()); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// TestHostRepo_ListPendingByProject scopes per-project and returns
// only `pending` rows, oldest first.
func TestHostRepo_ListPendingByProject(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	prod := seedProject(t, db, "prod", "Production", admin)
	stag := seedProject(t, db, "staging", "Staging", admin)

	// Two pending hosts in prod, one approved (should not appear),
	// one pending in staging (should not leak across projects).
	mk := func(proj string, m string, ts time.Time, approval storage.HostApprovalStatus) {
		_, err := db.Hosts().Upsert(ctx, &storage.HostIdentity{
			ProjectID: proj, MachineID: m, Fingerprint: "fp-" + m,
			Hostname: m, SeenAt: ts, InitialApproval: approval,
		})
		if err != nil {
			t.Fatalf("Upsert(%s, %s): %v", proj, m, err)
		}
	}
	now := time.Now().UTC()
	mk(prod.ID, "p1", now.Add(-2*time.Hour), storage.HostApprovalPending)
	mk(prod.ID, "p2", now.Add(-1*time.Hour), storage.HostApprovalPending)
	mk(prod.ID, "p3", now.Add(-30*time.Minute), storage.HostApprovalApproved)
	mk(stag.ID, "s1", now.Add(-10*time.Minute), storage.HostApprovalPending)

	got, err := db.Hosts().ListPendingByProject(ctx, prod.ID)
	if err != nil {
		t.Fatalf("ListPendingByProject: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].MachineID != "p1" || got[1].MachineID != "p2" {
		t.Fatalf("expected p1, p2 (oldest-first); got %s, %s",
			got[0].MachineID, got[1].MachineID)
	}

	n, err := db.Hosts().CountPendingByProject(ctx, prod.ID)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if n != 2 {
		t.Fatalf("count = %d, want 2", n)
	}
}

// TestHostRepo_Upsert_ApprovalNotClobberedOnUpdate: once a host is
// approved, a subsequent Upsert (e.g. agent reconnect refreshing
// SysInfo) must NOT reset approval_status back to pending.
func TestHostRepo_Upsert_ApprovalNotClobberedOnUpdate(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	proj := seedProject(t, db, "prod", "Production", admin)

	h, _ := db.Hosts().Upsert(ctx, &storage.HostIdentity{
		ProjectID: proj.ID, MachineID: "m-5", Fingerprint: "fp-5",
		Hostname: "host-5", SeenAt: time.Now().UTC(),
	})
	if err := db.Hosts().Approve(ctx, h.ID, admin.ID, "", time.Now().UTC()); err != nil {
		t.Fatalf("Approve: %v", err)
	}

	// Reconnect: caller doesn't set InitialApproval (or even sets
	// pending) — Upsert must not regress the field.
	h2, err := db.Hosts().Upsert(ctx, &storage.HostIdentity{
		ProjectID: proj.ID, MachineID: "m-5", Fingerprint: "fp-5",
		Hostname:        "host-5",
		SeenAt:          time.Now().UTC(),
		InitialApproval: storage.HostApprovalPending,
	})
	if err != nil {
		t.Fatalf("Upsert (reconnect): %v", err)
	}
	if h2.ApprovalStatus != storage.HostApprovalApproved {
		t.Fatalf("approval was clobbered by reconnect-time Upsert: got %q", h2.ApprovalStatus)
	}
}
