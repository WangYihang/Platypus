package agent

import (
	"context"
	"time"

	cfgaudit "github.com/WangYihang/Platypus/internal/agent/config_audit"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// HandleConfigAudit adapts the config_audit package's pure-Go scanner
// into the ConfigAudit RPC. Like the security handler it keeps the
// wire types confined to the agent package so the audit core stays
// protobuf-free and independently testable.
//
// The redaction invariant lives one layer down (auditors must already
// have masked any plaintext secrets in Leak.MatchRedacted before
// returning); this handler simply forwards the field across the wire.
func HandleConfigAudit(ctx context.Context, req *v2pb.ConfigAuditRequest) *v2pb.ConfigAuditResponse {
	opts := cfgaudit.AuditOptions{}
	if req != nil {
		opts.AuditorIDs = append(opts.AuditorIDs, req.GetAuditorIds()...)
		opts.Categories = append(opts.Categories, req.GetCategories()...)
		if t := req.GetPerAuditorTimeoutMs(); t > 0 {
			opts.PerAuditorTimeout = time.Duration(t) * time.Millisecond
		}
	}

	res := cfgaudit.Audit(ctx, opts)

	resp := &v2pb.ConfigAuditResponse{
		StartedAtUnix: res.StartedAt.Unix(),
		ElapsedMs:     uint64(res.Elapsed / time.Millisecond),
	}
	for _, l := range res.Leaks {
		resp.Leaks = append(resp.Leaks, &v2pb.ConfigLeak{
			Id:          l.ID,
			AuditorId:   deriveAuditorID(l.ID),
			Category:    l.Category,
			Risk:        l.Risk,
			Title:       l.Title,
			Location:    l.Location,
			Match:       l.MatchRedacted,
			Pattern:     l.Pattern,
			Description: l.Description,
			Remediation: l.Remediation,
			References:  append([]string(nil), l.References...),
		})
	}
	for _, a := range res.Auditors {
		resp.Auditors = append(resp.Auditors, &v2pb.AuditorResult{
			Id:        a.ID,
			Category:  a.Category,
			Status:    a.Status,
			Error:     a.Error,
			ElapsedMs: uint64(a.Elapsed / time.Millisecond),
			LeakCount: uint32(a.LeakCount),
		})
	}
	return resp
}

// deriveAuditorID strips a leak's trailing component and returns the
// owning auditor id. Leak ids follow "<auditor_id>.<sub>..." (e.g.
// "shell.history.behavior.curl-basic-auth"), and we keep the auditor
// portion as everything up to and including the second dot when
// present, mirroring how registered auditor ids look ("shell.history",
// "db.config", "webapp.config", "ssh.keys").
func deriveAuditorID(leakID string) string {
	dots := 0
	for i := 0; i < len(leakID); i++ {
		if leakID[i] == '.' {
			dots++
			if dots == 2 {
				return leakID[:i]
			}
		}
	}
	// Fallback: single-dot id ("env.process") or no dot at all.
	for i := len(leakID) - 1; i >= 0; i-- {
		if leakID[i] == '.' {
			return leakID[:i]
		}
	}
	return leakID
}

// HandleListConfigAuditors enumerates every registered Auditor so the
// UI's checklist / Coverage panel can render before any audit runs.
// Applicable() is evaluated against the live host so the UI can dim
// rows for auditors whose preconditions aren't met (e.g. a Linux-only
// auditor on a Darwin agent — there are none today, but the wire is
// honest about it).
func HandleListConfigAuditors(ctx context.Context, _ *v2pb.ListConfigAuditorsRequest) *v2pb.ListConfigAuditorsResponse {
	auditors := cfgaudit.Auditors()
	resp := &v2pb.ListConfigAuditorsResponse{
		Auditors: make([]*v2pb.AvailableAuditor, 0, len(auditors)),
	}
	for _, a := range auditors {
		meta := a.Metadata()
		resp.Auditors = append(resp.Auditors, &v2pb.AvailableAuditor{
			Id:          a.ID(),
			Category:    a.Category(),
			Applicable:  a.Applicable(ctx),
			Title:       meta.Title,
			Description: meta.Description,
			References:  append([]string(nil), meta.References...),
		})
	}
	return resp
}
