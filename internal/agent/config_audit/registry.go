// Package config_audit holds the agent-side sensitive-information /
// configuration audit scanner.
//
// It is the sibling of internal/agent/security: where security checks
// host hardening posture (kernel, sysctl, sshd_config), config_audit
// hunts for credentials and tokens leaking through configuration —
// shell history, env vars, well-known dotfiles, web app config files.
//
// The two packages are deliberately kept separate. They share a
// registry shape, but their result types, severities (security calls
// it "severity", we call it "risk"), and persistence are independent
// so each can evolve without the other paying a coupling tax.
//
// Detection of "is this string a secret?" is delegated to gitleaks
// (see detector.go). This package owns "where to look": each Auditor
// enumerates a specific source (a directory of dotfiles, the env of
// every process, the shell history of every user) and feeds bytes to
// the shared detector. Anything secret-looking is converted to a
// Leak with the underlying secret already redacted before it can
// leave the agent process.
package config_audit

import (
	"context"
	"sort"
	"sync"
	"time"
)

// Risk values for Leak.Risk. We deliberately use "risk" rather than
// "severity" to keep the UI vocabulary distinct from the security
// baseline tab — leaked secrets are an exposure question, not a
// CVE-style criticality question.
const (
	RiskInfo   = "info"
	RiskLow    = "low"
	RiskMedium = "medium"
	RiskHigh   = "high"
)

// Status values for AuditorResult.Status; mirror the security package
// so the UI can reuse its lifecycle rendering verbatim.
const (
	StatusOK      = "ok"
	StatusSkipped = "skipped"
	StatusError   = "error"
)

// Leak is one hit produced by one Auditor. MatchRedacted is REQUIRED
// to be the masked form of any underlying credential — auditors must
// never put plaintext secret material into this field. The RPC layer
// trusts this invariant; it does not re-check.
type Leak struct {
	ID            string
	Category      string
	Risk          string
	Title         string
	Location      string // "/root/.bash_history:142", "env:PID 1234"
	MatchRedacted string // e.g. "AKIA****WXYZ"
	Pattern       string // gitleaks RuleID, or "behavior:<name>" for our own rules
	Description   string
	Remediation   string
	References    []string
}

// AuditMetadata mirrors security.CheckMetadata: a stable description
// the UI's Coverage panel renders before any audit has run.
type AuditMetadata struct {
	Title       string
	Description string
	References  []string
}

// Auditor is the extension point. Implementations should be stateless
// (the registry holds the same instance across audits). Run returning
// a non-nil error means "the auditor itself could not run on this
// host" — a finding is a Leak, not an error.
type Auditor interface {
	ID() string
	Category() string
	Applicable(ctx context.Context) bool
	Run(ctx context.Context) ([]Leak, error)
	Metadata() AuditMetadata
}

// AuditorResult is the per-auditor lifecycle record (mirrors the
// AuditorResult proto). One is emitted for every Auditor the audit
// considered, including skipped ones, so the UI can distinguish "ran
// clean" from "wasn't applicable".
type AuditorResult struct {
	ID        string
	Category  string
	Status    string
	Error     string
	Elapsed   time.Duration
	LeakCount int
}

// AuditResult is the full output of one Audit invocation.
type AuditResult struct {
	Leaks     []Leak
	Auditors  []AuditorResult
	StartedAt time.Time
	Elapsed   time.Duration
}

// AuditOptions mirror the proto request, decoupled so the core has no
// protobuf dependency. Empty AuditorIDs + Categories means "run every
// applicable auditor".
type AuditOptions struct {
	AuditorIDs        []string
	Categories        []string
	PerAuditorTimeout time.Duration
}

var (
	registryMu sync.RWMutex
	registry   = map[string]Auditor{}
)

// Register installs a into the global registry. Intended to be called
// from each auditor file's init(). Panics on duplicate id (silent
// shadowing would cause confusing scan output).
func Register(a Auditor) {
	if a == nil {
		panic("config_audit: Register(nil)")
	}
	id := a.ID()
	if id == "" {
		panic("config_audit: Register: auditor has empty id")
	}
	registryMu.Lock()
	defer registryMu.Unlock()
	if _, dup := registry[id]; dup {
		panic("config_audit: Register: duplicate auditor id " + id)
	}
	registry[id] = a
}

// Auditors returns a snapshot sorted by id for deterministic ordering.
func Auditors() []Auditor {
	registryMu.RLock()
	defer registryMu.RUnlock()
	out := make([]Auditor, 0, len(registry))
	for _, a := range registry {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID() < out[j].ID() })
	return out
}

// Audit runs the registered auditors filtered by opts and returns a
// flat result. Sequential by design: most auditors are I/O-bound on
// the same handful of paths and parallelism would only add contention
// without measurable wall-clock savings.
func Audit(ctx context.Context, opts AuditOptions) AuditResult {
	started := time.Now()
	auditors := selectAuditors(opts)

	res := AuditResult{
		StartedAt: started,
		Auditors:  make([]AuditorResult, 0, len(auditors)),
	}

	for _, a := range auditors {
		if ctx.Err() != nil {
			break
		}
		res.Auditors = append(res.Auditors, runOne(ctx, a, opts.PerAuditorTimeout, &res.Leaks))
	}

	res.Elapsed = time.Since(started)
	return res
}

func runOne(ctx context.Context, a Auditor, perAuditorTimeout time.Duration, out *[]Leak) (ar AuditorResult) {
	ar = AuditorResult{ID: a.ID(), Category: a.Category()}
	start := time.Now()

	cctx := ctx
	var cancel context.CancelFunc
	if perAuditorTimeout > 0 {
		cctx, cancel = context.WithTimeout(ctx, perAuditorTimeout)
		defer cancel()
	}

	if !a.Applicable(cctx) {
		ar.Status = StatusSkipped
		ar.Elapsed = time.Since(start)
		return ar
	}

	defer func() {
		if r := recover(); r != nil {
			ar.Status = StatusError
			ar.Error = panicMessage(r)
			ar.Elapsed = time.Since(start)
		}
	}()

	leaks, err := a.Run(cctx)
	ar.Elapsed = time.Since(start)
	if err != nil {
		ar.Status = StatusError
		ar.Error = err.Error()
		return ar
	}
	ar.Status = StatusOK
	ar.LeakCount = len(leaks)
	*out = append(*out, leaks...)
	return ar
}

func selectAuditors(opts AuditOptions) []Auditor {
	all := Auditors()
	if len(opts.AuditorIDs) == 0 && len(opts.Categories) == 0 {
		return all
	}
	idSet := make(map[string]struct{}, len(opts.AuditorIDs))
	for _, id := range opts.AuditorIDs {
		idSet[id] = struct{}{}
	}
	catSet := make(map[string]struct{}, len(opts.Categories))
	for _, cat := range opts.Categories {
		catSet[cat] = struct{}{}
	}
	out := all[:0:0]
	for _, a := range all {
		if _, ok := idSet[a.ID()]; ok {
			out = append(out, a)
			continue
		}
		if _, ok := catSet[a.Category()]; ok {
			out = append(out, a)
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
		return "panic in auditor"
	}
}
