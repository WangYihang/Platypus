package plugin_test

import (
	"context"
	"encoding/json"
	"os"
	"runtime"
	"testing"

	"github.com/WangYihang/Platypus/internal/agent/plugin"
	"github.com/WangYihang/Platypus/internal/agent/plugin/bridge"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

func installSysPkgLinux(t *testing.T) *plugin.Registry {
	t.Helper()
	const id = "com.platypus.sys-pkg-linux"
	const ver = "1.0.0"
	wasm := stagedWasmBytes(t, id, ver, "sys_pkg_linux.wasm")
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
	sig, err := plugin.Sign(sk, wasm, plugin.DefaultTrustedComment("sys_pkg_linux.wasm"))
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

func TestSysPkgLinux_Manifest(t *testing.T) {
	manifestBytes := stagedManifestBytes(t, "com.platypus.sys-pkg-linux", "1.0.0")
	m, err := plugin.ParseManifest(manifestBytes)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := m.Runtime.OSTargets; len(got) != 1 || got[0] != "linux" {
		t.Errorf("os_targets = %v, want [linux]", got)
	}
	if len(m.RPC) != 2 {
		t.Errorf("rpc len = %d, want 2", len(m.RPC))
	}
	if m.Capabilities.Exec == nil || len(m.Capabilities.Exec.Commands) < 6 {
		t.Errorf("exec cap should allowlist /bin/sh + 5 backends, got: %+v", m.Capabilities.Exec)
	}
}

// TestSysPkgLinux_ListInstalled_RoundTrip exercises the plugin in
// the test container. Container has dpkg-query (Debian-based slim
// image) so detection should pick "apt"; package list is non-empty.
// On environments without any backend, the plugin returns
// error="no_supported_package_manager" — we tolerate that.
func TestSysPkgLinux_ListInstalled_RoundTrip(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("sys-pkg-linux is linux-only")
	}
	reg := installSysPkgLinux(t)

	resp := reg.Invoke(context.Background(), &v2pb.PluginCallRequest{
		PluginId: "com.platypus.sys-pkg-linux",
		Method:   "list_installed",
		Payload:  []byte(`{"max_results": 50}`),
	})
	if resp.GetError() != "" {
		t.Fatalf("plugin call: %s", resp.GetError())
	}
	var decoded struct {
		Packages []struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"packages"`
		Backend string `json:"backend"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(resp.GetPayload(), &decoded); err != nil {
		t.Fatalf("decode: %v\npayload: %s", err, resp.GetPayload())
	}
	t.Logf("backend=%q packages=%d error=%q",
		decoded.Backend, len(decoded.Packages), decoded.Error)
	if decoded.Error == "no_supported_package_manager" {
		t.Skip("no package manager available in this environment")
	}
	if decoded.Backend == "" {
		t.Errorf("expected non-empty backend in response")
	}
	for i, p := range decoded.Packages {
		if p.Name == "" {
			t.Errorf("package[%d] has empty name: %+v", i, p)
		}
	}
}

// TestSysPkgLinux_TypedBridge_RoundTrip exercises the typed bridge
// (bridge.PkgListInstalled). The point of this test is to fail loudly
// if proto/v2/sys_pkg.proto drifts from the JSON shape the plugin
// actually emits — protojson.Unmarshal would surface a "proto:
// (line 1:N): unknown field …" error in that case.
//
// Linux-only because the bridge dispatches on runtime.GOOS, and we
// only stage the linux variant here.
func TestSysPkgLinux_TypedBridge_RoundTrip(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("sys-pkg-linux is linux-only")
	}
	reg := installSysPkgLinux(t)

	resp := bridge.PkgListInstalled(reg)(context.Background(),
		&v2pb.PkgListInstalledRequest{MaxResults: 50})
	if got := resp.GetError(); got != "" && got != "no_supported_package_manager" {
		t.Fatalf("typed bridge returned error: %s", got)
	}
	if resp.GetError() == "no_supported_package_manager" {
		t.Skip("no package manager in this environment")
	}
	if resp.GetBackend() == "" {
		t.Errorf("expected non-empty backend in typed response")
	}
	for i, p := range resp.GetPackages() {
		if p.GetName() == "" {
			t.Errorf("package[%d] has empty name", i)
		}
	}
}
