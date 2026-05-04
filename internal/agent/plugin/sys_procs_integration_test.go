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

func installSysProcs(t *testing.T) *plugin.Registry {
	t.Helper()
	wasm := stagedWasmBytes(t, "com.platypus.sys-procs", "2.0.0", "sys_procs.wasm")
	manifestBytes := stagedManifestBytes(t, "com.platypus.sys-procs", "2.0.0")

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
	sig, err := plugin.Sign(sk, wasm, plugin.DefaultTrustedComment("sys_procs.wasm"))
	if err != nil {
		t.Fatal(err)
	}
	reg, err := plugin.New(plugin.Options{Paths: paths})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { reg.Close(context.Background()) })

	if err := reg.InstallFromBytes(context.Background(), plugin.InstallParams{
		PluginID:            "com.platypus.sys-procs",
		Version:             "2.0.0",
		PublisherPubkey:     []byte(plugin.EncodePublicKey(pk, "")),
		Manifest:            []byte(manifestStr),
		Wasm:                wasm,
		Signature:           []byte(plugin.EncodeSignature(sig)),
		Actor:               "test",
		GrantedCapabilities: []string{"fs.read"},
	}, nil); err != nil {
		t.Fatal(err)
	}
	return reg
}

// TestProcs_ReturnsCurrentProcess: after the plugin enumerates /proc
// it should at least see the test runner itself. Asserts the
// running pid is in the response (via the test process's own PID).
func TestProcs_ReturnsCurrentProcess(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("/proc enumeration is linux-only")
	}
	reg := installSysProcs(t)

	resp := bridge.ProcessList(reg)(context.Background(), &v2pb.ProcessListRequest{
		TopN: 0, // 0 = unlimited
	})
	if resp.GetError() != "" {
		t.Fatalf("process_list error: %s", resp.GetError())
	}
	if len(resp.GetProcesses()) == 0 {
		t.Fatal("expected at least one process; got empty list")
	}
	myPID := uint32(os.Getpid())
	found := false
	for _, p := range resp.GetProcesses() {
		if p.GetPid() == myPID {
			found = true
			if p.GetName() == "" {
				t.Errorf("self process has empty name")
			}
			break
		}
	}
	if !found {
		t.Errorf("self pid %d not in process list (count=%d)", myPID, len(resp.GetProcesses()))
	}
}

// TestProcs_TopNCapsResultLength: setting top_n=5 should return at
// most 5 processes; total_count should still report the full count
// so the UI can render "5 of N".
func TestProcs_TopNCapsResultLength(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("/proc enumeration is linux-only")
	}
	reg := installSysProcs(t)

	resp := bridge.ProcessList(reg)(context.Background(), &v2pb.ProcessListRequest{
		TopN:   5,
		SortBy: "pid",
	})
	if resp.GetError() != "" {
		t.Fatalf("process_list error: %s", resp.GetError())
	}
	if got := len(resp.GetProcesses()); got > 5 {
		t.Errorf("got %d processes; want ≤ 5", got)
	}
	if resp.GetTotalCount() < uint32(len(resp.GetProcesses())) {
		t.Errorf("total_count %d < returned len %d", resp.GetTotalCount(), len(resp.GetProcesses()))
	}
}

