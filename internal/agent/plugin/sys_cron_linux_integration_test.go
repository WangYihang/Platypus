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

func installSysCronLinux(t *testing.T) *plugin.Registry {
	t.Helper()
	const id = "com.platypus.sys-cron-linux"
	const ver = "1.0.0"
	wasm := stagedWasmBytes(t, id, ver, "sys_cron_linux.wasm")
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
	sig, err := plugin.Sign(sk, wasm, plugin.DefaultTrustedComment("sys_cron_linux.wasm"))
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

func TestSysCronLinux_Manifest(t *testing.T) {
	manifestBytes := stagedManifestBytes(t, "com.platypus.sys-cron-linux", "1.0.0")
	m, err := plugin.ParseManifest(manifestBytes)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := m.Runtime.OSTargets; len(got) != 1 || got[0] != "linux" {
		t.Errorf("os_targets = %v, want [linux]", got)
	}
	if len(m.RPC) != 1 || m.RPC[0].Name != "list_cron_jobs" {
		t.Errorf("rpc = %+v", m.RPC)
	}
	if m.Capabilities.FSRead == nil || len(m.Capabilities.FSRead.Paths) != 2 {
		t.Errorf("fs.read mis-declared: %+v", m.Capabilities.FSRead)
	}
}

// TestSysCronLinux_TypedBridge_RoundTrip exercises bridge.CronList
// against the staged plugin. Containers may have nothing scheduled
// at all (no /etc/crontab, no /etc/cron.d entries) — that's a clean
// empty response, not a failure. The point of the test is the proto
// round-trip + plugin install path.
func TestSysCronLinux_TypedBridge_RoundTrip(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("sys-cron-linux is linux-only")
	}
	reg := installSysCronLinux(t)

	resp := bridge.CronList(reg)(context.Background(),
		&v2pb.CronListRequest{IncludeDisabled: true})
	if resp.GetError() != "" {
		t.Fatalf("typed bridge returned error: %s", resp.GetError())
	}
	for i, j := range resp.GetJobs() {
		if j.GetCommand() == "" {
			t.Errorf("job[%d] empty command: %+v", i, j)
		}
		if j.GetSource() == "" {
			t.Errorf("job[%d] empty source: %+v", i, j)
		}
		switch j.GetKind() {
		case "crontab", "system_crontab", "cron_d", "run_parts", "anacron":
			// ok
		default:
			t.Errorf("job[%d] unexpected kind %q: %+v", i, j.GetKind(), j)
		}
	}
	t.Logf("found %d cron jobs", len(resp.GetJobs()))
}
