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

func installSysMountsLinux(t *testing.T) *plugin.Registry {
	t.Helper()
	const id = "com.platypus.sys-mounts-linux"
	const ver = "1.0.0"
	wasm := stagedWasmBytes(t, id, ver, "sys_mounts_linux.wasm")
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
	sig, err := plugin.Sign(sk, wasm, plugin.DefaultTrustedComment("sys_mounts_linux.wasm"))
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
		GrantedCapabilities: []plugin.CapabilityID{"fs.read"},
	}, nil); err != nil {
		t.Fatal(err)
	}
	return reg
}

func TestSysMountsLinux_Manifest(t *testing.T) {
	manifestBytes := stagedManifestBytes(t, "com.platypus.sys-mounts-linux", "1.0.0")
	m, err := plugin.ParseManifest(manifestBytes)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := m.Runtime.OSTargets; len(got) != 1 || got[0] != "linux" {
		t.Errorf("os_targets = %v, want [linux]", got)
	}
	if len(m.RPC) != 1 || m.RPC[0].Name != "list_mounts" {
		t.Errorf("rpc = %+v", m.RPC)
	}
}

// TestSysMountsLinux_TypedBridge_RoundTrip exercises bridge.MountList
// via the typed proto. Asserts proto matches actual plugin output and
// that the test container's root mount comes back.
func TestSysMountsLinux_TypedBridge_RoundTrip(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("sys-mounts-linux is linux-only")
	}
	reg := installSysMountsLinux(t)

	resp := bridge.MountList(reg)(context.Background(),
		&v2pb.MountListRequest{IncludePseudo: true})
	if resp.GetError() != "" {
		t.Fatalf("typed bridge: %s", resp.GetError())
	}
	if len(resp.GetMounts()) == 0 {
		t.Fatal("expected at least one mount")
	}
	hasRoot := false
	for _, m := range resp.GetMounts() {
		if m.GetMountpoint() == "/" {
			hasRoot = true
		}
		if m.GetSource() == "" || m.GetFstype() == "" {
			t.Errorf("mount missing source or fstype: %+v", m)
		}
	}
	if !hasRoot {
		t.Errorf("expected root '/' mount in response")
	}
	t.Logf("found %d mounts (incl. pseudo)", len(resp.GetMounts()))
}

// TestSysMountsLinux_FiltersPseudoByDefault asserts that without
// IncludePseudo=true the response excludes proc / tmpfs / sysfs etc.
func TestSysMountsLinux_FiltersPseudoByDefault(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("sys-mounts-linux is linux-only")
	}
	reg := installSysMountsLinux(t)

	resp := bridge.MountList(reg)(context.Background(),
		&v2pb.MountListRequest{})
	if resp.GetError() != "" {
		t.Fatalf("typed bridge: %s", resp.GetError())
	}
	for i, m := range resp.GetMounts() {
		if m.GetPseudo() {
			t.Errorf("mount[%d] is pseudo but include_pseudo=false didn't filter: %+v", i, m)
		}
		if m.GetFstype() == "tmpfs" || m.GetFstype() == "proc" || m.GetFstype() == "sysfs" {
			t.Errorf("mount[%d] has pseudo fstype %q but pseudo=false", i, m.GetFstype())
		}
	}
}
