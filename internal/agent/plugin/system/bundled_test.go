package system

import (
	"context"
	"strings"
	"testing"

	"github.com/WangYihang/Platypus/internal/agent/plugin"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// TestBundled_SysHostnameInstallsAndInvokes is the integration test
// that proves the production system-plugin bootstrap pipeline works
// end-to-end against the real artefacts shipped in
// internal/agent/plugin/system/embedded/.
//
// Specifically:
//   - the embedded `publisher.pub` parses as a minisign pubkey
//   - the bundled sys-hostname plugin's signature verifies against it
//   - the auto-install path persists + hot-loads the plugin
//   - calling the wasm export round-trips through host_sysinfo to
//     return the host's hostname
//
// This is the closest thing in the test suite to "boot a real agent
// and watch its system plugins come online" — short of spawning a
// real agent process.
func TestBundled_SysHostnameInstallsAndInvokes(t *testing.T) {
	embFS, err := EmbeddedFS()
	if err != nil {
		t.Fatalf("EmbeddedFS: %v", err)
	}

	root := t.TempDir()
	reg, err := plugin.New(plugin.Options{Paths: plugin.NewPaths(root)})
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	defer reg.Close(context.Background())

	res := EnsureInstalled(context.Background(), reg, embFS)
	if res.SetupError != nil {
		t.Fatalf("setup err: %v", res.SetupError)
	}
	// At least one bundle must install (the sys-hostname one). Future
	// iterations may add more system plugins; this assertion stays
	// stable as long as sys-hostname is among them.
	wantID := "com.platypus.sys-hostname"
	found := false
	for _, b := range res.Installed {
		if b.ID == wantID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected %s in Installed, got %+v (failed=%+v)",
			wantID, res.Installed, res.Failed)
	}
	if !reg.HasInstalledVersion(wantID, "2.0.0") {
		t.Fatalf("expected %s v2.0.0 in catalog", wantID)
	}

	// Invoke. v2 returns {"hostname":"...","source":"/etc/hostname"}
	// (or "/proc/sys/kernel/hostname" fallback). The source field
	// records which filesystem path produced the value — useful in
	// audit trails distinguishing canonical vs fallback reads.
	resp := reg.Invoke(context.Background(), &v2pb.PluginCallRequest{
		PluginId: wantID,
		Method:   "hostname",
	})
	if resp.GetError() != "" {
		t.Fatalf("invoke err: %s", resp.GetError())
	}
	body := string(resp.GetPayload())
	if !strings.Contains(body, `"hostname"`) {
		t.Errorf("payload missing hostname field: %s", body)
	}
	// Source must reference one of the two filesystem paths the v2
	// plugin tries; "host_sysinfo" (the v1 marker) means we're
	// running an old bundle.
	if !strings.Contains(body, `"source":"/`) {
		t.Errorf("payload missing /-prefixed source field (v2 sources are filesystem paths): %s", body)
	}
	t.Logf("sys-hostname returned: %s", body)
}
