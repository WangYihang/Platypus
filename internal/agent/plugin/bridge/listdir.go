package bridge

import (
	"context"

	"github.com/WangYihang/Platypus/internal/agent/plugin"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// listDirPluginID is the merged sys-files-read plugin: it owns
// list_dir + stat (this file) plus the file_read / file_scan /
// file_archive streams. One id, one wasm, one operator-grant
// boundary for the whole fs.read capability family.
const listDirPluginID = "com.platypus.sys-files-read"

// ListDir is the plugin-backed replacement for the agent's
// agent.HandleListDir built-in. Drop-in compatible: same proto
// signature, same dispatch slot in AgentRPCHandlers.ListDir; the body
// just delegates to the sys-files-read plugin instead of doing the
// directory walk in Go.
//
// On any plugin-level failure (capability denied, plugin not
// installed, malformed wasm, etc.) the response carries the plugin
// error in ListDirResponse.error, matching the legacy handler's
// failure-encoding contract — clients distinguish read errors from
// transport errors via the per-payload `error` field.
func ListDir(reg *plugin.Registry) func(ctx context.Context, req *v2pb.ListDirRequest) *v2pb.ListDirResponse {
	return func(ctx context.Context, req *v2pb.ListDirRequest) *v2pb.ListDirResponse {
		var jsonResp listDirJSONResponse
		pluginErr, err := invokeJSON(ctx, reg, listDirPluginID, "list_dir",
			listDirJSONRequest{Path: req.GetPath()}, &jsonResp)
		if err != nil {
			return &v2pb.ListDirResponse{Error: "bridge: " + err.Error()}
		}
		if pluginErr != "" {
			return &v2pb.ListDirResponse{Error: pluginErr}
		}
		entries := make([]*v2pb.FileEntry, 0, len(jsonResp.Entries))
		for _, e := range jsonResp.Entries {
			entries = append(entries, &v2pb.FileEntry{
				Name:          e.Name,
				Mode:          e.Mode,
				Size:          e.Size,
				MtimeUnixNano: e.MtimeUnixNano,
				SymlinkTarget: e.SymlinkTarget,
			})
		}
		return &v2pb.ListDirResponse{Entries: entries, Error: jsonResp.Error}
	}
}

// listDirJSONRequest / Response mirror the schema sys-listdir's
// Rust src/lib.rs serializes. Field tags match what serde produces;
// adding/removing a field requires touching both sides.
type listDirJSONRequest struct {
	Path string `json:"path"`
}

type listDirJSONResponse struct {
	Entries []listDirJSONEntry `json:"entries"`
	Error   string             `json:"error,omitempty"`
}

type listDirJSONEntry struct {
	Name          string `json:"name"`
	Mode          uint32 `json:"mode"`
	Size          int64  `json:"size"`
	MtimeUnixNano int64  `json:"mtime_unix_nano"`
	SymlinkTarget string `json:"symlink_target,omitempty"`
}
