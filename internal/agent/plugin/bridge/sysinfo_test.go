package bridge_test

import (
	"context"
	"runtime"
	"testing"

	"github.com/WangYihang/Platypus/internal/agent"
	"github.com/WangYihang/Platypus/internal/agent/plugin"
	"github.com/WangYihang/Platypus/internal/agent/plugin/bridge"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

func TestBridge_SysInfo_RoundTripsProto(t *testing.T) {
	plugin.SetHostCollectSysInfoProvider(agent.CollectSysInfo)
	t.Cleanup(func() {
		plugin.SetHostCollectSysInfoProvider(nil) // reset to default
	})

	reg := newRegWithSysPlugins(t)
	defer reg.Close(context.Background())

	resp := bridge.SysInfo(reg)(context.Background(), &v2pb.SysInfoRequest{})
	if resp.GetError() != "" {
		t.Fatalf("sys_info err: %s", resp.GetError())
	}
	// Sanity: a few mandatory fields the legacy CollectSysInfo always
	// fills — os/arch from runtime, hostname non-empty.
	if resp.GetOs() != runtime.GOOS {
		t.Errorf("os = %q, want %q", resp.GetOs(), runtime.GOOS)
	}
	if resp.GetArch() != runtime.GOARCH {
		t.Errorf("arch = %q, want %q", resp.GetArch(), runtime.GOARCH)
	}
	if resp.GetHostname() == "" {
		t.Errorf("hostname is empty")
	}
}
