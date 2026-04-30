package agent

import (
	"context"
	"time"

	"github.com/WangYihang/Platypus/internal/agent/security"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// HandleSecurityScan adapts the security package's pure-Go scanner
// into the SecurityScan RPC. The wire types are kept agent-package-
// local so the security package itself stays free of protobuf deps
// (which keeps it independently unit-testable and lets us swap the
// transport layer without touching the checks).
func HandleSecurityScan(ctx context.Context, req *v2pb.SecurityScanRequest) *v2pb.SecurityScanResponse {
	opts := security.ScanOptions{}
	if req != nil {
		opts.CheckIDs = append(opts.CheckIDs, req.GetCheckIds()...)
		opts.Categories = append(opts.Categories, req.GetCategories()...)
		if t := req.GetPerCheckTimeoutMs(); t > 0 {
			opts.PerCheckTimeout = time.Duration(t) * time.Millisecond
		}
	}

	res := security.Scan(ctx, opts)

	resp := &v2pb.SecurityScanResponse{
		StartedAtUnix: res.StartedAt.Unix(),
		ElapsedMs:     uint64(res.Elapsed / time.Millisecond),
	}
	for _, f := range res.Findings {
		resp.Findings = append(resp.Findings, &v2pb.SecurityFinding{
			Id:          f.ID,
			CheckId:     deriveCheckID(f.ID),
			Category:    f.Category,
			Severity:    f.Severity,
			Title:       f.Title,
			Description: f.Description,
			Evidence:    f.Evidence,
			Remediation: f.Remediation,
			References:  append([]string(nil), f.References...),
		})
	}
	for _, c := range res.Checks {
		resp.Checks = append(resp.Checks, &v2pb.CheckResult{
			Id:           c.ID,
			Category:     c.Category,
			Status:       c.Status,
			Error:        c.Error,
			ElapsedMs:    uint64(c.Elapsed / time.Millisecond),
			FindingCount: uint32(c.FindingCount),
		})
	}
	return resp
}

// deriveCheckID extracts the owning check id from a finding id by
// dropping the trailing component after the last dot. Findings ids
// follow "<check_id>.<finding_subkey>" by convention (e.g.
// "ssh.permitrootlogin" → "ssh"), and falling back to the full id
// when there's no dot keeps the field meaningful for single-finding
// checks like "kernel.version.eol".
func deriveCheckID(findingID string) string {
	for i := len(findingID) - 1; i >= 0; i-- {
		if findingID[i] == '.' {
			return findingID[:i]
		}
	}
	return findingID
}

// HandleListSecurityChecks enumerates every registered Checker so the
// UI can render its checklist (and the Coverage panel) before any
// scan completes. Applicable is evaluated against the live host so
// the UI can render not-applicable rows dimmed (e.g. ssh.config
// skipped when /etc/ssh/sshd_config is missing).
func HandleListSecurityChecks(ctx context.Context, _ *v2pb.ListSecurityChecksRequest) *v2pb.ListSecurityChecksResponse {
	checkers := security.Checkers()
	resp := &v2pb.ListSecurityChecksResponse{
		Checks: make([]*v2pb.AvailableSecurityCheck, 0, len(checkers)),
	}
	for _, c := range checkers {
		meta := c.Metadata()
		resp.Checks = append(resp.Checks, &v2pb.AvailableSecurityCheck{
			Id:          c.ID(),
			Category:    c.Category(),
			Applicable:  c.Applicable(ctx),
			Title:       meta.Title,
			Description: meta.Description,
			References:  append([]string(nil), meta.References...),
		})
	}
	return resp
}
