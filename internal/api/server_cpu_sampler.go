package api

import (
	"context"
	"math"
	"os"
	"sync/atomic"
	"time"

	procinfo "github.com/shirou/gopsutil/v4/process"
)

// CPUPercent is the package-level hook the /api/v1/info handler reads
// to populate `cpu_percent`. Production wires this to a sampler's
// Percent method from cmd/platypus-server/main.go; tests that don't
// care leave the default no-op (returns 0) so the response shape
// stays stable.
//
// Kept as a function var rather than an interface to mirror the
// existing LiveAgentCounter / Counts pattern in handler_info_v1.go —
// no abstraction tax, no DI plumbing.
var CPUPercent func() float64 = func() float64 { return 0 }

// CPUSampler periodically samples the running process's CPU
// percentage so the /api/v1/info handler doesn't have to pay the
// gopsutil-blocks-for-1s tax on every poll. The percentage is
// per-core normalised (matches gopsutil/v4/process.Process.Percent's
// default), so it can exceed 100% under multi-core load — the
// status-bar tooltip explains that.
//
// Read concurrency is unbounded — Percent() decodes a single
// atomic.Uint64 (float64 bit pattern). Write concurrency is bounded
// to the single Start goroutine.
type CPUSampler struct {
	proc *procinfo.Process
	pct  atomic.Uint64 // float64 bit pattern; load with math.Float64frombits
}

// NewCPUSampler attaches to the current process. Returns an error if
// gopsutil refuses to look up the PID — extremely unlikely on the
// platforms we target, but we surface it so main.go can degrade
// gracefully rather than panicking on startup.
func NewCPUSampler() (*CPUSampler, error) {
	p, err := procinfo.NewProcess(int32(os.Getpid()))
	if err != nil {
		return nil, err
	}
	return &CPUSampler{proc: p}, nil
}

// Start launches a single goroutine that samples the process CPU%
// every second until ctx is cancelled. Each iteration calls
// proc.Percent(time.Second) which blocks for ~1s while it computes
// the delta against its own previous sample — that's why the work
// belongs in a background goroutine and not the request path.
//
// On error (process gone, permission denied — none expected on the
// happy path) we keep the previous value so transient failures
// don't blank the chip.
func (s *CPUSampler) Start(ctx context.Context) {
	go func() {
		for {
			pct, err := s.proc.PercentWithContext(ctx, time.Second)
			if ctx.Err() != nil {
				return
			}
			if err == nil && !math.IsNaN(pct) && pct >= 0 {
				s.pct.Store(math.Float64bits(pct))
			}
		}
	}()
}

// Percent returns the most recently sampled CPU percent. Safe to
// call concurrently. Returns 0 before the first sample lands.
func (s *CPUSampler) Percent() float64 {
	return math.Float64frombits(s.pct.Load())
}
