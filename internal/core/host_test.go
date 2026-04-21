package core

import (
	"context"
	"testing"
	"time"

	"github.com/WangYihang/Platypus/internal/app"
	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
	"github.com/WangYihang/Platypus/internal/utils/os"
)

func TestComputeFingerprint_Deterministic(t *testing.T) {
	a := ComputeFingerprint("alpha", map[string]string{"eth0": "aa:bb", "lo": ""})
	b := ComputeFingerprint("alpha", map[string]string{"lo": "", "eth0": "aa:bb"})
	if a != b {
		t.Fatalf("order-independence broken: %q vs %q", a, b)
	}
}

func TestComputeFingerprint_HostnameMatters(t *testing.T) {
	a := ComputeFingerprint("alpha", map[string]string{"eth0": "aa:bb"})
	b := ComputeFingerprint("beta", map[string]string{"eth0": "aa:bb"})
	if a == b {
		t.Fatalf("different hostnames produced same fingerprint")
	}
}

func TestSplitAgentMachineID(t *testing.T) {
	for _, tc := range []struct {
		in             string
		wantID         string
		wantIsFallback bool
	}{
		{"real-uuid", "real-uuid", false},
		{"fp-abc", "", true},
		{"", "", true},
	} {
		gotID, gotFb := SplitAgentMachineID(tc.in)
		if gotID != tc.wantID || gotFb != tc.wantIsFallback {
			t.Errorf("SplitAgentMachineID(%q) = (%q,%v); want (%q,%v)",
				tc.in, gotID, gotFb, tc.wantID, tc.wantIsFallback)
		}
	}
}

// TestUpsertHostForAgent_UsesDefaultProject sets up a fresh in-memory DB
// with only a "default" project, hands the upsert an AgentClient, and
// verifies a Host row shows up under that project.
func TestUpsertHostForAgent_UsesDefaultProject(t *testing.T) {
	Ctx = app.New(nil)
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	Ctx.Storage = db

	// Seed: admin user + default project.
	admin := &user.User{
		ID: "u-admin", Username: "admin", PasswordHash: "x",
		Role: user.RoleAdmin, CreatedAt: time.Now().UTC(),
	}
	_ = db.Users().Create(context.Background(), admin)
	proj := &storage.Project{
		ID: "prj-default", Name: "Default", Slug: "default",
		CreatedAt: time.Now().UTC(), CreatedBy: admin.ID,
	}
	_ = db.Projects().Create(context.Background(), proj)

	// Minimal AgentClient — only the fields UpsertHostForAgent reads.
	c := &AgentClient{
		MachineID:         "real-id-123",
		Hostname:          "alpha-01",
		OS:                os.Linux,
		NetworkInterfaces: map[string]string{"eth0": "aa:bb:cc:dd:ee:ff"},
	}
	UpsertHostForAgent(context.Background(), c)

	if c.HostID == "" {
		t.Fatal("HostID not set after UpsertHostForAgent")
	}
	if c.ProjectID != proj.ID {
		t.Fatalf("ProjectID = %q; want %q", c.ProjectID, proj.ID)
	}
	hosts, err := db.Hosts().ListByProject(context.Background(), proj.ID)
	if err != nil || len(hosts) != 1 {
		t.Fatalf("hosts: %+v err=%v", hosts, err)
	}
	if hosts[0].MachineID != "real-id-123" || hosts[0].FingerprintFallback {
		t.Fatalf("host state: %+v", hosts[0])
	}

	// Second call with fp-prefixed id and the same fingerprint should
	// be treated as a fingerprint-fallback observation and match the
	// existing row (because the fingerprint matches).
	c2 := &AgentClient{
		MachineID:         "fp-xyz", // agent fallback
		Hostname:          "alpha-01",
		OS:                os.Linux,
		NetworkInterfaces: map[string]string{"eth0": "aa:bb:cc:dd:ee:ff"},
	}
	UpsertHostForAgent(context.Background(), c2)
	hosts, _ = db.Hosts().ListByProject(context.Background(), proj.ID)
	if len(hosts) != 1 {
		t.Fatalf("fingerprint-match didn't merge; got %d hosts", len(hosts))
	}
}

// When no default project exists, upsert skips silently — the TCPServer
// is still alive, the session still connects; we just don't get a Host
// row until the operator bootstraps.
func TestUpsertHostForAgent_NoDefaultProject_SkipsCleanly(t *testing.T) {
	Ctx = app.New(nil)
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	Ctx.Storage = db

	c := &AgentClient{
		MachineID: "m-abc",
		Hostname:  "alpha-01",
		OS:        os.Linux,
	}
	// Must not panic and must not crash.
	UpsertHostForAgent(context.Background(), c)

	if c.HostID != "" {
		t.Fatal("HostID was set despite missing default project")
	}
}
