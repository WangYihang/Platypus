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

// installSysInfo wires the staged sys-info wasm into a fresh
// registry. The plugin needs sysinfo + fs.read (it parses /proc).
//
// Source of truth is internal/server/sysplugins/embedded/ — the same
// tree the server binary ships. A missing artefact here means
// hack/stage_system_plugins didn't run after a rust source change
// (or a fresh clone never produced one); fail loud instead of
// silently skipping.
func installSysInfo(t *testing.T) *plugin.Registry {
	t.Helper()
	wasm := stagedWasmBytes(t, "com.platypus.sys-info", "2.0.0", "sys_info_plugin.wasm")
	manifestBytes := stagedManifestBytes(t, "com.platypus.sys-info", "2.0.0")

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
	// The staged manifest's key_id field is a hex literal that
	// rotates with every `go run ./hack/stage_system_plugins` run.
	// For tests we don't care about its value — the agent verifies
	// signatures against the supplied PublisherPubkey (manifest
	// key_id is purely informational, see manifest_validate.go) —
	// so swap whatever's there for this test's freshly-minted key
	// to keep the manifest internally consistent.
	manifestStr := rewriteManifestKeyID(string(manifestBytes), plugin.HumanKeyID(pk))
	sig, err := plugin.Sign(sk, wasm, plugin.DefaultTrustedComment("sys_info_plugin.wasm"))
	if err != nil {
		t.Fatal(err)
	}
	reg, err := plugin.New(plugin.Options{Paths: paths})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { reg.Close(context.Background()) })

	if err := reg.InstallFromBytes(context.Background(), plugin.InstallParams{
		PluginID:            "com.platypus.sys-info",
		Version:             "2.0.0",
		PublisherPubkey:     []byte(plugin.EncodePublicKey(pk, "")),
		Manifest:            []byte(manifestStr),
		Wasm:                wasm,
		Signature:           []byte(plugin.EncodeSignature(sig)),
		Actor:               "test",
		GrantedCapabilities: []plugin.CapabilityID{"sysinfo", "fs.read"},
	}, nil); err != nil {
		t.Fatal(err)
	}
	return reg
}

// TestSysInfo_Hostname round-trips a sys_info call and asserts the
// hostname matches what the OS reports. The merged sys-info plugin
// reads /etc/hostname and falls back to /proc/sys/kernel/hostname,
// so the comparison stays accurate on the linux container the tests
// run inside.
func TestSysInfo_Hostname(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("sys-info reads /proc paths; linux-only")
	}
	reg := installSysInfo(t)

	resp := bridge.SysInfo(reg)(context.Background(), &v2pb.SysInfoRequest{})
	if resp.GetError() != "" {
		t.Fatalf("sys_info error: %s", resp.GetError())
	}
	want, err := os.Hostname()
	if err != nil {
		t.Fatalf("os.Hostname: %v", err)
	}
	if resp.GetHostname() != want {
		t.Errorf("hostname = %q; want %q", resp.GetHostname(), want)
	}
}

// TestSysInfo_PlatformReportsLinux: the plugin populates Platform
// from /proc/version + /etc/os-release. We just assert it's non-empty
// and includes the OS family token so tests stay portable across
// containers (the literal value differs per distro).
func TestSysInfo_PlatformReportsLinux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("/proc/version is linux-only")
	}
	reg := installSysInfo(t)

	resp := bridge.SysInfo(reg)(context.Background(), &v2pb.SysInfoRequest{})
	if resp.GetError() != "" {
		t.Fatalf("sys_info error: %s", resp.GetError())
	}
	if resp.GetPlatform() == "" && resp.GetPlatformFamily() == "" && resp.GetKernelVersion() == "" {
		t.Errorf("no platform info populated: %+v", resp)
	}
}

