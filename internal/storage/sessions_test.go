package storage_test

import (
	"context"
	"testing"
	"time"

	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

const testIngressAddr = "0.0.0.0:9443"

func seedSessionDeps(t *testing.T, db *storage.DB) (*storage.Project, *storage.Host) {
	t.Helper()
	ctx := context.Background()
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	proj := seedProject(t, db, "prod", "Production", admin)
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
	return proj, host
}

func TestSessions_InsertAndListForHost(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	proj, host := seedSessionDeps(t, db)

	now := time.Now().UTC()
	s1 := &storage.Session{
		ID: "s-1", ProjectID: proj.ID, IngressAddr: testIngressAddr, HostID: host.ID,
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
	if got[0].IngressAddr != testIngressAddr {
		t.Fatalf("IngressAddr roundtrip: got %q want %q", got[0].IngressAddr, testIngressAddr)
	}
	if got[0].DisconnectedAt != nil {
		t.Fatalf("new session should have DisconnectedAt=nil; got %v", got[0].DisconnectedAt)
	}
}

func TestSessions_MarkDisconnected(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	proj, host := seedSessionDeps(t, db)

	_ = db.Sessions().Insert(ctx, &storage.Session{
		ID: "s-1", ProjectID: proj.ID, IngressAddr: testIngressAddr, HostID: host.ID,
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
	proj, host := seedSessionDeps(t, db)

	_ = db.Sessions().Insert(ctx, &storage.Session{
		ID: "live", ProjectID: proj.ID, IngressAddr: testIngressAddr, HostID: host.ID,
		ConnectedAt: time.Now().UTC(),
	})
	_ = db.Sessions().Insert(ctx, &storage.Session{
		ID: "dead", ProjectID: proj.ID, IngressAddr: testIngressAddr, HostID: host.ID,
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

// ListForProject is the project-wide query that backs the dashboard
// time-series and the SessionsPage. Verifies all three optional filters
// (Live, Since, Limit) do what their names say.
func TestSessions_ListForProject(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	proj, host := seedSessionDeps(t, db)

	// Three sessions: one closed two days ago, one live an hour ago, one
	// live ten minutes ago. The 2-day-old one should fall out of a "since
	// 24h" filter; the closed one should fall out of a Live=true filter.
	now := time.Now().UTC()
	insert := func(id string, connected time.Time, alive bool) {
		t.Helper()
		if err := db.Sessions().Insert(ctx, &storage.Session{
			ID: id, ProjectID: proj.ID, IngressAddr: testIngressAddr, HostID: host.ID,
			ConnectedAt: connected,
		}); err != nil {
			t.Fatalf("Insert %s: %v", id, err)
		}
		if !alive {
			if err := db.Sessions().MarkDisconnected(ctx, id); err != nil {
				t.Fatalf("MarkDisconnected %s: %v", id, err)
			}
		}
	}
	insert("old-closed", now.Add(-48*time.Hour), false)
	insert("recent-live", now.Add(-1*time.Hour), true)
	insert("newest-live", now.Add(-10*time.Minute), true)

	// No filters: all three rows, newest first.
	all, err := db.Sessions().ListForProject(ctx, proj.ID, storage.SessionListOpts{})
	if err != nil {
		t.Fatalf("ListForProject: %v", err)
	}
	if len(all) != 3 ||
		all[0].ID != "newest-live" ||
		all[1].ID != "recent-live" ||
		all[2].ID != "old-closed" {
		t.Fatalf("expected newest first; got %+v", ids(all))
	}

	// Live=true drops the closed one.
	live := true
	liveOnly, err := db.Sessions().ListForProject(ctx, proj.ID, storage.SessionListOpts{Live: &live})
	if err != nil {
		t.Fatalf("ListForProject live: %v", err)
	}
	if len(liveOnly) != 2 || liveOnly[0].ID != "newest-live" || liveOnly[1].ID != "recent-live" {
		t.Fatalf("live filter: %+v", ids(liveOnly))
	}

	// Live=false keeps only the closed one.
	closed := false
	closedOnly, err := db.Sessions().ListForProject(ctx, proj.ID, storage.SessionListOpts{Live: &closed})
	if err != nil {
		t.Fatalf("ListForProject closed: %v", err)
	}
	if len(closedOnly) != 1 || closedOnly[0].ID != "old-closed" {
		t.Fatalf("closed filter: %+v", ids(closedOnly))
	}

	// Since 24h cutoff drops the 2-day-old row.
	since := now.Add(-24 * time.Hour)
	recent, err := db.Sessions().ListForProject(ctx, proj.ID, storage.SessionListOpts{Since: &since})
	if err != nil {
		t.Fatalf("ListForProject since: %v", err)
	}
	if len(recent) != 2 {
		t.Fatalf("since filter: %+v", ids(recent))
	}

	// Limit=1 only returns the newest.
	one, err := db.Sessions().ListForProject(ctx, proj.ID, storage.SessionListOpts{Limit: 1})
	if err != nil {
		t.Fatalf("ListForProject limit: %v", err)
	}
	if len(one) != 1 || one[0].ID != "newest-live" {
		t.Fatalf("limit filter: %+v", ids(one))
	}
}

func ids(ss []*storage.Session) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = s.ID
	}
	return out
}

// Insert enforces the alias + group_dispatch + user fields make their way
// to the DB so the hosts/sessions view can render them without a second
// lookup.
func TestSessions_FieldRoundtrip(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	proj, host := seedSessionDeps(t, db)

	now := time.Now().UTC()
	orig := &storage.Session{
		ID: "s-1", ProjectID: proj.ID, IngressAddr: testIngressAddr, HostID: host.ID,
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

// StampOpenAuditRowsClosed only stamps the still-open rows and is
// safely idempotent on a second call. Pure historical maintenance —
// the result has no bearing on liveness, which lives in
// core.AgentLinkService.
func TestSessions_StampOpenAuditRowsClosed_IdempotentAndScoped(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	proj := seedProject(t, db, "prod", "Production", admin)
	host, _ := db.Hosts().Upsert(ctx, &storage.HostIdentity{
		ProjectID: proj.ID, MachineID: "m-x", Fingerprint: "fp-x",
		Hostname: "h-x", OS: "linux", SeenAt: time.Now().UTC(),
	})

	open := &storage.Session{
		ID: "open-1", ProjectID: proj.ID, IngressAddr: "0.0.0.0:9443", HostID: host.ID,
		ConnectedAt: time.Now().UTC(),
	}
	if err := db.Sessions().Insert(ctx, open); err != nil {
		t.Fatalf("Insert open: %v", err)
	}
	closed := &storage.Session{
		ID: "closed-1", ProjectID: proj.ID, IngressAddr: "0.0.0.0:9443", HostID: host.ID,
		ConnectedAt: time.Now().UTC().Add(-time.Hour),
	}
	if err := db.Sessions().Insert(ctx, closed); err != nil {
		t.Fatalf("Insert closed: %v", err)
	}
	if err := db.Sessions().MarkDisconnected(ctx, "closed-1"); err != nil {
		t.Fatalf("MarkDisconnected: %v", err)
	}
	closedBefore, _ := db.Sessions().Get(ctx, "closed-1")

	n, err := db.Sessions().StampOpenAuditRowsClosed(ctx)
	if err != nil {
		t.Fatalf("StampOpenAuditRowsClosed: %v", err)
	}
	if n != 1 {
		t.Fatalf("first sweep stamped %d rows; want 1 (the open one)", n)
	}
	openAfter, _ := db.Sessions().Get(ctx, "open-1")
	if openAfter.DisconnectedAt == nil {
		t.Fatalf("open row's disconnected_at not stamped")
	}
	closedAfter, _ := db.Sessions().Get(ctx, "closed-1")
	if !closedAfter.DisconnectedAt.Equal(*closedBefore.DisconnectedAt) {
		t.Fatalf("already-closed row's disconnected_at moved: before=%v after=%v",
			closedBefore.DisconnectedAt, closedAfter.DisconnectedAt)
	}

	// Second call: nothing left to stamp, no error.
	n, err = db.Sessions().StampOpenAuditRowsClosed(ctx)
	if err != nil {
		t.Fatalf("idempotent sweep: %v", err)
	}
	if n != 0 {
		t.Fatalf("second sweep stamped %d rows; want 0", n)
	}
}
