package bridge

import (
	"context"
	"runtime"

	"github.com/WangYihang/Platypus/internal/agent/plugin"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

const journaldPluginID = "com.platypus.sys-journald-linux"

// LogPluginID returns the per-OS log-query plugin id. All three
// plugins (sys-journald-linux, sys-log-darwin, sys-log-windows)
// expose the same RPC name + JournalQuery* shape so callers can
// dispatch on runtime.GOOS without per-OS branching.
func LogPluginID(goos string) string {
	switch goos {
	case "darwin":
		return "com.platypus.sys-log-darwin"
	case "windows":
		return "com.platypus.sys-log-windows"
	default:
		return journaldPluginID
	}
}

// JournalQuery forwards to com.platypus.sys-journald-linux's query
// RPC (`journalctl -o json` with the operator's filter knobs).
// macOS / Windows callers should use LogQuery (which dispatches by
// runtime.GOOS to the right per-OS plugin via the same shape).
func JournalQuery(reg *plugin.Registry) func(ctx context.Context, req *v2pb.JournalQueryRequest) *v2pb.JournalQueryResponse {
	return func(ctx context.Context, req *v2pb.JournalQueryRequest) *v2pb.JournalQueryResponse {
		return queryLog(ctx, reg, journaldPluginID, req)
	}
}

// LogQuery is the cross-OS log search. Forwards to the per-OS
// sys-log plugin via runtime.GOOS dispatch. All three plugins
// (sys-journald-linux, sys-log-darwin, sys-log-windows) share the
// JournalQueryRequest / JournalQueryResponse shape so the typed
// bridge is identical everywhere.
func LogQuery(reg *plugin.Registry) func(ctx context.Context, req *v2pb.JournalQueryRequest) *v2pb.JournalQueryResponse {
	return func(ctx context.Context, req *v2pb.JournalQueryRequest) *v2pb.JournalQueryResponse {
		return queryLog(ctx, reg, LogPluginID(runtime.GOOS), req)
	}
}

func queryLog(ctx context.Context, reg *plugin.Registry, pluginID string, req *v2pb.JournalQueryRequest) *v2pb.JournalQueryResponse {
	var out v2pb.JournalQueryResponse
	errStr, err := invokeProto(ctx, reg, pluginID, "query", req, &out)
	if err != nil {
		return &v2pb.JournalQueryResponse{Error: err.Error()}
	}
	if errStr != "" {
		return &v2pb.JournalQueryResponse{Error: errStr}
	}
	return &out
}
