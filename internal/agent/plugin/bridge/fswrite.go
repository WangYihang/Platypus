package bridge

import (
	"context"

	"github.com/WangYihang/Platypus/internal/agent/plugin"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// fsWritePluginIDs is the preference chain. Merged sys-files-write
// (post-merge, also owns the file_write stream) wins where the
// publisher staged it; sys-fs-write is the legacy fallback for
// production agents still on the read-only embed FS. Mkdir / Chmod
// / Delete / Rename have byte-identical wire shapes in both ids.
var fsWritePluginIDs = []string{
	"com.platypus.sys-files-write",
	"com.platypus.sys-fs-write",
}

// errOnlyJSON is the response shape every fs.write handler returns:
// just an error string (empty on success). Mirrors the legacy
// handlers' wire response, all of which were `<X>Response { error }`.
type errOnlyJSON struct {
	Error string `json:"error,omitempty"`
}

// Mkdir is the plugin-backed replacement for agent.HandleMkdir.
func Mkdir(reg *plugin.Registry) func(ctx context.Context, req *v2pb.MkdirRequest) *v2pb.MkdirResponse {
	return func(ctx context.Context, req *v2pb.MkdirRequest) *v2pb.MkdirResponse {
		var resp errOnlyJSON
		pluginErr, err := invokeJSONFallback(ctx, reg, fsWritePluginIDs, "mkdir", mkdirJSON{
			Path: req.GetPath(), Mode: req.GetMode(), Mkdirs: req.GetMkdirs(),
		}, &resp)
		if err != nil {
			return &v2pb.MkdirResponse{Error: "bridge: " + err.Error()}
		}
		if pluginErr != "" {
			return &v2pb.MkdirResponse{Error: pluginErr}
		}
		return &v2pb.MkdirResponse{Error: resp.Error}
	}
}

// Chmod is the plugin-backed replacement for agent.HandleChmod.
func Chmod(reg *plugin.Registry) func(ctx context.Context, req *v2pb.ChmodRequest) *v2pb.ChmodResponse {
	return func(ctx context.Context, req *v2pb.ChmodRequest) *v2pb.ChmodResponse {
		var resp errOnlyJSON
		pluginErr, err := invokeJSONFallback(ctx, reg, fsWritePluginIDs, "chmod", chmodJSON{
			Path: req.GetPath(), Mode: req.GetMode(),
		}, &resp)
		if err != nil {
			return &v2pb.ChmodResponse{Error: "bridge: " + err.Error()}
		}
		if pluginErr != "" {
			return &v2pb.ChmodResponse{Error: pluginErr}
		}
		return &v2pb.ChmodResponse{Error: resp.Error}
	}
}

// Delete is the plugin-backed replacement for agent.HandleDelete.
func Delete(reg *plugin.Registry) func(ctx context.Context, req *v2pb.DeleteRequest) *v2pb.DeleteResponse {
	return func(ctx context.Context, req *v2pb.DeleteRequest) *v2pb.DeleteResponse {
		var resp errOnlyJSON
		pluginErr, err := invokeJSONFallback(ctx, reg, fsWritePluginIDs, "delete", deleteJSON{
			Path: req.GetPath(), Recursive: req.GetRecursive(),
		}, &resp)
		if err != nil {
			return &v2pb.DeleteResponse{Error: "bridge: " + err.Error()}
		}
		if pluginErr != "" {
			return &v2pb.DeleteResponse{Error: pluginErr}
		}
		return &v2pb.DeleteResponse{Error: resp.Error}
	}
}

// Rename is the plugin-backed replacement for agent.HandleRename.
func Rename(reg *plugin.Registry) func(ctx context.Context, req *v2pb.RenameRequest) *v2pb.RenameResponse {
	return func(ctx context.Context, req *v2pb.RenameRequest) *v2pb.RenameResponse {
		var resp errOnlyJSON
		pluginErr, err := invokeJSONFallback(ctx, reg, fsWritePluginIDs, "rename", renameJSON{
			From: req.GetFrom(), To: req.GetTo(),
		}, &resp)
		if err != nil {
			return &v2pb.RenameResponse{Error: "bridge: " + err.Error()}
		}
		if pluginErr != "" {
			return &v2pb.RenameResponse{Error: pluginErr}
		}
		return &v2pb.RenameResponse{Error: resp.Error}
	}
}

// JSON request shapes the Rust side decodes via serde. Names + tags
// match example/plugins/sys-fs-write/src/lib.rs.
type mkdirJSON struct {
	Path   string `json:"path"`
	Mode   uint32 `json:"mode,omitempty"`
	Mkdirs bool   `json:"mkdirs,omitempty"`
}

type chmodJSON struct {
	Path string `json:"path"`
	Mode uint32 `json:"mode"`
}

type deleteJSON struct {
	Path      string `json:"path"`
	Recursive bool   `json:"recursive,omitempty"`
}

type renameJSON struct {
	From string `json:"from"`
	To   string `json:"to"`
}
