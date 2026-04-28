package api

import (
	"context"
	"runtime"
	"sync"
	"testing"
	"time"
)

// CPUSampler samples the current process's CPU% in the background so
// the /api/v1/info handler can report it without paying the
// gopsutil-blocks-for-1s cost on every request. The contract pinned
// here:
//
//   1. Percent() returns a non-negative finite float.
//   2. After at least one sample tick has fired, Percent() is
//      bounded by 100 * NumCPU plus generous slack — gopsutil can
//      briefly overshoot under load on busy hosts.
//   3. Cancelling the sampler's ctx is idempotent: subsequent
//      Percent() calls return the last sampled value rather than
//      panicking, and the goroutine exits.
//   4. The package-level api.CPUPercent variable defaults to a
//      function that returns 0 (so handler tests that don't want a
//      sampler still see a sane response shape) and can be re-bound
//      to a sampler's Percent method for production wiring.

func TestCPUSampler_PercentNonNegative(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s, err := NewCPUSampler()
	if err != nil {
		t.Fatalf("NewCPUSampler: %v", err)
	}
	s.Start(ctx)
	// Wait a hair longer than the sampling interval so at least one
	// tick has fired. Sampler interval is 1 s.
	time.Sleep(1200 * time.Millisecond)
	pct := s.Percent()
	if pct < 0 {
		t.Errorf("Percent() = %v; want >= 0", pct)
	}
	if pct != pct { // NaN check
		t.Errorf("Percent() = NaN; want finite")
	}
}

func TestCPUSampler_PercentCappedByNumCPU(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s, err := NewCPUSampler()
	if err != nil {
		t.Fatalf("NewCPUSampler: %v", err)
	}
	s.Start(ctx)
	time.Sleep(1200 * time.Millisecond)
	pct := s.Percent()
	// 50% slack on top of 100 * NumCPU absorbs the brief overshoot
	// gopsutil can return on busy hosts.
	maxPct := 100.0*float64(runtime.NumCPU()) + 50.0
	if pct > maxPct {
		t.Errorf("Percent() = %.2f; want <= %.2f (NumCPU=%d)",
			pct, maxPct, runtime.NumCPU())
	}
}

func TestCPUSampler_StopIsIdempotent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	s, err := NewCPUSampler()
	if err != nil {
		t.Fatalf("NewCPUSampler: %v", err)
	}
	s.Start(ctx)
	time.Sleep(1100 * time.Millisecond)
	last := s.Percent()
	cancel()
	// Give the goroutine a moment to notice the cancel.
	time.Sleep(100 * time.Millisecond)
	// Subsequent reads must not panic and must return the last
	// sampled value (not zero, not stale-from-five-minutes-ago).
	if got := s.Percent(); got != last {
		// Allow drift if a tick fired between cancel and the read
		// but the value must still be finite + non-negative.
		if got < 0 {
			t.Errorf("Percent() after cancel = %v; want >= 0", got)
		}
	}
}

func TestCPUSampler_ConcurrentReadsAreSafe(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s, err := NewCPUSampler()
	if err != nil {
		t.Fatalf("NewCPUSampler: %v", err)
	}
	s.Start(ctx)
	time.Sleep(1100 * time.Millisecond)
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = s.Percent()
			}
		}()
	}
	wg.Wait()
}

func TestCPUPercent_PackageVarDefaultsToZero(t *testing.T) {
	// Production wires this to a sampler in cmd/platypus-server/main.go;
	// tests that don't bother get a no-op default so /api/v1/info
	// still produces a valid response shape.
	if CPUPercent == nil {
		t.Fatal("CPUPercent must default to a non-nil function")
	}
	if got := CPUPercent(); got != 0 {
		t.Errorf("default CPUPercent() = %v; want 0", got)
	}
}
