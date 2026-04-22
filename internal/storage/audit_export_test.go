package storage_test

import (
	"context"
	"testing"
	"time"

	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

// Time-range filter on pat_redemption_events — rows outside [From, To]
// must be excluded, and project scoping must filter out events whose
// token_id belongs to a different project.
func TestListInRange_PATRedemptionEvents_FilterAndScope(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	proj1 := seedProject(t, db, "p1", "P1", admin)
	proj2 := seedProject(t, db, "p2", "P2", admin)

	// Seed PATs in two different projects, then events against each.
	seedPAT(t, db, "plt_p1", proj1.ID, admin.ID, []byte("s"), time.Hour, 1)
	seedPAT(t, db, "plt_p2", proj2.ID, admin.ID, []byte("s"), time.Hour, 1)

	base := time.Now().UTC()
	events := []*storage.PATRedemptionEvent{
		{At: base.Add(-2 * time.Hour), TokenID: "plt_p1", Outcome: "success"},
		{At: base.Add(-1 * time.Hour), TokenID: "plt_p2", Outcome: "success"},
		{At: base, TokenID: "plt_p1", Outcome: "invalid_secret"},
		// Scanning attempt — no matching pat_tokens row at all.
		{At: base, TokenID: "plt_ghost", Outcome: "unknown_token"},
	}
	for _, e := range events {
		if err := db.PATRedemptionEvents().Record(ctx, e); err != nil {
			t.Fatalf("Record: %v", err)
		}
	}

	t.Run("all_projects_within_window", func(t *testing.T) {
		got, err := db.PATRedemptionEvents().ListInRange(ctx, storage.AuditExportFilter{
			From: base.Add(-90 * time.Minute),
			To:   base.Add(time.Minute),
		})
		if err != nil {
			t.Fatalf("ListInRange: %v", err)
		}
		if len(got) != 3 {
			t.Fatalf("len = %d; want 3 (excluded the -2h event)", len(got))
		}
	})

	t.Run("project_scoped", func(t *testing.T) {
		got, err := db.PATRedemptionEvents().ListInRange(ctx, storage.AuditExportFilter{
			ProjectID: proj1.ID,
		})
		if err != nil {
			t.Fatalf("ListInRange: %v", err)
		}
		// Only plt_p1's events; the ghost attempt is dropped because it
		// can't be scoped without a project.
		if len(got) != 2 {
			t.Fatalf("len = %d; want 2", len(got))
		}
		for _, e := range got {
			if e.TokenID != "plt_p1" {
				t.Errorf("unexpected token_id %q", e.TokenID)
			}
		}
	})
}

func TestListInRange_AdminAuditLog_ProjectFilter(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	base := time.Now().UTC()
	for i, pid := range []string{"p1", "p1", "p2", ""} {
		if err := db.AdminAuditLog().Record(ctx, &storage.AdminAuditEvent{
			At:        base.Add(time.Duration(i) * time.Minute),
			ActorUser: admin.ID,
			Action:    "pat.issue",
			ProjectID: pid,
			Outcome:   "success",
		}); err != nil {
			t.Fatalf("Record: %v", err)
		}
	}

	// Project-scoped export returns only p1 rows.
	got, err := db.AdminAuditLog().ListInRange(ctx, storage.AuditExportFilter{ProjectID: "p1"})
	if err != nil {
		t.Fatalf("ListInRange: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d; want 2", len(got))
	}

	// Global export (no project filter) returns all four.
	all, _ := db.AdminAuditLog().ListInRange(ctx, storage.AuditExportFilter{})
	if len(all) != 4 {
		t.Fatalf("len global = %d; want 4", len(all))
	}
}

func TestListInRange_ConnectionEvents_TimeFilter(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	base := time.Now().UTC()
	for i, at := range []time.Time{base.Add(-time.Hour), base, base.Add(time.Hour)} {
		if err := db.AgentConnectionEvents().Record(ctx, &storage.AgentConnectionEvent{
			At:        at,
			AgentID:   "a1",
			EventType: "reconnect_success",
			Transport: "tls_direct",
		}); err != nil {
			t.Fatalf("Record[%d]: %v", i, err)
		}
	}

	// Only the middle event is within a tight window.
	got, err := db.AgentConnectionEvents().ListInRange(ctx, storage.AuditExportFilter{
		From: base.Add(-5 * time.Minute),
		To:   base.Add(5 * time.Minute),
	})
	if err != nil {
		t.Fatalf("ListInRange: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d; want 1", len(got))
	}
}
