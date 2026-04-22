package agent

import (
	"context"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	gopshost "github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"

	agentpb "github.com/WangYihang/Platypus/pkg/proto/agent/v1"
)

// SysInfoSampleInterval is the cadence at which the agent pushes a
// fresh SysInfoResponse to the server when sysinfo streaming is
// enabled. Kept low enough to drive Topology CPU/mem gauges, high
// enough to stay cheap on busy hosts (gopsutil samples run in < 5ms).
const SysInfoSampleInterval = 30 * time.Second

// CollectSysInfo takes a single best-effort snapshot of operating-
// system metrics. Every field is independently recoverable: a failure
// in one sub-call (say, cpu.PercentWithContext on a locked-down WSL)
// leaves other fields populated. Callers should not treat a returned
// error as fatal — a partial SysInfo is still useful for display.
func CollectSysInfo(ctx context.Context) (*agentpb.SysInfo, error) {
	info := &agentpb.SysInfo{
		SampledAtUnix: time.Now().Unix(),
	}

	// Short-interval CPU sample. 500ms gives a meaningful busy/idle
	// read without adding noticeable latency to the RPC. Uses the
	// aggregated "all cores" percentage.
	if pcts, err := cpu.PercentWithContext(ctx, 500*time.Millisecond, false); err == nil && len(pcts) > 0 {
		info.CpuPercent = pcts[0]
	}

	if vm, err := mem.VirtualMemoryWithContext(ctx); err == nil && vm != nil {
		info.MemPercent = vm.UsedPercent
		info.MemTotalBytes = vm.Total
		info.MemUsedBytes = vm.Used
	}

	if h, err := gopshost.InfoWithContext(ctx); err == nil && h != nil {
		info.KernelVersion = h.KernelVersion
		info.Platform = h.Platform
		info.PlatformVersion = h.PlatformVersion
		info.UptimeSeconds = h.Uptime
		// "Ubuntu 22.04.4 LTS" style — gopsutil reports
		// PlatformVersion as just the version number; combine with
		// the distribution name for human readability.
		if h.Platform != "" && h.PlatformVersion != "" {
			info.OsDistribution = h.Platform + " " + h.PlatformVersion
		} else if h.Platform != "" {
			info.OsDistribution = h.Platform
		}
	}

	return info, nil
}

// StartSysInfoPusher starts a background goroutine that periodically
// collects a SysInfo snapshot and pushes it to the server via the
// supplied client. It returns immediately; the goroutine exits when
// stop is closed. Safe to call even if the peer doesn't understand
// SysInfoResponse (the envelope is a no-op on older servers).
func StartSysInfoPusher(c *Client, stop <-chan struct{}) {
	go func() {
		// First sample right away so the server sees CPU/mem within
		// the first second of the connection, not 30 s later.
		pushOnce(c)

		ticker := time.NewTicker(SysInfoSampleInterval)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				pushOnce(c)
			}
		}
	}()
}

func pushOnce(c *Client) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	info, _ := CollectSysInfo(ctx)
	if info == nil {
		return
	}
	send(c, &agentpb.Envelope{
		Payload: &agentpb.Envelope_SysInfoResponse{
			SysInfoResponse: &agentpb.SysInfoResponse{Info: info},
		},
	})
}
