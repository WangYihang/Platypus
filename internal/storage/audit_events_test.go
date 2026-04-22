package storage_test

import (
	"context"
	"testing"
	"time"

	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

func TestPATRedemptionEvents_RecordAndList(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	events := []*storage.PATRedemptionEvent{
		{At: time.Now().UTC(), TokenID: "plt_a", Outcome: "success", ClientIP: "1.2.3.4", AgentID: "agent-1"},
		{At: time.Now().UTC(), TokenID: "plt_a", Outcome: "max_uses_reached", ClientIP: "1.2.3.5"},
		{At: time.Now().UTC(), TokenID: "plt_b", Outcome: "invalid_secret", ClientIP: "1.2.3.4"},
		// Recording an event against a token_id that doesn't exist in
		// pat_tokens is intentional: scanning/brute-force attempts.
		{At: time.Now().UTC(), TokenID: "plt_fake", Outcome: "unknown_token", ClientIP: "attacker"},
	}
	for _, e := range events {
		if err := db.PATRedemptionEvents().Record(ctx, e); err != nil {
			t.Fatalf("Record: %v", err)
		}
	}

	got, err := db.PATRedemptionEvents().ListByToken(ctx, "plt_a", 10)
	if err != nil {
		t.Fatalf("ListByToken: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d; want 2", len(got))
	}

	// "unknown_token" outcome must have been accepted (no FK rejection).
	fake, err := db.PATRedemptionEvents().ListByToken(ctx, "plt_fake", 10)
	if err != nil {
		t.Fatalf("ListByToken plt_fake: %v", err)
	}
	if len(fake) != 1 || fake[0].Outcome != "unknown_token" {
		t.Fatalf("fake events = %+v; expected 1 unknown_token", fake)
	}
}

func TestPATRedemptionEvents_RejectsInvalidOutcome(t *testing.T) {
	db := newTestDB(t)
	err := db.PATRedemptionEvents().Record(context.Background(), &storage.PATRedemptionEvent{
		At: time.Now().UTC(), TokenID: "plt_x", Outcome: "not_a_real_outcome",
	})
	if err == nil {
		t.Fatal("expected CHECK violation for invalid outcome")
	}
}

func TestAgentConnectionEvents_RecordAndList(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	evts := []*storage.AgentConnectionEvent{
		{At: time.Now().UTC(), AgentID: "agent-a", EventType: "enroll_success", ClientIP: "1.1.1.1", Transport: "tls_direct"},
		{At: time.Now().UTC(), AgentID: "agent-a", EventType: "reconnect_success", SessionID: "sess-1", Transport: "tls_direct"},
		{At: time.Now().UTC(), AgentID: "agent-a", EventType: "disconnect", SessionID: "sess-1", Reason: "client closed"},
	}
	for _, e := range evts {
		if err := db.AgentConnectionEvents().Record(ctx, e); err != nil {
			t.Fatalf("Record: %v", err)
		}
	}

	got, err := db.AgentConnectionEvents().ListByAgent(ctx, "agent-a", 10)
	if err != nil {
		t.Fatalf("ListByAgent: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len = %d; want 3", len(got))
	}
}

func TestAdminAuditLog_RecordRequiresActor(t *testing.T) {
	db := newTestDB(t)
	err := db.AdminAuditLog().Record(context.Background(), &storage.AdminAuditEvent{
		At:      time.Now().UTC(),
		Action:  "pat.issue",
		Outcome: "success",
	})
	if err == nil {
		t.Fatal("expected missing-actor error")
	}
}

func TestAdminAuditLog_RecentOrdering(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	admin := seedUser(t, db, "admin", user.RoleAdmin)

	actions := []string{"pat.issue", "pat.revoke", "session.revoke"}
	base := time.Now().UTC()
	for i, a := range actions {
		if err := db.AdminAuditLog().Record(ctx, &storage.AdminAuditEvent{
			At:        base.Add(time.Duration(i) * time.Minute),
			ActorUser: admin.ID,
			Action:    a,
			Outcome:   "success",
		}); err != nil {
			t.Fatalf("Record %s: %v", a, err)
		}
	}

	got, err := db.AdminAuditLog().ListRecent(ctx, 10)
	if err != nil {
		t.Fatalf("ListRecent: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len = %d; want 3", len(got))
	}
	// Newest first: the last Insert (session.revoke) must come first.
	if got[0].Action != "session.revoke" {
		t.Fatalf("newest action = %q; want session.revoke", got[0].Action)
	}
}
