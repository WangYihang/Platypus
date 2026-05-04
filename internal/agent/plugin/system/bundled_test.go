package system

import (
	"context"
	"strings"
	"testing"

	"github.com/WangYihang/Platypus/internal/agent/plugin"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// TestBundled_SysInfoInstallsAndInvokes is the integration test
// that proves the production system-plugin bootstrap pipeline works
// end-to-end against the real artefacts shipped in
// internal/agent/plugin/system/embedded/.
//
// Specifically:
//   - the embedded `publisher.pub` parses as a minisign pubkey
//   - the bundled sys-info plugin's signature verifies against it
//   - the auto-install path persists + hot-loads the plugin
//   - calling the wasm export round-trips through host_uname /
//     host_fs_read to return a populated SysInfoResponse (which
//     covers what sys-hostname used to expose as a separate plugin)
//
// This is the closest thing in the test suite to "boot a real agent
// and watch its system plugins come online" — short of spawning a
// real agent process. We picked sys-info as the canary because it's
// the mandatory-core plugin every agent installs regardless of the
// operator's baseline allowlist.
func TestBundled_SysInfoInstallsAndInvokes(t *testing.T) {
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

	res := EnsureInstalled(context.Background(), reg, embFS, EnsureOptions{AllowAll: true})
	if res.SetupError != nil {
		t.Fatalf("setup err: %v", res.SetupError)
	}
	wantID := "com.platypus.sys-info"
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

	// Invoke. SysInfoResponse carries the hostname field that
	// sys-hostname used to surface as a standalone plugin —
	// merging the two means hostname is now read in the same call
	// that fetches kernel / mem / cpu / load / uptime.
	resp := reg.Invoke(context.Background(), &v2pb.PluginCallRequest{
		PluginId: wantID,
		Method:   "sys_info",
	})
	if resp.GetError() != "" {
		t.Fatalf("invoke err: %s", resp.GetError())
	}
	body := string(resp.GetPayload())
	if !strings.Contains(body, `"hostname"`) {
		t.Errorf("payload missing hostname field (sys-info should fold in what sys-hostname used to expose): %s", body)
	}
	t.Logf("sys-info returned: %s", body)
}
