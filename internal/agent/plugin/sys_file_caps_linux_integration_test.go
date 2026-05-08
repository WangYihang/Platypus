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

func installSysFileCapsLinux(t *testing.T) *plugin.Registry {
	t.Helper()
	const id = "com.platypus.sys-file-caps-linux"
	const ver = "1.0.0"
	wasm := stagedWasmBytes(t, id, ver, "sys_file_caps_linux.wasm")
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
	sig, err := plugin.Sign(sk, wasm, plugin.DefaultTrustedComment("sys_file_caps_linux.wasm"))
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

// TestSysFileCapsLinux_Manifest sanity-checks the staged manifest:
// linux-only os_targets, one RPC, exec capability allowlists getcap
// on both Debian and RHEL paths plus /bin/sh for the existence probe.
func TestSysFileCapsLinux_Manifest(t *testing.T) {
	manifestBytes := stagedManifestBytes(t, "com.platypus.sys-file-caps-linux", "1.0.0")
	m, err := plugin.ParseManifest(manifestBytes)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := m.Runtime.OSTargets; len(got) != 1 || got[0] != "linux" {
		t.Errorf("os_targets = %v, want [linux]", got)
	}
	if len(m.RPC) != 1 || m.RPC[0].Name != "list_file_caps" {
		t.Errorf("rpc = %+v, want one entry named 'list_file_caps'", m.RPC)
	}
	if m.Capabilities.Exec == nil || len(m.Capabilities.Exec.Commands) != 3 {
		t.Errorf("exec capability mis-declared: %+v", m.Capabilities.Exec)
	}
}

// TestSysFileCapsLinux_TypedBridge_RoundTrip exercises bridge.FileCapsList
// against the staged plugin. The container's getcap is usually
// installed via libcap2-bin; if not, the response carries
// `getcap_not_installed` and we treat that as expected. The point of
// the test is to verify the proto round-trip + plugin install path,
// not to assert specific cap'd binaries on the runner.
func TestSysFileCapsLinux_TypedBridge_RoundTrip(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("sys-file-caps-linux is linux-only")
	}
	reg := installSysFileCapsLinux(t)

	resp := bridge.FileCapsList(reg)(context.Background(),
		&v2pb.FileCapsListRequest{IncludeAllowlisted: true})
	if got := resp.GetError(); got != "" && got != "getcap_not_installed" {
		t.Fatalf("typed bridge unexpected error: %s", got)
	}
	if resp.GetError() == "getcap_not_installed" {
		t.Skip("getcap not installed in test environment (apt install libcap2-bin)")
	}
	if resp.GetBackend() != "getcap" {
		t.Errorf("backend = %q, want getcap", resp.GetBackend())
	}
	for i, e := range resp.GetEntries() {
		if e.GetPath() == "" || e.GetCaps() == "" || e.GetRisk() == "" {
			t.Errorf("entry[%d] missing required field: %+v", i, e)
		}
		if e.GetRisk() != "low" && e.GetRisk() != "medium" && e.GetRisk() != "high" {
			t.Errorf("entry[%d].risk = %q, want low|medium|high", i, e.GetRisk())
		}
	}
	t.Logf("found %d cap'd binaries", len(resp.GetEntries()))
}

// TestSysFileCapsLinux_FiltersOutAllowlistedByDefault: the default
// (include_allowlisted=false) hides ping et al. Asserts ping is NOT
// in the response when the flag is off — useful sanity that the
// allowlist tagging actually works.
func TestSysFileCapsLinux_FiltersOutAllowlistedByDefault(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("sys-file-caps-linux is linux-only")
	}
	reg := installSysFileCapsLinux(t)

	resp := bridge.FileCapsList(reg)(context.Background(),
		&v2pb.FileCapsListRequest{})
	if resp.GetError() == "getcap_not_installed" {
		t.Skip("getcap not installed in test environment")
	}
	if resp.GetError() != "" {
		t.Fatalf("typed bridge: %s", resp.GetError())
	}
	for i, e := range resp.GetEntries() {
		if e.GetAllowlisted() {
			t.Errorf("entry[%d] is allowlisted but include_allowlisted=false hid nothing: %+v", i, e)
		}
	}
}
