package storage_test

import (
	"context"
	"testing"
	"time"

	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

func seedSession(t *testing.T, db *storage.DB, id, agentID, projectID, reason string) *storage.AgentSession {
	t.Helper()
	s := &storage.AgentSession{
		SessionID:        id,
		AgentID:          agentID,
		ProjectID:        projectID,
		SessionTokenHash: []byte("h-" + id),
		IssuedAt:         time.Now().UTC(),
		IssuedReason:     reason,
		ExpiresAt:        time.Now().Add(24 * time.Hour).UTC(),
	}
	if err := db.AgentSessions().InsertActive(context.Background(), s); err != nil {
		t.Fatalf("InsertActive(%s): %v", id, err)
	}
	return s
}

func TestAgentSessions_InsertAndGetActive(t *testing.T) {
	db := newTestDB(t)
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	proj := seedProject(t, db, "p1", "Project 1", admin)
	seedSession(t, db, "sess-1", "agent-a", proj.ID, "enroll")

	got, err := db.AgentSessions().GetActive(context.Background(), "agent-a")
	if err != nil {
		t.Fatalf("GetActive: %v", err)
	}
	if got.SessionID != "sess-1" {
		t.Fatalf("SessionID = %q; want sess-1", got.SessionID)
	}
	if !got.IsActive(time.Now()) {
		t.Fatal("expected IsActive=true")
	}
}

func TestAgentSessions_TwoActiveRejected(t *testing.T) {
	db := newTestDB(t)
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	proj := seedProject(t, db, "p1", "Project 1", admin)
	seedSession(t, db, "sess-1", "agent-a", proj.ID, "enroll")

	dup := &storage.AgentSession{
		SessionID:        "sess-2",
		AgentID:          "agent-a",
		ProjectID:        proj.ID,
		SessionTokenHash: []byte("h-2"),
		IssuedAt:         time.Now().UTC(),
		IssuedReason:     "enroll",
		ExpiresAt:        time.Now().Add(24 * time.Hour).UTC(),
	}
	err := db.AgentSessions().InsertActive(context.Background(), dup)
	if err != storage.ErrSessionAlreadyActive {
		t.Fatalf("err = %v; want ErrSessionAlreadyActive", err)
	}
}

func TestAgentSessions_RotateToLineage(t *testing.T) {
	db := newTestDB(t)
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	proj := seedProject(t, db, "p1", "Project 1", admin)
	seedSession(t, db, "sess-1", "agent-a", proj.ID, "enroll")

	next := &storage.AgentSession{
		SessionID:        "sess-2",
		AgentID:          "agent-a",
		ProjectID:        proj.ID,
		SessionTokenHash: []byte("h-2"),
		IssuedAt:         time.Now().UTC(),
		IssuedReason:     "rotation",
		ExpiresAt:        time.Now().Add(24 * time.Hour).UTC(),
	}
	if err := db.AgentSessions().RotateTo(context.Background(), "sess-1", next, time.Now()); err != nil {
		t.Fatalf("RotateTo: %v", err)
	}

	ctx := context.Background()
	active, err := db.AgentSessions().GetActive(ctx, "agent-a")
	if err != nil {
		t.Fatalf("GetActive: %v", err)
	}
	if active.SessionID != "sess-2" {
		t.Fatalf("active = %q; want sess-2", active.SessionID)
	}
	if active.RotatedFrom != "sess-1" {
		t.Fatalf("RotatedFrom = %q; want sess-1", active.RotatedFrom)
	}

	// Old row must still exist but marked rotated.
	old, err := db.AgentSessions().GetBySessionID(ctx, "sess-1")
	if err != nil {
		t.Fatalf("GetBySessionID: %v", err)
	}
	if old.RotatedAt == nil {
		t.Fatal("old session RotatedAt still nil")
	}

	// History contains both.
	hist, err := db.AgentSessions().History(ctx, "agent-a")
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(hist) != 2 {
		t.Fatalf("len(history) = %d; want 2", len(hist))
	}
}

func TestAgentSessions_RotateTo_AlreadyRotated(t *testing.T) {
	db := newTestDB(t)
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	proj := seedProject(t, db, "p1", "Project 1", admin)
	seedSession(t, db, "sess-1", "agent-a", proj.ID, "enroll")
	ctx := context.Background()

	next := &storage.AgentSession{
		SessionID:        "sess-2",
		AgentID:          "agent-a",
		ProjectID:        proj.ID,
		SessionTokenHash: []byte("h-2"),
		IssuedAt:         time.Now().UTC(),
		IssuedReason:     "rotation",
		ExpiresAt:        time.Now().Add(24 * time.Hour).UTC(),
	}
	if err := db.AgentSessions().RotateTo(ctx, "sess-1", next, time.Now()); err != nil {
		t.Fatalf("first rotate: %v", err)
	}

	// Rotating sess-1 again must fail — it's already dead.
	third := &storage.AgentSession{
		SessionID:        "sess-3",
		AgentID:          "agent-a",
		ProjectID:        proj.ID,
		SessionTokenHash: []byte("h-3"),
		IssuedAt:         time.Now().UTC(),
		IssuedReason:     "rotation",
		ExpiresAt:        time.Now().Add(24 * time.Hour).UTC(),
	}
	err := db.AgentSessions().RotateTo(ctx, "sess-1", third, time.Now())
	if err != storage.ErrNotFound {
		t.Fatalf("err = %v; want ErrNotFound", err)
	}
}

func TestAgentSessions_RevokeActive(t *testing.T) {
	db := newTestDB(t)
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	proj := seedProject(t, db, "p1", "Project 1", admin)
	seedSession(t, db, "sess-1", "agent-a", proj.ID, "enroll")
	ctx := context.Background()

	if err := db.AgentSessions().RevokeActive(ctx, "agent-a", admin.ID, "lost laptop", time.Now()); err != nil {
		t.Fatalf("RevokeActive: %v", err)
	}
	_, err := db.AgentSessions().GetActive(ctx, "agent-a")
	if err != storage.ErrNotFound {
		t.Fatalf("err = %v; want ErrNotFound", err)
	}

	// Row must still exist in history with revoked_at set.
	old, err := db.AgentSessions().GetBySessionID(ctx, "sess-1")
	if err != nil {
		t.Fatalf("GetBySessionID: %v", err)
	}
	if old.RevokedAt == nil {
		t.Fatal("RevokedAt still nil after RevokeActive")
	}

	// After revoke, inserting a new active session should succeed (the
	// partial unique index counts only active rows).
	fresh := &storage.AgentSession{
		SessionID:        "sess-2",
		AgentID:          "agent-a",
		ProjectID:        proj.ID,
		SessionTokenHash: []byte("h-2"),
		IssuedAt:         time.Now().UTC(),
		IssuedReason:     "reset",
		ExpiresAt:        time.Now().Add(24 * time.Hour).UTC(),
	}
	if err := db.AgentSessions().InsertActive(ctx, fresh); err != nil {
		t.Fatalf("InsertActive after revoke: %v", err)
	}
}
