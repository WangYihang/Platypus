package bridge_test

import (
	"context"
	"runtime"
	"testing"

	"github.com/WangYihang/Platypus/internal/agent/plugin/bridge"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// TestBridge_ListSecurityChecks_RealPluginEnumerates exercises the
// v2 sys-security plugin's check registry. v2 covers two checks
// (kernel.version + ssh.config); the legacy host-fn-backed
// implementation is gone.
func TestBridge_ListSecurityChecks_RealPluginEnumerates(t *testing.T) {
	reg := newRegWithSysPlugins(t)
	defer reg.Close(context.Background())

	resp := bridge.ListSecurityChecks(reg)(context.Background(), &v2pb.ListSecurityChecksRequest{})
	if resp.GetError() != "" {
		t.Fatalf("err: %s", resp.GetError())
	}
	got := map[string]bool{}
	for _, c := range resp.GetChecks() {
		got[c.GetId()] = true
	}
	for _, id := range []string{"kernel.version", "ssh.config"} {
		if !got[id] {
			t.Errorf("missing check %q in %+v", id, got)
		}
	}
}

// TestBridge_SecurityScan_RealPluginRuns runs the kernel.version
// check against /proc/sys/kernel/osrelease. Result depends on the
// host's kernel — both "no findings" (modern kernel) and "one finding"
// (kernel < 5.10, unlikely in any modern e2e env) are valid; we only
// assert the check ran (status="ok") + the response decoded.
func TestBridge_SecurityScan_RealPluginRuns(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("sys-security v2 reads /proc/sys/kernel/osrelease + /etc/ssh/sshd_config; meaningful only on linux")
	}
	reg := newRegWithSysPlugins(t)
	defer reg.Close(context.Background())

	resp := bridge.SecurityScan(reg)(context.Background(),
		&v2pb.SecurityScanRequest{CheckIds: []string{"kernel.version"}})
	if resp.GetError() != "" {
		t.Fatalf("err: %s", resp.GetError())
	}
	if len(resp.GetChecks()) != 1 || resp.GetChecks()[0].GetId() != "kernel.version" {
		t.Errorf("checks = %+v", resp.GetChecks())
	}
}
