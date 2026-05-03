package bridge_test

import (
	"context"
	"testing"

	"github.com/WangYihang/Platypus/internal/agent"
	"github.com/WangYihang/Platypus/internal/agent/plugin"
	"github.com/WangYihang/Platypus/internal/agent/plugin/bridge"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// TestBridge_ProcessList_RoundTripsProto wires the real
// CollectProcessList behind host_process_list (matching what main.go
// does at boot) and asserts the bridge round-trips the
// ProcessListResponse cleanly. Catches protojson encoding mismatches
// between the host fn and the bridge.
func TestBridge_ProcessList_RoundTripsProto(t *testing.T) {
	plugin.SetHostProcessListProvider(agent.CollectProcessList)
	t.Cleanup(func() {
		plugin.SetHostProcessListProvider(func(ctx context.Context, _ uint32, _ string) *v2pb.ProcessListResponse {
			return &v2pb.ProcessListResponse{Error: "test cleanup: provider reset"}
		})
	})

	reg := newRegWithSysPlugins(t)
	defer reg.Close(context.Background())

	resp := bridge.ProcessList(reg)(context.Background(),
		&v2pb.ProcessListRequest{TopN: 5, SortBy: "cpu"})
	if resp.GetError() != "" {
		t.Fatalf("process_list err: %s", resp.GetError())
	}
	// Sanity: this test process is running, so total count > 0.
	if resp.GetTotalCount() == 0 {
		t.Errorf("expected total_count > 0")
	}
	if len(resp.GetProcesses()) == 0 {
		t.Errorf("expected at least one process in top_n=5")
	}
}

func TestBridge_ProcessList_StubbedProviderError(t *testing.T) {
	// Inject a deterministic-error provider so we hit the failure
	// path of the bridge.
	plugin.SetHostProcessListProvider(func(ctx context.Context, _ uint32, _ string) *v2pb.ProcessListResponse {
		return &v2pb.ProcessListResponse{Error: "synthetic failure"}
	})
	t.Cleanup(func() {
		plugin.SetHostProcessListProvider(agent.CollectProcessList)
	})

	reg := newRegWithSysPlugins(t)
	defer reg.Close(context.Background())

	resp := bridge.ProcessList(reg)(context.Background(),
		&v2pb.ProcessListRequest{TopN: 1, SortBy: "cpu"})
	// Provider's error string flows through as the response's error
	// (the host fn wraps the proto via protojson regardless of
	// success/failure, so error text reaches the bridge intact).
	if resp.GetError() != "synthetic failure" {
		t.Errorf("error = %q, want synthetic failure", resp.GetError())
	}
}
