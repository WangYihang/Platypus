package storage_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	agentplugin "github.com/WangYihang/Platypus/internal/agent/plugin"
	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

func seedHost(t *testing.T, db *storage.DB, projectID, machineID string) *storage.Host {
	t.Helper()
	h, err := db.Hosts().Upsert(context.Background(), &storage.HostIdentity{
		ProjectID:   projectID,
		MachineID:   machineID,
		Fingerprint: "fp-" + machineID,
		Hostname:    "host-" + machineID,
		SeenAt:      time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("seedHost upsert: %v", err)
	}
	return h
}

func TestHostPlugins_Upsert_Insert(t *testing.T) {
	db := newTestDB(t)
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	proj := seedProject(t, db, "p1", "Project 1", admin)
	h := seedHost(t, db, proj.ID, "m1")

	cfg := json.RawMessage(`{"region":"us-east-1"}`)
	err := db.HostPlugins().Upsert(context.Background(), storage.HostPluginUpsert{
		HostID:              h.ID,
		PluginID:            "datadog-forwarder",
		Version:             "1.2.0",
		GrantedCapabilities: []agentplugin.CapabilityID{"net.http", "fs.read"},
		ConfigResolved:      cfg,
		SchemaVersion:       1,
		State:               storage.HostPluginInstalled,
	})
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	got, err := db.HostPlugins().Get(context.Background(), h.ID, "datadog-forwarder")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Version != "1.2.0" || got.SchemaVersion != 1 {
		t.Fatalf("identity: %+v", got)
	}
	if got.State != storage.HostPluginInstalled {
		t.Fatalf("State = %v, want installed", got.State)
	}
	if len(got.GrantedCapabilities) != 2 || got.GrantedCapabilities[0] != "net.http" {
		t.Fatalf("caps = %v", got.GrantedCapabilities)
	}
	if string(got.ConfigResolved) != string(cfg) {
		t.Fatalf("config = %s, want %s", got.ConfigResolved, cfg)
	}
	if got.InstalledAt == nil {
		t.Fatalf("installed_at not stamped on first transition into installed")
	}
}

func TestHostPlugins_Upsert_PreservesInstalledAt(t *testing.T) {
	db := newTestDB(t)
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	proj := seedProject(t, db, "p1", "Project 1", admin)
	h := seedHost(t, db, proj.ID, "m1")

	mk := func(version string) storage.HostPluginUpsert {
		return storage.HostPluginUpsert{
			HostID:        h.ID,
			PluginID:      "p",
			Version:       version,
			SchemaVersion: 1,
			State:         storage.HostPluginInstalled,
		}
	}
	if err := db.HostPlugins().Upsert(context.Background(), mk("1.0.0")); err != nil {
		t.Fatalf("Upsert v1: %v", err)
	}
	first, _ := db.HostPlugins().Get(context.Background(), h.ID, "p")

	// Sleep a millisecond so a fresh installed_at would be
	// distinguishable. Then upsert a version bump.
	time.Sleep(2 * time.Millisecond)
	if err := db.HostPlugins().Upsert(context.Background(), mk("1.0.1")); err != nil {
		t.Fatalf("Upsert v2: %v", err)
	}
	second, _ := db.HostPlugins().Get(context.Background(), h.ID, "p")

	if !first.InstalledAt.Equal(*second.InstalledAt) {
		t.Fatalf("installed_at changed across version bump (was %v, now %v) — should be preserved",
			first.InstalledAt, second.InstalledAt)
	}
	if second.Version != "1.0.1" {
		t.Fatalf("Version = %q, want 1.0.1", second.Version)
	}
}

func TestHostPlugins_Upsert_FailedCarriesError(t *testing.T) {
	db := newTestDB(t)
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	proj := seedProject(t, db, "p1", "Project 1", admin)
	h := seedHost(t, db, proj.ID, "m1")

	if err := db.HostPlugins().Upsert(context.Background(), storage.HostPluginUpsert{
		HostID:    h.ID,
		PluginID:  "p",
		Version:   "1.0.0",
		State:     storage.HostPluginFailed,
		LastError: "validation: missing required field destination",
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	got, err := db.HostPlugins().Get(context.Background(), h.ID, "p")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.LastError == "" {
		t.Fatalf("LastError empty on failed state")
	}
	// installed_at should NOT be set for the failed-only path.
	if got.InstalledAt != nil {
		t.Fatalf("installed_at set on failed-only upsert: %v", got.InstalledAt)
	}
}

func TestHostPlugins_ListByHost_AndByPlugin(t *testing.T) {
	db := newTestDB(t)
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	proj := seedProject(t, db, "p1", "Project 1", admin)
	h1 := seedHost(t, db, proj.ID, "m1")
	h2 := seedHost(t, db, proj.ID, "m2")

	for _, up := range []storage.HostPluginUpsert{
		{HostID: h1.ID, PluginID: "syslog", Version: "1.0", State: storage.HostPluginInstalled},
		{HostID: h1.ID, PluginID: "av", Version: "2.0", State: storage.HostPluginInstalled},
		{HostID: h2.ID, PluginID: "syslog", Version: "1.0", State: storage.HostPluginPending},
	} {
		if err := db.HostPlugins().Upsert(context.Background(), up); err != nil {
			t.Fatalf("Upsert %+v: %v", up, err)
		}
	}

	byHost, err := db.HostPlugins().ListByHost(context.Background(), h1.ID)
	if err != nil {
		t.Fatalf("ListByHost: %v", err)
	}
	if len(byHost) != 2 {
		t.Fatalf("byHost = %d rows, want 2", len(byHost))
	}

	// "Which hosts have syslog v1.0 live?" is the rollout
	// reconciler's bread-and-butter — only h1's row should match
	// since h2 is still pending.
	live, err := db.HostPlugins().ListHostsRunningPlugin(context.Background(), "syslog", "1.0")
	if err != nil {
		t.Fatalf("ListHostsRunningPlugin: %v", err)
	}
	if len(live) != 1 || live[0].HostID != h1.ID {
		t.Fatalf("live = %+v, want only h1", live)
	}
}

func TestHostPlugins_MarkRemoved(t *testing.T) {
	db := newTestDB(t)
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	proj := seedProject(t, db, "p1", "Project 1", admin)
	h := seedHost(t, db, proj.ID, "m1")

	if err := db.HostPlugins().Upsert(context.Background(), storage.HostPluginUpsert{
		HostID: h.ID, PluginID: "p", Version: "1.0", State: storage.HostPluginInstalled,
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if err := db.HostPlugins().MarkRemoved(context.Background(), h.ID, "p"); err != nil {
		t.Fatalf("MarkRemoved: %v", err)
	}
	got, err := db.HostPlugins().Get(context.Background(), h.ID, "p")
	if err != nil {
		t.Fatalf("Get post-remove: %v", err)
	}
	if got.State != storage.HostPluginRemoved {
		t.Fatalf("State = %v, want removed (audit trail should keep the row)", got.State)
	}
	if err := db.HostPlugins().MarkRemoved(context.Background(), h.ID, "ghost"); err != storage.ErrNotFound {
		t.Fatalf("MarkRemoved missing err = %v, want ErrNotFound", err)
	}
}
