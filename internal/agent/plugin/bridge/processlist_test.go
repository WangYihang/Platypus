package bridge_test

import (
	"context"
	"runtime"
	"testing"

	"github.com/WangYihang/Platypus/internal/agent/plugin/bridge"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// TestBridge_ProcessList_RealPluginEnumeratesProc exercises the v2
// sys-procs plugin end-to-end. The plugin walks /proc/<pid> dirs +
// reads stat/status/cmdline directly via host_fs_*; the agent-side
// gopsutil-backed CollectProcessList is no longer in the path.
//
// Linux is the only host where /proc enumeration is meaningful.
// On other OSes the plugin returns total_count=0 and an empty list,
// which is "graceful degradation" not a failure — the bridge's
// own behaviour stays correct.
func TestBridge_ProcessList_RealPluginEnumeratesProc(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("sys-procs v2 reads /proc; meaningful only on linux")
	}
	reg := newRegWithSysPlugins(t)
	defer reg.Close(context.Background())

	resp := bridge.ProcessList(reg)(context.Background(),
		&v2pb.ProcessListRequest{TopN: 5, SortBy: "rss"})
	if resp.GetError() != "" {
		t.Fatalf("err: %s", resp.GetError())
	}
	if resp.GetTotalCount() == 0 {
		t.Errorf("expected total_count > 0")
	}
	if len(resp.GetProcesses()) == 0 {
		t.Errorf("expected at least one process in top_n=5")
	}
	// Each process row must have a pid; status field is the single
	// char from /proc/<pid>/stat field 3 (R/S/D/Z/...).
	for i, p := range resp.GetProcesses() {
		if p.GetPid() == 0 {
			t.Errorf("process[%d] has pid=0: %+v", i, p)
		}
		if p.GetName() == "" {
			t.Errorf("process[%d] has empty name", i)
		}
	}
}
