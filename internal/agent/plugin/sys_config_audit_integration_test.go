package plugin_test

import (
	"context"
	"os"
	"runtime"
	"testing"

	"github.com/WangYihang/Platypus/internal/agent/plugin"
	"github.com/WangYihang/Platypus/internal/agent/plugin/bridge"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

func installSysConfigAudit(t *testing.T) *plugin.Registry {
	t.Helper()
	wasm := stagedWasmBytes(t, "com.platypus.sys-config-audit", "3.0.0", "sys_config_audit.wasm")
	manifestBytes := stagedManifestBytes(t, "com.platypus.sys-config-audit", "3.0.0")

	pluginRoot := t.TempDir()
	paths := plugin.NewPaths(pluginRoot)
	sk, pk, err := plugin.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(paths.PublishersDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.PublisherKeyFile(plugin.HumanKeyID(pk)),
		[]byte(plugin.EncodePublicKey(pk, "")), 0o600); err != nil {
		t.Fatal(err)
	}
	manifestStr := rewriteManifestKeyID(string(manifestBytes), plugin.HumanKeyID(pk))
	sig, err := plugin.Sign(sk, wasm, plugin.DefaultTrustedComment("sys_config_audit.wasm"))
	if err != nil {
		t.Fatal(err)
	}
	reg, err := plugin.New(plugin.Options{Paths: paths})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { reg.Close(context.Background()) })

	if err := reg.InstallFromBytes(context.Background(), plugin.InstallParams{
		PluginID:            "com.platypus.sys-config-audit",
		Version:             "3.0.0",
		PublisherPubkey:     []byte(plugin.EncodePublicKey(pk, "")),
		Manifest:            []byte(manifestStr),
		Wasm:                wasm,
		Signature:           []byte(plugin.EncodeSignature(sig)),
		Actor:               "test",
		GrantedCapabilities: []plugin.CapabilityID{"fs.read"},
	}, nil); err != nil {
		t.Fatal(err)
	}
	return reg
}

// TestConfigAudit_ListAuditors: list_config_auditors reports the v3
// catalog. Asserts every auditor the manifest documents shows up —
// shell.history, cloud.aws, ssh.private_keys, env.process, db.config,
// webapp.config. A stale plugin (still on v2) would fail the id-set
// comparison loudly.
func TestConfigAudit_ListAuditors(t *testing.T) {
	reg := installSysConfigAudit(t)

	resp := bridge.ListConfigAuditors(reg)(context.Background(),
		&v2pb.ListConfigAuditorsRequest{})
	if resp.GetError() != "" {
		t.Fatalf("list_config_auditors error: %s", resp.GetError())
	}
	got := map[string]bool{}
	for i, a := range resp.GetAuditors() {
		if a.GetId() == "" {
			t.Errorf("auditor[%d] has empty id", i)
		}
		got[a.GetId()] = true
	}
	want := []string{
		"shell.history",
		"cloud.aws",
		"ssh.private_keys",
		"env.process",
		"db.config",
		"webapp.config",
	}
	for _, id := range want {
		if !got[id] {
			t.Errorf("v3 catalog missing auditor %q; got %v", id, auditorIDs(resp.GetAuditors()))
		}
	}
}

// TestConfigAudit_BridgeFillsTimingMetadata: a no-filter audit
// returns StartedAtUnix populated by the bridge wrap (wasm can't
// read clocks) plus one AuditorResult per registered auditor.
func TestConfigAudit_BridgeFillsTimingMetadata(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("v3 auditors read /home + /etc + /proc — linux-only")
	}
	reg := installSysConfigAudit(t)

	resp := bridge.ConfigAudit(reg)(context.Background(),
		&v2pb.ConfigAuditRequest{})
	if resp.GetError() != "" {
		t.Fatalf("config_audit error: %s", resp.GetError())
	}
	if resp.GetStartedAtUnix() == 0 {
		t.Errorf("started_at_unix not populated by bridge")
	}
	if len(resp.GetAuditors()) == 0 {
		t.Errorf("audit returned zero AuditorResults")
	}
}

// TestConfigAudit_FiltersByAuditorIDs: each documented v3 auditor id
// can be selected individually and the audit returns exactly one
// AuditorResult for it. Loops over the catalog so adding a new
// auditor automatically extends the test.
func TestConfigAudit_FiltersByAuditorIDs(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("v3 auditors read /home + /etc + /proc — linux-only")
	}
	reg := installSysConfigAudit(t)

	listed := bridge.ListConfigAuditors(reg)(context.Background(),
		&v2pb.ListConfigAuditorsRequest{})
	if len(listed.GetAuditors()) == 0 {
		t.Fatal("empty catalog")
	}
	for _, a := range listed.GetAuditors() {
		id := a.GetId()
		t.Run(id, func(t *testing.T) {
			resp := bridge.ConfigAudit(reg)(context.Background(),
				&v2pb.ConfigAuditRequest{AuditorIds: []string{id}})
			if resp.GetError() != "" {
				t.Fatalf("config_audit error: %s", resp.GetError())
			}
			if got := len(resp.GetAuditors()); got != 1 {
				t.Errorf("expected 1 AuditorResult for filter %q, got %d", id, got)
			} else if resp.GetAuditors()[0].GetId() != id {
				t.Errorf("auditor[0].id = %q; want %q",
					resp.GetAuditors()[0].GetId(), id)
			}
		})
	}
}

// TestConfigAudit_FiltersByCategory: passing a category restricts
// the audit to auditors in that category. v3 has multiple categories
// — the response should only contain that one.
func TestConfigAudit_FiltersByCategory(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("v3 auditors read /home + /etc + /proc — linux-only")
	}
	reg := installSysConfigAudit(t)

	resp := bridge.ConfigAudit(reg)(context.Background(),
		&v2pb.ConfigAuditRequest{Categories: []string{"shell"}})
	if resp.GetError() != "" {
		t.Fatalf("config_audit error: %s", resp.GetError())
	}
	if len(resp.GetAuditors()) == 0 {
		t.Fatal("shell category returned no AuditorResults")
	}
	for _, a := range resp.GetAuditors() {
		if a.GetCategory() != "shell" {
			t.Errorf("auditor %q has category %q, want shell",
				a.GetId(), a.GetCategory())
		}
	}
}

func auditorIDs(auditors []*v2pb.AvailableAuditor) []string {
	out := make([]string, 0, len(auditors))
	for _, a := range auditors {
		out = append(out, a.GetId())
	}
	return out
}
