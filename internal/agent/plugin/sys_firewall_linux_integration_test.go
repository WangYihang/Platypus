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

func installSysFirewallLinux(t *testing.T) *plugin.Registry {
	t.Helper()
	const id = "com.platypus.sys-firewall-linux"
	const ver = "1.0.0"
	wasm := stagedWasmBytes(t, id, ver, "sys_firewall_linux.wasm")
	manifestBytes := stagedManifestBytes(t, id, ver)

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
	sig, err := plugin.Sign(sk, wasm, plugin.DefaultTrustedComment("sys_firewall_linux.wasm"))
	if err != nil {
		t.Fatal(err)
	}
	reg, err := plugin.New(plugin.Options{Paths: paths})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { reg.Close(context.Background()) })

	if err := reg.InstallFromBytes(context.Background(), plugin.InstallParams{
		PluginID:            id,
		Version:             ver,
		PublisherPubkey:     []byte(plugin.EncodePublicKey(pk, "")),
		Manifest:            []byte(manifestStr),
		Wasm:                wasm,
		Signature:           []byte(plugin.EncodeSignature(sig)),
		Actor:               "test",
		GrantedCapabilities: []plugin.CapabilityID{"exec"},
	}, nil); err != nil {
		t.Fatal(err)
	}
	return reg
}

func TestSysFirewallLinux_Manifest(t *testing.T) {
	manifestBytes := stagedManifestBytes(t, "com.platypus.sys-firewall-linux", "1.0.0")
	m, err := plugin.ParseManifest(manifestBytes)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := m.Runtime.OSTargets; len(got) != 1 || got[0] != "linux" {
		t.Errorf("os_targets = %v, want [linux]", got)
	}
	if len(m.RPC) != 1 || m.RPC[0].Name != "list_firewall_rules" {
		t.Errorf("rpc = %+v", m.RPC)
	}
}

// TestSysFirewallLinux_TypedBridge_RoundTrip exercises bridge.FirewallList.
// The container may have no firewall installed at all — that's a
// "no_supported_firewall_backend" response, not a failure. The
// point of the test is the proto round trip + plugin install path.
func TestSysFirewallLinux_TypedBridge_RoundTrip(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("sys-firewall-linux is linux-only")
	}
	reg := installSysFirewallLinux(t)

	resp := bridge.FirewallList(reg)(context.Background(),
		&v2pb.FirewallListRequest{IncludeDisabled: true})
	if got := resp.GetError(); got != "" && got != "no_supported_firewall_backend" {
		t.Fatalf("typed bridge unexpected error: %s", got)
	}
	if resp.GetError() == "no_supported_firewall_backend" {
		t.Skip("no firewall backend in test environment")
	}
	for i, r := range resp.GetRules() {
		if r.GetRaw() == "" {
			t.Errorf("rule[%d] missing raw: %+v", i, r)
		}
	}
	t.Logf("backend=%q rules=%d", resp.GetBackend(), len(resp.GetRules()))
}
