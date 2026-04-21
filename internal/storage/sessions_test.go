package storage_test

import (
	"context"
	"testing"
	"time"

	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

func seedSessionDeps(t *testing.T, db *storage.DB) (*storage.Project, *storage.Listener, *storage.Host) {
	t.Helper()
	ctx := context.Background()
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	proj := seedProject(t, db, "prod", "Production", admin)
	lis := seedListener(t, db, "lis-1", proj, "0.0.0.0", 13337)
	host, err := db.Hosts().Upsert(ctx, &storage.HostIdentity{
		ProjectID:   proj.ID,
		MachineID:   "m-abc",
		Fingerprint: "fp-x",
		Hostname:    "alpha",
		OS:          "linux",
		SeenAt:      time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("seed host: %v", err)
	}
	return proj, lis, host
}

func TestSessions_InsertAndListForHost(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	proj, lis, host := seedSessionDeps(t, db)

	now := time.Now().UTC()
	s1 := &storage.Session{
		ID: "s-1", ProjectID: proj.ID, ListenerID: lis.ID, HostID: host.ID,
		User: "root", RemoteAddr: "10.0.0.5:47212",
		ConnectedAt: now,
	}
	if err := db.Sessions().Insert(ctx, s1); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	got, err := db.Sessions().ListForHost(ctx, host.ID)
	if err != nil {
		t.Fatalf("ListForHost: %v", err)
	}
	if len(got) != 1 || got[0].ID != "s-1" {
		t.Fatalf("list: %+v", got)
	}
	if got[0].DisconnectedAt != nil {
		t.Fatalf("new session should have DisconnectedAt=nil; got %v", got[0].DisconnectedAt)
	}
}

func TestSessions_MarkDisconnected(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	proj, lis, host := seedSessionDeps(t, db)

	_ = db.Sessions().Insert(ctx, &storage.Session{
		ID: "s-1", ProjectID: proj.ID, ListenerID: lis.ID, HostID: host.ID,
		ConnectedAt: time.Now().UTC(),
	})

	if err := db.Sessions().MarkDisconnected(ctx, "s-1"); err != nil {
		t.Fatalf("MarkDisconnected: %v", err)
	}
	got, _ := db.Sessions().Get(ctx, "s-1")
	if got.DisconnectedAt == nil {
		t.Fatal("DisconnectedAt still nil after MarkDisconnected")
	}

	// Idempotent — second call is a no-op on the already-disconnected row.
	if err := db.Sessions().MarkDisconnected(ctx, "s-1"); err != nil {
		t.Fatalf("second MarkDisconnected: %v", err)
	}
}

// ListLiveForProject returns only sessions whose disconnected_at is NULL.
// This is the query the dispatch handler uses to target "currently online"
// sessions.
func TestSessions_ListLiveForProject(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	proj, lis, host := seedSessionDeps(t, db)

	_ = db.Sessions().Insert(ctx, &storage.Session{
		ID: "live", ProjectID: proj.ID, ListenerID: lis.ID, HostID: host.ID,
		ConnectedAt: time.Now().UTC(),
	})
	_ = db.Sessions().Insert(ctx, &storage.Session{
		ID: "dead", ProjectID: proj.ID, ListenerID: lis.ID, HostID: host.ID,
		ConnectedAt: time.Now().UTC(),
	})
	_ = db.Sessions().MarkDisconnected(ctx, "dead")

	live, err := db.Sessions().ListLiveForProject(ctx, proj.ID)
	if err != nil {
		t.Fatalf("ListLiveForProject: %v", err)
	}
	if len(live) != 1 || live[0].ID != "live" {
		t.Fatalf("live: %+v", live)
	}
}

// Insert enforces the alias + group_dispatch + user fields make their way
// to the DB so the hosts/sessions view can render them without a second
// lookup.
func TestSessions_FieldRoundtrip(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	proj, lis, host := seedSessionDeps(t, db)

	now := time.Now().UTC()
	orig := &storage.Session{
		ID: "s-1", ProjectID: proj.ID, ListenerID: lis.ID, HostID: host.ID,
		Alias: "box-01", User: "root", RemoteAddr: "10.0.0.5:47212",
		Version: "1.2.3", Python3: "python3", GroupDispatch: true,
		InterfacesJSON: `{"eth0":"aa:bb"}`, ConnectedAt: now,
	}
	if err := db.Sessions().Insert(ctx, orig); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	got, _ := db.Sessions().Get(ctx, "s-1")
	if got.Alias != "box-01" || got.User != "root" || got.Version != "1.2.3" ||
		got.Python3 != "python3" || !got.GroupDispatch ||
		got.InterfacesJSON != `{"eth0":"aa:bb"}` {
		t.Fatalf("roundtrip: %+v", got)
	}
}
