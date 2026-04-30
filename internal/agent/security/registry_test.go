package security

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeChecker struct {
	id, cat   string
	app       bool
	findings  []Finding
	err       error
	panicWith any
	called    int
}

func (f *fakeChecker) ID() string                        { return f.id }
func (f *fakeChecker) Category() string                  { return f.cat }
func (f *fakeChecker) Applicable(_ context.Context) bool { return f.app }
func (f *fakeChecker) Metadata() CheckMetadata           { return CheckMetadata{Title: f.id} }
func (f *fakeChecker) Run(_ context.Context) ([]Finding, error) {
	f.called++
	if f.panicWith != nil {
		panic(f.panicWith)
	}
	return f.findings, f.err
}

// withIsolatedRegistry swaps in a fresh registry for a test and
// restores the original on cleanup, so tests can Register without
// stepping on each other or on the production checker set.
func withIsolatedRegistry(t *testing.T) {
	t.Helper()
	registryMu.Lock()
	saved := registry
	registry = map[string]Checker{}
	registryMu.Unlock()
	t.Cleanup(func() {
		registryMu.Lock()
		registry = saved
		registryMu.Unlock()
	})
}

func TestScan_RunsAllApplicable(t *testing.T) {
	withIsolatedRegistry(t)
	a := &fakeChecker{id: "a", cat: "kernel", app: true, findings: []Finding{{ID: "a.f1", Severity: SeverityHigh}}}
	b := &fakeChecker{id: "b", cat: "ssh", app: true}
	c := &fakeChecker{id: "c", cat: "ssh", app: false}
	Register(a)
	Register(b)
	Register(c)

	res := Scan(context.Background(), ScanOptions{})

	if a.called != 1 || b.called != 1 {
		t.Fatalf("expected applicable checkers to run once, got a=%d b=%d", a.called, b.called)
	}
	if c.called != 0 {
		t.Fatalf("non-applicable checker should not run, got c=%d", c.called)
	}
	if len(res.Findings) != 1 || res.Findings[0].ID != "a.f1" {
		t.Fatalf("unexpected findings: %+v", res.Findings)
	}
	if len(res.Checks) != 3 {
		t.Fatalf("expected per-check result for every registered checker, got %d", len(res.Checks))
	}
	statuses := map[string]string{}
	for _, cr := range res.Checks {
		statuses[cr.ID] = cr.Status
	}
	if statuses["a"] != StatusOK || statuses["b"] != StatusOK || statuses["c"] != StatusSkipped {
		t.Fatalf("unexpected statuses: %+v", statuses)
	}
}

func TestScan_FilterByIDAndCategory(t *testing.T) {
	withIsolatedRegistry(t)
	Register(&fakeChecker{id: "a", cat: "kernel", app: true})
	Register(&fakeChecker{id: "b", cat: "ssh", app: true})
	Register(&fakeChecker{id: "c", cat: "fs", app: true})

	res := Scan(context.Background(), ScanOptions{
		CheckIDs:   []string{"a"},
		Categories: []string{"ssh"},
	})

	got := map[string]bool{}
	for _, cr := range res.Checks {
		got[cr.ID] = true
	}
	if !got["a"] || !got["b"] || got["c"] {
		t.Fatalf("expected {a,b}, got %+v", got)
	}
}

func TestScan_CheckerErrorBecomesErrorStatus(t *testing.T) {
	withIsolatedRegistry(t)
	Register(&fakeChecker{id: "x", cat: "kernel", app: true, err: errors.New("boom")})
	res := Scan(context.Background(), ScanOptions{})
	if len(res.Checks) != 1 || res.Checks[0].Status != StatusError || res.Checks[0].Error != "boom" {
		t.Fatalf("unexpected: %+v", res.Checks)
	}
	if len(res.Findings) != 0 {
		t.Fatalf("error should not contribute findings")
	}
}

func TestScan_PanicInCheckerIsContained(t *testing.T) {
	withIsolatedRegistry(t)
	Register(&fakeChecker{id: "x", cat: "kernel", app: true, panicWith: "wat"})
	Register(&fakeChecker{id: "y", cat: "kernel", app: true, findings: []Finding{{ID: "y.ok"}}})
	res := Scan(context.Background(), ScanOptions{})
	if len(res.Checks) != 2 {
		t.Fatalf("both checkers should be reported, got %d", len(res.Checks))
	}
	var xErr string
	for _, cr := range res.Checks {
		if cr.ID == "x" {
			xErr = cr.Error
		}
	}
	if xErr == "" {
		t.Fatalf("expected error message on panicking checker")
	}
	if len(res.Findings) != 1 || res.Findings[0].ID != "y.ok" {
		t.Fatalf("subsequent checker findings lost: %+v", res.Findings)
	}
}

func TestScan_RespectsContextCancellation(t *testing.T) {
	withIsolatedRegistry(t)
	Register(&fakeChecker{id: "a", cat: "kernel", app: true})
	Register(&fakeChecker{id: "b", cat: "kernel", app: true})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res := Scan(ctx, ScanOptions{PerCheckTimeout: 10 * time.Millisecond})
	if len(res.Checks) != 0 {
		t.Fatalf("cancelled context should short-circuit before any check runs, got %d", len(res.Checks))
	}
}

func TestRegister_DuplicateIDPanics(t *testing.T) {
	withIsolatedRegistry(t)
	Register(&fakeChecker{id: "dup", cat: "k", app: true})
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected panic on duplicate id")
		}
	}()
	Register(&fakeChecker{id: "dup", cat: "k", app: true})
}
