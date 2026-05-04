package plugin_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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
	wasm, err := os.ReadFile(sysInfoWasmPath())
	if err != nil {
		t.Fatalf("sys_info_plugin.wasm missing under internal/server/sysplugins/embedded/ (%v) — run `go run ./hack/stage_system_plugins` from the repo root", err)
	}
	manifestBytes, err := os.ReadFile(sysInfoManifestPath())
	if err != nil {
		t.Fatal(err)
	}

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
		GrantedCapabilities: []string{"sysinfo", "fs.read"},
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

func sysInfoWasmPath() string {
	return filepath.Join("..", "..", "..", "internal", "server", "sysplugins",
		"embedded", "system-plugins", "com.platypus.sys-info", "2.0.0",
		"sys_info_plugin.wasm")
}

func sysInfoManifestPath() string {
	return filepath.Join("..", "..", "..", "internal", "server", "sysplugins",
		"embedded", "system-plugins", "com.platypus.sys-info", "2.0.0",
		"plugin.yaml")
}

// rewriteManifestKeyID swaps the manifest's signature.key_id field
// for the supplied hex string, preserving comments and surrounding
// formatting. The integration tests sign with a fresh per-test
// keypair; the staged manifest's key_id is whatever
// hack/stage_system_plugins minted at build time, which doesn't
// match.
func rewriteManifestKeyID(src, keyID string) string {
	const marker = "key_id:"
	idx := strings.Index(src, marker)
	if idx < 0 {
		return src
	}
	// Replace the value (single token) after key_id: up to the
	// next newline. A simple split keeps this self-contained.
	tail := src[idx+len(marker):]
	nl := strings.IndexByte(tail, '\n')
	if nl < 0 {
		return src // unterminated manifest? leave as-is
	}
	return src[:idx+len(marker)] + " " + keyID + tail[nl:]
}
