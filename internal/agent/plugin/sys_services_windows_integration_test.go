package plugin_test

import (
	"context"
	"os"
	"testing"

	"github.com/WangYihang/Platypus/internal/agent/plugin"
)

func installSysServicesWindows(t *testing.T) *plugin.Registry {
	t.Helper()
	const id = "com.platypus.sys-services-windows"
	const ver = "1.0.0"
	wasm := stagedWasmBytes(t, id, ver, "sys_services_windows.wasm")
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
	sig, err := plugin.Sign(sk, wasm, plugin.DefaultTrustedComment("sys_services_windows.wasm"))
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

func TestSysServicesWindows_Manifest(t *testing.T) {
	manifestBytes := stagedManifestBytes(t, "com.platypus.sys-services-windows", "1.0.0")
	m, err := plugin.ParseManifest(manifestBytes)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := m.Runtime.OSTargets; len(got) != 1 || got[0] != "windows" {
		t.Errorf("os_targets = %v, want [windows]", got)
	}
	if len(m.RPC) != 2 {
		t.Errorf("rpc len = %d, want 2", len(m.RPC))
	}
	if m.Capabilities.Exec == nil || len(m.Capabilities.Exec.Commands) != 1 {
		t.Errorf("exec mis-declared: %+v", m.Capabilities.Exec)
	}
}

// Install path covered by Manifest test. A windows-runner round-
// trip would invoke list_units against the live SCM; not feasible
// from a Linux container.
func TestSysServicesWindows_InstallSucceeds(t *testing.T) {
	_ = installSysServicesWindows(t)
}
