package agent

import (
	"context"
	"sort"
	"strings"
	"time"

	procinfo "github.com/shirou/gopsutil/v4/process"

	"github.com/WangYihang/Platypus/internal/log"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// slowProbeMs is the per-process snapshot threshold above which we
// emit a debug line identifying the offending pid. Useful when a
// single wedged /proc/<pid>/... read dominates wallclock — the log
// pinpoints the pid without needing strace.
const slowProbeMs = 100

// processListCap is the hard ceiling on how many entries the agent
// will ever ship in a single ProcessListResponse. Even with top_n=0
// ("all"), we clip to this to keep the stream frame bounded.
const processListCap = 500

// cmdlineTruncateBytes bounds the per-process command line sent to
// the UI. Long container / java cmdlines can exceed 64 KB; we don't
// need the full text for the overview table.
const cmdlineTruncateBytes = 512

// CollectProcessList returns a snapshot of running processes,
// sorted by the requested key and truncated to top_n entries.
// Every per-process probe is guarded by a 200ms context so a
// single wedged /proc entry doesn't stall the collection. The
// returned response always carries total_count (pre-truncation).
func CollectProcessList(ctx context.Context, top uint32, sortBy string) *v2pb.ProcessListResponse {
	t0 := time.Now()
	resp := &v2pb.ProcessListResponse{}
	procs, err := procinfo.ProcessesWithContext(ctx)
	if err != nil {
		log.L.Warn("process_list_enumerate_failed",
			"error", err.Error(),
			"elapsed_ms", time.Since(t0).Milliseconds(),
		)
		resp.Error = err.Error()
		return resp
	}
	tEnumerate := time.Now()
	resp.TotalCount = uint32(len(procs))
	log.L.Info("process_list_enumerated",
		"count", len(procs),
		"elapsed_ms", tEnumerate.Sub(t0).Milliseconds(),
	)

	var slowCount int
	out := make([]*v2pb.ProcessInfo, 0, len(procs))
	for _, p := range procs {
		if p == nil {
			continue
		}
		probeStart := time.Now()
		info := snapshotProcess(ctx, p)
		if d := time.Since(probeStart); d.Milliseconds() > slowProbeMs {
			slowCount++
			log.L.Debug("process_list_slow_probe",
				"pid", p.Pid,
				"elapsed_ms", d.Milliseconds(),
			)
		}
		if info == nil {
			continue
		}
		out = append(out, info)
	}
	tSnapshot := time.Now()

	sortProcesses(out, sortBy)
	tSort := time.Now()

	limit := int(top)
	if limit <= 0 || limit > processListCap {
		limit = processListCap
	}
	if len(out) > limit {
		out = out[:limit]
	}
	resp.Processes = out

	log.L.Info("process_list_done",
		"total", resp.TotalCount,
		"returned", len(resp.Processes),
		"slow_probes", slowCount,
		"enumerate_ms", tEnumerate.Sub(t0).Milliseconds(),
		"snapshot_ms", tSnapshot.Sub(tEnumerate).Milliseconds(),
		"sort_ms", tSort.Sub(tSnapshot).Milliseconds(),
		"total_ms", time.Since(t0).Milliseconds(),
	)
	return resp
}

// snapshotProcess reads the fields we care about from a single
// gopsutil process handle. Each field read is wrapped in its own
// short-lived context so a blocked /proc/<pid>/... read at most
// stalls this one probe, not the whole enumeration. Processes that
// disappear mid-read (pid already gone) simply return whatever we
// managed to collect.
func snapshotProcess(parent context.Context, p *procinfo.Process) *v2pb.ProcessInfo {
	info := &v2pb.ProcessInfo{Pid: uint32(p.Pid)}

	probe := func(fn func(ctx context.Context)) {
		ctx, cancel := context.WithTimeout(parent, 200*time.Millisecond)
		defer cancel()
		defer func() { _ = recover() }()
		fn(ctx)
	}

	probe(func(ctx context.Context) {
		if ppid, err := p.PpidWithContext(ctx); err == nil {
			info.Ppid = uint32(ppid)
		}
	})
	probe(func(ctx context.Context) {
		if u, err := p.UsernameWithContext(ctx); err == nil {
			info.User = u
		}
	})
	probe(func(ctx context.Context) {
		if n, err := p.NameWithContext(ctx); err == nil {
			info.Name = n
		}
	})
	probe(func(ctx context.Context) {
		if c, err := p.CmdlineWithContext(ctx); err == nil {
			info.Cmdline = truncateCmdline(c)
		}
	})
	probe(func(ctx context.Context) {
		if s, err := p.StatusWithContext(ctx); err == nil {
			info.Status = strings.Join(s, ",")
		}
	})
	probe(func(ctx context.Context) {
		if cpu, err := p.CPUPercentWithContext(ctx); err == nil {
			info.CpuPercent = cpu
		}
	})
	probe(func(ctx context.Context) {
		if mem, err := p.MemoryPercentWithContext(ctx); err == nil {
			info.MemPercent = float64(mem)
		}
	})
	probe(func(ctx context.Context) {
		if m, err := p.MemoryInfoWithContext(ctx); err == nil && m != nil {
			info.RssBytes = m.RSS
		}
	})
	probe(func(ctx context.Context) {
		if n, err := p.NumThreadsWithContext(ctx); err == nil {
			info.NumThreads = uint32(n)
		}
	})
	probe(func(ctx context.Context) {
		if t, err := p.CreateTimeWithContext(ctx); err == nil {
			// gopsutil returns milliseconds since epoch; normalize
			// to seconds so the UI can feed it straight into Date().
			info.CreatedAtUnix = t / 1000
		}
	})

	if info.Pid == 0 && info.Name == "" {
		return nil
	}
	return info
}

// sortProcesses orders the slice in place by the requested key.
// Unknown values fall back to CPU-desc, matching the documented
// default for ProcessListRequest.sort_by.
func sortProcesses(out []*v2pb.ProcessInfo, sortBy string) {
	key := strings.ToLower(strings.TrimSpace(sortBy))
	switch key {
	case "mem":
		sort.SliceStable(out, func(i, j int) bool {
			return out[i].MemPercent > out[j].MemPercent
		})
	case "rss":
		sort.SliceStable(out, func(i, j int) bool {
			return out[i].RssBytes > out[j].RssBytes
		})
	case "pid":
		sort.SliceStable(out, func(i, j int) bool {
			return out[i].Pid < out[j].Pid
		})
	default: // "cpu" and anything unrecognized
		sort.SliceStable(out, func(i, j int) bool {
			return out[i].CpuPercent > out[j].CpuPercent
		})
	}
}

func truncateCmdline(s string) string {
	if len(s) <= cmdlineTruncateBytes {
		return s
	}
	return s[:cmdlineTruncateBytes] + "…"
}
