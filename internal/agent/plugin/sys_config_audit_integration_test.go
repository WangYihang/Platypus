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
	wasm := stagedWasmBytes(t, "com.platypus.sys-config-audit", "2.0.0", "sys_config_audit.wasm")
	manifestBytes := stagedManifestBytes(t, "com.platypus.sys-config-audit", "2.0.0")

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
		Version:             "2.0.0",
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

// TestConfigAudit_ListAuditors: list_config_auditors reports the v2
// auditor catalog. Asserts at least one auditor entry comes back
// with non-empty id; per the manifest the v2 set covers
// shell.history / cloud.aws / ssh.private_keys.
func TestConfigAudit_ListAuditors(t *testing.T) {
	reg := installSysConfigAudit(t)

	resp := bridge.ListConfigAuditors(reg)(context.Background(),
		&v2pb.ListConfigAuditorsRequest{})
	if resp.GetError() != "" {
		t.Fatalf("list_config_auditors error: %s", resp.GetError())
	}
	auditors := resp.GetAuditors()
	if len(auditors) == 0 {
		t.Fatal("expected at least one available auditor; got empty list")
	}
	for i, a := range auditors {
		if a.GetId() == "" {
			t.Errorf("auditor[%d] has empty id", i)
		}
	}
}

// TestConfigAudit_BridgeFillsTimingMetadata: a no-filter audit
// returns StartedAtUnix + ElapsedMs populated by the bridge wrap
// (the wasm plugin can't read clocks). Mirrors the security bridge
// fix from the previous commit.
func TestConfigAudit_BridgeFillsTimingMetadata(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("v2 auditors read /home + /etc — linux-only")
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
	// Some auditor results should always come back even on a fresh
	// host with no shell history (each registered auditor emits a
	// "no findings" AuditorResult so the UI shows "ran 3 auditors").
	if len(resp.GetAuditors()) == 0 {
		t.Errorf("audit returned zero AuditorResults")
	}
}

// TestConfigAudit_FiltersByAuditorIDs: passing auditor_ids restricts
// the audit to those ids only. We pick whatever the catalog reports
// first so the test stays robust against future auditor renames.
func TestConfigAudit_FiltersByAuditorIDs(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("v2 auditors read /home + /etc — linux-only")
	}
	reg := installSysConfigAudit(t)

	listed := bridge.ListConfigAuditors(reg)(context.Background(),
		&v2pb.ListConfigAuditorsRequest{})
	if len(listed.GetAuditors()) == 0 {
		t.Skip("no auditors in catalog; nothing to filter against")
	}
	pick := listed.GetAuditors()[0].GetId()

	resp := bridge.ConfigAudit(reg)(context.Background(),
		&v2pb.ConfigAuditRequest{AuditorIds: []string{pick}})
	if resp.GetError() != "" {
		t.Fatalf("config_audit error: %s", resp.GetError())
	}
	if got := len(resp.GetAuditors()); got != 1 {
		t.Errorf("expected exactly 1 AuditorResult for filter %q, got %d",
			pick, got)
	}
	if got := resp.GetAuditors()[0].GetId(); got != pick {
		t.Errorf("auditor[0].id = %q; want %q", got, pick)
	}
}

