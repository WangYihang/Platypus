package storage_test

import (
	"context"
	"testing"
	"time"

	agentplugin "github.com/WangYihang/Platypus/internal/agent/plugin"
	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

func TestHostRepo_UpsertByMachineID_MergesRepeatedCalls(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	proj := seedProject(t, db, "prod", "Production", admin)

	now := time.Now().UTC()
	first := &storage.HostIdentity{
		ProjectID:   proj.ID,
		MachineID:   "m-abc",
		Fingerprint: "fp-1",
		Hostname:    "alpha-01",
		OS:          "linux",
		SeenAt:      now,
	}
	h1, err := db.Hosts().Upsert(ctx, first)
	if err != nil {
		t.Fatalf("Upsert#1: %v", err)
	}
	if h1.MachineID != "m-abc" || h1.FirstSeenAt.Compare(h1.LastSeenAt) > 0 {
		t.Fatalf("unexpected host: %+v", h1)
	}

	// Second call with same machine_id (possibly different fingerprint)
	// must return the same row id and leave first_seen_at untouched.
	later := now.Add(time.Hour)
	second := &storage.HostIdentity{
		ProjectID:   proj.ID,
		MachineID:   "m-abc",
		Fingerprint: "fp-2",
		Hostname:    "alpha-01-renamed",
		OS:          "linux",
		SeenAt:      later,
	}
	h2, err := db.Hosts().Upsert(ctx, second)
	if err != nil {
		t.Fatalf("Upsert#2: %v", err)
	}
	if h2.ID != h1.ID {
		t.Fatalf("upsert created a new row: h1=%s h2=%s", h1.ID, h2.ID)
	}
	if !h2.FirstSeenAt.Equal(h1.FirstSeenAt) {
		t.Fatalf("first_seen_at drifted: %v vs %v", h2.FirstSeenAt, h1.FirstSeenAt)
	}
	if !h2.LastSeenAt.After(h1.LastSeenAt) {
		t.Fatalf("last_seen_at did not advance: %v vs %v", h2.LastSeenAt, h1.LastSeenAt)
	}
	// Hostname should track the newest observed value — the agent might
	// have been aliased or renamed.
	if h2.Hostname != "alpha-01-renamed" {
		t.Fatalf("hostname = %q; want newest value", h2.Hostname)
	}
}

// A session without a machine_id falls back to the fingerprint-based key.
// If that same machine later reports a machine_id, the existing row gets
// upgraded in place — history is preserved, the FingerprintFallback flag
// flips off.
func TestHostRepo_Upsert_FingerprintUpgradeToMachineID(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	proj := seedProject(t, db, "prod", "Production", admin)
	now := time.Now().UTC()

	h1, err := db.Hosts().Upsert(ctx, &storage.HostIdentity{
		ProjectID:   proj.ID,
		MachineID:   "", // unavailable
		Fingerprint: "fp-abc",
		Hostname:    "alpha-01",
		OS:          "linux",
		SeenAt:      now,
	})
	if err != nil {
		t.Fatalf("Upsert#1: %v", err)
	}
	if !h1.FingerprintFallback {
		t.Fatal("expected FingerprintFallback=true for machine_id-less upsert")
	}

	// Same machine now reports an id.
	h2, err := db.Hosts().Upsert(ctx, &storage.HostIdentity{
		ProjectID:   proj.ID,
		MachineID:   "m-abc",
		Fingerprint: "fp-abc",
		Hostname:    "alpha-01",
		OS:          "linux",
		SeenAt:      now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("Upsert#2: %v", err)
	}
	if h2.ID != h1.ID {
		t.Fatalf("fingerprint-id upgrade created new row: %s vs %s", h1.ID, h2.ID)
	}
	if h2.FingerprintFallback {
		t.Fatal("FingerprintFallback should be false after machine_id upgrade")
	}
	if h2.MachineID != "m-abc" {
		t.Fatalf("MachineID = %q; want m-abc", h2.MachineID)
	}
}

func TestHostRepo_ListByProject(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	prod := seedProject(t, db, "prod", "Production", admin)
	stag := seedProject(t, db, "staging", "Staging", admin)
	now := time.Now().UTC()

	_, _ = db.Hosts().Upsert(ctx, &storage.HostIdentity{
		ProjectID: prod.ID, MachineID: "m-1", Fingerprint: "fp-1",
		Hostname: "alpha", OS: "linux", SeenAt: now,
	})
	_, _ = db.Hosts().Upsert(ctx, &storage.HostIdentity{
		ProjectID: prod.ID, MachineID: "m-2", Fingerprint: "fp-2",
		Hostname: "beta", OS: "linux", SeenAt: now,
	})
	_, _ = db.Hosts().Upsert(ctx, &storage.HostIdentity{
		ProjectID: stag.ID, MachineID: "m-3", Fingerprint: "fp-3",
		Hostname: "gamma", OS: "linux", SeenAt: now,
	})

	prodHosts, err := db.Hosts().ListByProject(ctx, prod.ID)
	if err != nil {
		t.Fatalf("ListByProject(prod): %v", err)
	}
	if len(prodHosts) != 2 {
		t.Fatalf("prodHosts len=%d; want 2", len(prodHosts))
	}
	stagHosts, _ := db.Hosts().ListByProject(ctx, stag.ID)
	if len(stagHosts) != 1 || stagHosts[0].Hostname != "gamma" {
		t.Fatalf("stagHosts: %+v", stagHosts)
	}
}

// Different projects with the same machine_id are independent rows — a
// machine could be connected to two red-team engagements at once.
func TestHostRepo_Upsert_SameMachineDifferentProjects(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	prod := seedProject(t, db, "prod", "Production", admin)
	stag := seedProject(t, db, "staging", "Staging", admin)
	now := time.Now().UTC()

	h1, _ := db.Hosts().Upsert(ctx, &storage.HostIdentity{
		ProjectID: prod.ID, MachineID: "m-abc", Fingerprint: "fp-x",
		Hostname: "alpha", OS: "linux", SeenAt: now,
	})
	h2, _ := db.Hosts().Upsert(ctx, &storage.HostIdentity{
		ProjectID: stag.ID, MachineID: "m-abc", Fingerprint: "fp-x",
		Hostname: "alpha", OS: "linux", SeenAt: now,
	})
	if h1.ID == h2.ID {
		t.Fatal("same machine_id in different projects was merged into one row")
	}
}

// TouchLastSeen bumps last_seen_at on a per-tick heartbeat from the
// agent-link handler so the Web UI's "online" presence dot stays
// accurate over a long-lived idle link. No-op (ErrNotFound) when the
// agent_id has no host yet — Upsert may still be in flight.
func TestHostRepo_TouchLastSeen(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	proj := seedProject(t, db, "prod", "Production", admin)

	t0 := time.Now().Add(-time.Hour).UTC()
	h, err := db.Hosts().Upsert(ctx, &storage.HostIdentity{
		ProjectID: proj.ID, AgentID: "agent-1", Fingerprint: "fp-1",
		Hostname: "alpha", OS: "linux", SeenAt: t0,
	})
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if !h.LastSeenAt.Equal(t0) {
		t.Fatalf("setup: LastSeenAt = %v; want %v", h.LastSeenAt, t0)
	}

	t1 := t0.Add(45 * time.Second)
	if err := db.Hosts().TouchLastSeen(ctx, "agent-1", t1); err != nil {
		t.Fatalf("TouchLastSeen: %v", err)
	}
	got, err := db.Hosts().GetByAgentID(ctx, "agent-1")
	if err != nil {
		t.Fatalf("GetByAgentID: %v", err)
	}
	if !got.LastSeenAt.Equal(t1) {
		t.Fatalf("after touch: LastSeenAt = %v; want %v", got.LastSeenAt, t1)
	}

	if err := db.Hosts().TouchLastSeen(ctx, "agent-missing", t1); err == nil {
		t.Fatalf("TouchLastSeen on missing agent_id should return error")
	}
}

// TestHostRepo_PluginSpecs_RoundtripPreservesRichFields: the
// hosts.plugin_specs column captures the rich PluginSpec shape
// chosen at enroll time. Pinning the round-trip means the agent-
// link reconciler (PR 3.4 next) can read full version + caps +
// config — not just plugin_id — when it diffs the agent's
// installed catalog against the operator's baseline.
func TestHostRepo_PluginSpecs_RoundtripPreservesRichFields(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	proj := seedProject(t, db, "p1", "Project 1", admin)

	h, err := db.Hosts().Upsert(ctx, &storage.HostIdentity{
		ProjectID:   proj.ID,
		MachineID:   "m-rich",
		Fingerprint: "fp-rich",
		Hostname:    "rich-host",
		SeenAt:      time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("Upsert host: %v", err)
	}

	specs := []storage.PluginSpec{
		{
			PluginID:            "com.example.syslog",
			Version:             "1.4.0",
			GrantedCapabilities: []agentplugin.CapabilityID{"net.dial"},
			ConfigOverrides:     []byte(`{"destination":"udp://10.0.0.1:514"}`),
			SchemaVersion:       1,
		},
		{PluginID: "com.example.sysinfo"},
	}
	if err := db.Hosts().SetPluginSpecs(ctx, h.ID, specs); err != nil {
		t.Fatalf("SetPluginSpecs: %v", err)
	}

	got, err := db.Hosts().GetByID(ctx, h.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if len(got.PluginSpecs) != 2 {
		t.Fatalf("PluginSpecs len = %d, want 2", len(got.PluginSpecs))
	}
	first := got.PluginSpecs[0]
	if first.PluginID != "com.example.syslog" || first.Version != "1.4.0" {
		t.Fatalf("first.identity = %+v", first)
	}
	if len(first.GrantedCapabilities) != 1 || first.GrantedCapabilities[0] != "net.dial" {
		t.Fatalf("first.caps = %v", first.GrantedCapabilities)
	}
	if string(first.ConfigOverrides) != `{"destination":"udp://10.0.0.1:514"}` {
		t.Fatalf("first.config = %s", first.ConfigOverrides)
	}
	if first.SchemaVersion != 1 {
		t.Fatalf("first.schema_version = %d", first.SchemaVersion)
	}
}
