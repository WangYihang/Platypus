package bridge_test

import (
	"context"
	"runtime"
	"testing"

	"github.com/WangYihang/Platypus/internal/agent/plugin/bridge"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// TestBridge_SysInfo_RealPluginReadsProc exercises the v2 sys-info
// plugin end-to-end. The plugin reads /proc + /etc + /sys directly
// via host_fs_read + host_uname (no agent-side gopsutil involved
// anymore). On a Linux test host every probe in the assertion list
// is reachable; on macOS/Windows CI the probe set degrades
// gracefully (kernel/load/cpu fields stay zero) so we only assert
// the always-derivable ones: os/arch from host_uname.
func TestBridge_SysInfo_RealPluginReadsProc(t *testing.T) {
	reg := newRegWithSysPlugins(t)
	defer reg.Close(context.Background())

	resp := bridge.SysInfo(reg)(context.Background(), &v2pb.SysInfoRequest{})
	if resp.GetError() != "" {
		t.Fatalf("sys_info err: %s", resp.GetError())
	}
	// host_uname always returns runtime.GOOS / GOARCH so these are
	// the cross-platform invariants.
	if resp.GetOs() != runtime.GOOS {
		t.Errorf("os = %q, want %q", resp.GetOs(), runtime.GOOS)
	}
	if resp.GetArch() != runtime.GOARCH {
		t.Errorf("arch = %q, want %q", resp.GetArch(), runtime.GOARCH)
	}
	// On Linux the /proc-derived basics MUST be populated. Skip on
	// other OSes — the plugin gracefully leaves them empty.
	if runtime.GOOS == "linux" {
		if resp.GetHostname() == "" {
			t.Errorf("expected non-empty hostname on linux")
		}
		if resp.GetKernelVersion() == "" {
			t.Errorf("expected non-empty kernel_version on linux")
		}
		if resp.GetMemTotal() == 0 {
			t.Errorf("expected non-zero mem_total on linux")
		}
		if resp.GetNumCpu() == 0 {
			t.Errorf("expected non-zero num_cpu on linux")
		}
		if resp.GetProcessCount() == 0 {
			t.Errorf("expected non-zero process_count on linux")
		}
	}
}
