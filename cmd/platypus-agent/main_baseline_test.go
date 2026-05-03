package main

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/WangYihang/Platypus/internal/agent"
)

// silentLogger discards everything. Tests don't want the agent's
// structured slog spew on stderr but the helper takes a real
// *slog.Logger; satisfy the API without flooding test output.
func silentLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// TestResolveBaselineAllowlist_FirstBootPersists exercises the
// "no baseline.json on disk yet, opts says install sys-listdir" path:
//
//   - the function returns sys-listdir + sys-info (the mandatory core)
//   - baseline.json is written so subsequent boots take the persisted path
func TestResolveBaselineAllowlist_FirstBootPersists(t *testing.T) {
	dir := t.TempDir()
	got := resolveBaselineAllowlist(silentLogger(), dir, []string{"com.platypus.sys-listdir"})
	want := []string{"com.platypus.sys-listdir", "com.platypus.sys-info"}
	if !equalStrSlice(got, want) {
		t.Fatalf("resolveBaselineAllowlist = %v; want %v", got, want)
	}
	persisted, err := agent.LoadBaseline(dir)
	if err != nil {
		t.Fatalf("baseline.json was not persisted: %v", err)
	}
	if !equalStrSlice(persisted, []string{"com.platypus.sys-listdir"}) {
		t.Fatalf("persisted = %v; want only operator pick (mandatory core lives in the runtime merge, not the file)", persisted)
	}
}

// TestResolveBaselineAllowlist_SteadyStateIgnoresOpts: once
// baseline.json exists, the install bundle / CLI flag is ignored.
// Tests the security-relevant "operator decision is sticky" path —
// a re-run with stale opts cannot silently widen the allowlist.
func TestResolveBaselineAllowlist_SteadyStateIgnoresOpts(t *testing.T) {
	dir := t.TempDir()
	if err := agent.SaveBaseline(dir, []string{"com.platypus.sys-listdir"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	got := resolveBaselineAllowlist(silentLogger(), dir,
		[]string{"com.platypus.sys-procs", "com.platypus.sys-exec"})
	want := []string{"com.platypus.sys-listdir", "com.platypus.sys-info"}
	if !equalStrSlice(got, want) {
		t.Fatalf("resolveBaselineAllowlist = %v; want %v (persisted baseline must win)", got, want)
	}
}

// TestResolveBaselineAllowlist_EmptyFirstBootStillCreatesFile: on
// first boot with no operator pick, the function falls back to
// mandatory-core-only AND persists an empty baseline.json so future
// boots stop re-evaluating opts.
func TestResolveBaselineAllowlist_EmptyFirstBootStillCreatesFile(t *testing.T) {
	dir := t.TempDir()
	got := resolveBaselineAllowlist(silentLogger(), dir, nil)
	if !equalStrSlice(got, []string{"com.platypus.sys-info"}) {
		t.Fatalf("resolveBaselineAllowlist = %v; want sys-info only", got)
	}
	persisted, err := agent.LoadBaseline(dir)
	if err != nil {
		t.Fatalf("baseline.json was not persisted: %v", err)
	}
	if len(persisted) != 0 {
		t.Fatalf("persisted = %v; want empty (operator picked nothing)", persisted)
	}
}

// TestResolveBaselineAllowlist_CorruptFileFallsBack: a corrupt
// baseline.json must produce mandatory-core-only, NOT silently
// re-read opts (which would let a truncated file silently re-enable
// plugins the operator removed).
func TestResolveBaselineAllowlist_CorruptFileFallsBack(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "baseline.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatalf("seed corrupt: %v", err)
	}
	got := resolveBaselineAllowlist(silentLogger(), dir,
		[]string{"com.platypus.sys-procs"})
	if !equalStrSlice(got, []string{"com.platypus.sys-info"}) {
		t.Fatalf("corrupt-file path returned %v; want sys-info only (no opts fallback)", got)
	}
}
