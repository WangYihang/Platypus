package bridge

import (
	"context"

	"github.com/WangYihang/Platypus/internal/agent/plugin"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// Stat is the plugin-backed replacement for agent.HandleStat.
// Lives in the same com.platypus.sys-listdir bundle as ListDir
// (since both want the fs.read capability with the same allowlist;
// sharing one .wasm halves the embedded-bundle bytes vs. shipping a
// dedicated sys-stat plugin).
//
// Wire shape: StatRequest{path} → StatResponse{entry|error}, same
// JSON↔proto bridge pattern as ListDir.
func Stat(reg *plugin.Registry) func(ctx context.Context, req *v2pb.StatRequest) *v2pb.StatResponse {
	return func(ctx context.Context, req *v2pb.StatRequest) *v2pb.StatResponse {
		var jsonResp statJSONResponse
		pluginErr, err := invokeJSON(ctx, reg, listDirPluginID, "stat",
			statJSONRequest{Path: req.GetPath()}, &jsonResp)
		if err != nil {
			return &v2pb.StatResponse{Error: "bridge: " + err.Error()}
		}
		if pluginErr != "" {
			return &v2pb.StatResponse{Error: pluginErr}
		}
		out := &v2pb.StatResponse{Error: jsonResp.Error}
		if jsonResp.Entry != nil {
			out.Entry = &v2pb.FileEntry{
				Name:          jsonResp.Entry.Name,
				Mode:          jsonResp.Entry.Mode,
				Size:          jsonResp.Entry.Size,
				MtimeUnixNano: jsonResp.Entry.MtimeUnixNano,
				SymlinkTarget: jsonResp.Entry.SymlinkTarget,
			}
		}
		return out
	}
}

type statJSONRequest struct {
	Path string `json:"path"`
}

type statJSONResponse struct {
	Entry *listDirJSONEntry `json:"entry,omitempty"`
	Error string            `json:"error,omitempty"`
}
