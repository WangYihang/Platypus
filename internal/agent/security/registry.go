// Package security holds the agent-side host-hardening scanner.
//
// The scanner is a registry of small, independent Checker
// implementations. Each Checker probes one aspect of the host (kernel
// version, a sysctl, the SSH server config, etc.) and emits zero or
// more Findings — a clean check produces an empty slice, not a
// "looks fine" finding, so the absence of findings is the
// default-good signal.
//
// Why a registry rather than a runtime plugin system: Platypus ships
// a single statically-linked agent binary, and host-security probes
// need raw filesystem / /proc / syscall access — exactly the
// affordances that any cross-platform plugin sandbox (Go plugin
// package, WASM, embedded scripting) either disallows or makes
// awkward. The registry pattern gets the extensibility benefit
// (adding a check = adding one .go file with an init()) without the
// distribution / signing / ABI-versioning cost of out-of-process
// plugins.
package security

import (
	"context"
	"sort"
	"sync"
	"time"
)

// Severity values for Finding.Severity. The agent never emits "info"
// on a clean check; reserve it for "this configuration is unusual
// but not broken" hits.
const (
	SeverityInfo     = "info"
	SeverityLow      = "low"
	SeverityMedium   = "medium"
	SeverityHigh     = "high"
	SeverityCritical = "critical"
)

// Status values for CheckResult.Status mirrored over the wire.
const (
	StatusOK      = "ok"
	StatusSkipped = "skipped"
	StatusError   = "error"
)

// Finding is one hit produced by one Checker. The fields mirror the
// SecurityFinding proto message; the package keeps its own struct so
// checkers don't pull a dep on the generated protobuf code (lets
// them be unit-tested without the proto round trip).
type Finding struct {
	ID          string
	Category    string
	Severity    string
	Title       string
	Description string
	Evidence    string
	Remediation string
	References  []string
}

// Checker is the extension point. Implementations should be
// stateless — Run is called from a fresh goroutine per scan, and the
// registry holds the same instance across scans, so any mutable state
// must be guarded by the implementation.
//
// Run's returned error means "this check could not run on this host"
// (e.g. /proc/version unreadable, sshd_config missing). It does NOT
// mean "the host failed the check" — that's a Finding. Returning
// (nil, nil) is the clean / passing case.
//
// Applicable is consulted before Run. A checker that returns false
// reports a "skipped" CheckResult and does not contribute findings.
// Use it for OS-specific checks (linux-only sysctl, ssh-only when
// /etc/ssh/sshd_config is absent, etc.) so the wire shape stays
// honest about what was attempted.
type Checker interface {
	ID() string
	Category() string
	Applicable(ctx context.Context) bool
	Run(ctx context.Context) ([]Finding, error)
}

// CheckResult is the per-checker lifecycle record (mirrors the
// CheckResult proto). The scanner emits one of these for every
// checker it considered, even skipped ones, so the UI can
// distinguish "check ran clean" from "check wasn't applicable".
type CheckResult struct {
	ID           string
	Category     string
	Status       string
	Error        string
	Elapsed      time.Duration
	FindingCount int
}

// ScanResult is the full output of one Scan invocation.
type ScanResult struct {
	Findings  []Finding
	Checks    []CheckResult
	StartedAt time.Time
	Elapsed   time.Duration
}

// ScanOptions mirror the SecurityScanRequest proto, decoupled so the
// scanner core has no protobuf dependency. Empty CheckIDs +
// Categories means "run every applicable check".
type ScanOptions struct {
	CheckIDs        []string
	Categories      []string
	PerCheckTimeout time.Duration
}

var (
	registryMu sync.RWMutex
	registry   = map[string]Checker{}
)

// Register installs c into the global registry. Intended to be
// called from a checker file's init(). Panics on duplicate id —
// duplicate ids would silently shadow each other and produce
// confusing scan output, so we'd rather fail loudly at startup.
func Register(c Checker) {
	if c == nil {
		panic("security: Register(nil)")
	}
	id := c.ID()
	if id == "" {
		panic("security: Register: checker has empty id")
	}
	registryMu.Lock()
	defer registryMu.Unlock()
	if _, dup := registry[id]; dup {
		panic("security: Register: duplicate checker id " + id)
	}
	registry[id] = c
}

// Checkers returns a snapshot of the registered checkers, sorted by
// id for deterministic scan ordering.
func Checkers() []Checker {
	registryMu.RLock()
	defer registryMu.RUnlock()
	out := make([]Checker, 0, len(registry))
	for _, c := range registry {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID() < out[j].ID() })
	return out
}

// Scan runs the registered checkers (filtered by opts) sequentially
// and returns a flat result. Sequential is deliberate: most checks
// hit the same handful of files (/proc, /etc), running them in
// parallel adds contention without meaningful wall-clock savings on
// a host that only has a few dozen checks. Switch to a worker pool
// here if the registry grows past O(100) checks.
func Scan(ctx context.Context, opts ScanOptions) ScanResult {
	started := time.Now()
	checkers := selectCheckers(opts)

	res := ScanResult{
		StartedAt: started,
		Checks:    make([]CheckResult, 0, len(checkers)),
	}

	for _, c := range checkers {
		if ctx.Err() != nil {
			break
		}
		res.Checks = append(res.Checks, runOne(ctx, c, opts.PerCheckTimeout, &res.Findings))
	}

	res.Elapsed = time.Since(started)
	return res
}

// runOne executes a single Checker under an optional per-check
// timeout, captures panics so one rogue check can't kill the whole
// scan, and appends any findings into out.
func runOne(ctx context.Context, c Checker, perCheckTimeout time.Duration, out *[]Finding) (cr CheckResult) {
	cr = CheckResult{ID: c.ID(), Category: c.Category()}
	start := time.Now()

	cctx := ctx
	var cancel context.CancelFunc
	if perCheckTimeout > 0 {
		cctx, cancel = context.WithTimeout(ctx, perCheckTimeout)
		defer cancel()
	}

	if !c.Applicable(cctx) {
		cr.Status = StatusSkipped
		cr.Elapsed = time.Since(start)
		return cr
	}

	// Named return so the recover-block can mutate cr after the
	// panicking Run aborts the rest of this function.
	defer func() {
		if r := recover(); r != nil {
			cr.Status = StatusError
			cr.Error = panicMessage(r)
			cr.Elapsed = time.Since(start)
		}
	}()

	findings, err := c.Run(cctx)
	cr.Elapsed = time.Since(start)
	if err != nil {
		cr.Status = StatusError
		cr.Error = err.Error()
		return cr
	}
	cr.Status = StatusOK
	cr.FindingCount = len(findings)
	*out = append(*out, findings...)
	return cr
}

// selectCheckers applies the CheckIDs / Categories filter. Empty
// filter on both fields means "everything"; otherwise it's a union —
// a checker matches if its id appears in CheckIDs OR its category
// appears in Categories. Unknown ids / categories are silently
// dropped (the scan stays best-effort).
func selectCheckers(opts ScanOptions) []Checker {
	all := Checkers()
	if len(opts.CheckIDs) == 0 && len(opts.Categories) == 0 {
		return all
	}
	idSet := make(map[string]struct{}, len(opts.CheckIDs))
	for _, id := range opts.CheckIDs {
		idSet[id] = struct{}{}
	}
	catSet := make(map[string]struct{}, len(opts.Categories))
	for _, cat := range opts.Categories {
		catSet[cat] = struct{}{}
	}
	out := all[:0:0]
	for _, c := range all {
		if _, ok := idSet[c.ID()]; ok {
			out = append(out, c)
			continue
		}
		if _, ok := catSet[c.Category()]; ok {
			out = append(out, c)
		}
	}
	return out
}

func panicMessage(r any) string {
	switch v := r.(type) {
	case string:
		return "panic: " + v
	case error:
		return "panic: " + v.Error()
	default:
		return "panic in checker"
	}
}
