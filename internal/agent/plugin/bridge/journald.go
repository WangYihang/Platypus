package bridge

import (
	"context"

	"github.com/WangYihang/Platypus/internal/agent/plugin"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

const journaldPluginID = "com.platypus.sys-journald-linux"

// JournalQuery forwards to com.platypus.sys-journald-linux's query
// RPC (`journalctl -o json` with the operator's filter knobs).
//
// Future macOS / Windows siblings (sys-log-darwin via `log show`,
// sys-log-windows via Get-WinEvent) are intended to share the same
// JournalQueryRequest / JournalQueryResponse shape so a single
// "search logs across the fleet" UI works everywhere.
func JournalQuery(reg *plugin.Registry) func(ctx context.Context, req *v2pb.JournalQueryRequest) *v2pb.JournalQueryResponse {
	return func(ctx context.Context, req *v2pb.JournalQueryRequest) *v2pb.JournalQueryResponse {
		var out v2pb.JournalQueryResponse
		errStr, err := invokeProto(ctx, reg, journaldPluginID, "query", req, &out)
		if err != nil {
			return &v2pb.JournalQueryResponse{Error: err.Error()}
		}
		if errStr != "" {
			return &v2pb.JournalQueryResponse{Error: errStr}
		}
		return &out
	}
}
