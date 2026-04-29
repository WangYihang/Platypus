package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

// TestHosts_PendingList_FiltersByApprovalStatus seeds three hosts in
// the project (one approved, two pending) and confirms the pending
// endpoint returns only the two pending rows in oldest-first order.
func TestHosts_PendingList_FiltersByApprovalStatus(t *testing.T) {
	r, db := hostsTestSetup(t)
	admin := seedUserForAPITest(t, db, "admin", user.RoleAdmin)
	proj := seedProjectForAPITest(t, db, "prod", admin)

	now := time.Now().UTC()
	ctx := context.Background()
	_, _ = db.Hosts().Upsert(ctx, &storage.HostIdentity{
		ProjectID: proj.ID, MachineID: "m-old", Fingerprint: "fp-old",
		Hostname: "older-pending", SeenAt: now.Add(-2 * time.Hour),
		InitialApproval: storage.HostApprovalPending,
	})
	_, _ = db.Hosts().Upsert(ctx, &storage.HostIdentity{
		ProjectID: proj.ID, MachineID: "m-new", Fingerprint: "fp-new",
		Hostname: "newer-pending", SeenAt: now.Add(-1 * time.Hour),
		InitialApproval: storage.HostApprovalPending,
	})
	_, _ = db.Hosts().Upsert(ctx, &storage.HostIdentity{
		ProjectID: proj.ID, MachineID: "m-ok", Fingerprint: "fp-ok",
		Hostname: "approved-host", SeenAt: now,
		InitialApproval: storage.HostApprovalApproved,
	})

	tok := mintBearerForUserID(t, db, admin.ID, user.RoleAdmin)
	w := probeReqWithPath(r, "GET", "/api/v1/projects/"+proj.ID+"/hosts/pending", tok, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Hosts []hostResponse `json:"hosts"`
	}
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Hosts) != 2 {
		t.Fatalf("len=%d, want 2 — body=%s", len(resp.Hosts), w.Body.String())
	}
	if resp.Hosts[0].Hostname != "older-pending" || resp.Hosts[1].Hostname != "newer-pending" {
		t.Fatalf("ordering: %+v", resp.Hosts)
	}

	// Count endpoint mirrors the list count.
	w2 := probeReqWithPath(r, "GET", "/api/v1/projects/"+proj.ID+"/hosts/pending/count", tok, nil)
	if w2.Code != http.StatusOK {
		t.Fatalf("count status=%d", w2.Code)
	}
	var cresp struct {
		Pending int `json:"pending"`
	}
	_ = json.NewDecoder(w2.Body).Decode(&cresp)
	if cresp.Pending != 2 {
		t.Fatalf("count=%d, want 2", cresp.Pending)
	}
}

// TestHosts_Approve_RequiresAdmin pins the RBAC layer: viewer-tier
// users get 403, admin-tier flips the row.
func TestHosts_Approve_RequiresAdmin(t *testing.T) {
	r, db := hostsTestSetup(t)
	admin := seedUserForAPITest(t, db, "admin", user.RoleAdmin)
	viewer := seedUserForAPITest(t, db, "viewer", user.RoleViewer)
	proj := seedProjectForAPITest(t, db, "prod", admin)

	h, _ := db.Hosts().Upsert(context.Background(), &storage.HostIdentity{
		ProjectID: proj.ID, MachineID: "m-1", Fingerprint: "fp-1",
		Hostname: "h1", SeenAt: time.Now().UTC(),
	})

	// Viewer cannot approve.
	viewerTok := mintBearerForUserID(t, db, viewer.ID, user.RoleViewer)
	w := probeReqWithPath(r, "POST",
		"/api/v1/projects/"+proj.ID+"/hosts/"+h.ID+"/approve",
		viewerTok, map[string]string{"reason": "fine"})
	if w.Code != http.StatusForbidden {
		t.Fatalf("viewer approve status=%d, want 403", w.Code)
	}

	// Admin can.
	adminTok := mintBearerForUserID(t, db, admin.ID, user.RoleAdmin)
	w = probeReqWithPath(r, "POST",
		"/api/v1/projects/"+proj.ID+"/hosts/"+h.ID+"/approve",
		adminTok, map[string]string{"reason": "kim's laptop"})
	if w.Code != http.StatusOK {
		t.Fatalf("admin approve status=%d body=%s", w.Code, w.Body.String())
	}

	// State persisted.
	got, _ := db.Hosts().GetByID(context.Background(), h.ID)
	if got.ApprovalStatus != storage.HostApprovalApproved {
		t.Fatalf("status = %q, want approved", got.ApprovalStatus)
	}
	if got.ApprovalDecidedBy != admin.ID {
		t.Fatalf("decided_by = %q, want %q", got.ApprovalDecidedBy, admin.ID)
	}
	if got.ApprovalReason != "kim's laptop" {
		t.Fatalf("reason = %q", got.ApprovalReason)
	}
}

// TestHosts_Reject_PersistsAndReturnsOK is the symmetric Reject case.
func TestHosts_Reject_PersistsAndReturnsOK(t *testing.T) {
	r, db := hostsTestSetup(t)
	admin := seedUserForAPITest(t, db, "admin", user.RoleAdmin)
	proj := seedProjectForAPITest(t, db, "prod", admin)

	h, _ := db.Hosts().Upsert(context.Background(), &storage.HostIdentity{
		ProjectID: proj.ID, MachineID: "m-r", Fingerprint: "fp-r",
		Hostname: "rejected-host", SeenAt: time.Now().UTC(),
	})

	tok := mintBearerForUserID(t, db, admin.ID, user.RoleAdmin)
	w := probeReqWithPath(r, "POST",
		"/api/v1/projects/"+proj.ID+"/hosts/"+h.ID+"/reject",
		tok, map[string]string{"reason": "wrong host"})
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	got, _ := db.Hosts().GetByID(context.Background(), h.ID)
	if got.ApprovalStatus != storage.HostApprovalRejected {
		t.Fatalf("status=%q, want rejected", got.ApprovalStatus)
	}
}

// TestHosts_Approve_NotFound: missing host_id → 404.
func TestHosts_Approve_NotFound(t *testing.T) {
	r, db := hostsTestSetup(t)
	admin := seedUserForAPITest(t, db, "admin", user.RoleAdmin)
	proj := seedProjectForAPITest(t, db, "prod", admin)

	tok := mintBearerForUserID(t, db, admin.ID, user.RoleAdmin)
	w := probeReqWithPath(r, "POST",
		"/api/v1/projects/"+proj.ID+"/hosts/no-such-id/approve",
		tok, map[string]string{})
	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d, want 404", w.Code)
	}
}
