package storage_test

import (
	"context"
	"testing"
	"time"

	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

// TestDB_Counts_HostsAndSessions pins the contract the status-bar
// telemetry endpoint relies on. The aggregate is server-wide (across
// every project) and keeps "live" decoupled from "total" so a host
// going offline doesn't drop out of the historical counter.
//
// Live host criterion: last_seen_at within `onlineWindow` of `now`.
// Live session criterion: disconnected_at IS NULL.
func TestDB_Counts_HostsAndSessions(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	prod := seedProject(t, db, "prod", "Prod", admin)
	stage := seedProject(t, db, "stage", "Staging", admin)

	now := time.Now().UTC()

	// 3 hosts: 2 just-pinged (live), 1 stale (>2 min ago, offline).
	mustUpsertHost(t, db, &storage.HostIdentity{
		ProjectID: prod.ID, MachineID: "m-1", Fingerprint: "fp-1", SeenAt: now,
	})
	mustUpsertHost(t, db, &storage.HostIdentity{
		ProjectID: stage.ID, MachineID: "m-2", Fingerprint: "fp-2", SeenAt: now,
	})
	mustUpsertHost(t, db, &storage.HostIdentity{
		ProjectID: prod.ID, MachineID: "m-3", Fingerprint: "fp-3",
		SeenAt: now.Add(-5 * time.Minute),
	})

	// 4 sessions: 2 live (no disconnected_at), 2 closed.
	for i, p := range []struct {
		id, host, project string
		closed            bool
	}{
		{"sess-1", "m-1", prod.ID, false},
		{"sess-2", "m-1", prod.ID, true},
		{"sess-3", "m-2", stage.ID, false},
		{"sess-4", "m-3", prod.ID, true},
	} {
		s := &storage.Session{
			ID:          p.id,
			ProjectID:   p.project,
			HostID:      hostIDByMachine(t, db, p.host, p.project),
			ConnectedAt: now.Add(time.Duration(-i) * time.Second),
		}
		if err := db.Sessions().Insert(ctx, s); err != nil {
			t.Fatalf("insert session %s: %v", p.id, err)
		}
		if p.closed {
			// Insert always writes disconnected_at=NULL; flip it via
			// MarkDisconnected so the row reflects the state our
			// production handlers would record.
			if err := db.Sessions().MarkDisconnected(ctx, p.id); err != nil {
				t.Fatalf("mark disconnected %s: %v", p.id, err)
			}
		}
	}

	got, err := db.Counts(ctx, time.Minute)
	if err != nil {
		t.Fatalf("Counts: %v", err)
	}

	if got.Hosts != 3 {
		t.Errorf("Hosts = %d, want 3", got.Hosts)
	}
	if got.LiveHosts != 2 {
		t.Errorf("LiveHosts = %d, want 2", got.LiveHosts)
	}
	if got.Sessions != 4 {
		t.Errorf("Sessions = %d, want 4", got.Sessions)
	}
	if got.LiveSessions != 2 {
		t.Errorf("LiveSessions = %d, want 2", got.LiveSessions)
	}
}

func TestDB_Counts_EmptyDB(t *testing.T) {
	db := newTestDB(t)
	got, err := db.Counts(context.Background(), time.Minute)
	if err != nil {
		t.Fatalf("Counts: %v", err)
	}
	want := storage.Counts{}
	if got != want {
		t.Errorf("Counts = %+v, want %+v", got, want)
	}
}

// mustUpsertHost is a helper that fails the test instead of returning
// an error — the count tests treat host fixture creation as setup.
func mustUpsertHost(t *testing.T, db *storage.DB, ident *storage.HostIdentity) *storage.Host {
	t.Helper()
	h, err := db.Hosts().Upsert(context.Background(), ident)
	if err != nil {
		t.Fatalf("Upsert %s: %v", ident.MachineID, err)
	}
	return h
}

func hostIDByMachine(t *testing.T, db *storage.DB, machineID, projectID string) string {
	t.Helper()
	hs, err := db.Hosts().ListByProject(context.Background(), projectID)
	if err != nil {
		t.Fatalf("list hosts: %v", err)
	}
	for _, h := range hs {
		if h.MachineID == machineID {
			return h.ID
		}
	}
	t.Fatalf("host with machine_id=%s not found in project %s", machineID, projectID)
	return ""
}
