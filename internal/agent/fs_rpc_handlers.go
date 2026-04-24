package agent

import (
	"context"
	"os"
	"path/filepath"

	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// Production implementations for the filesystem-shaped RPCs in
// AgentRPCHandlers. Each is intentionally tiny — a syscall wrapper
// that packages the result into the matching response proto. Agent
// main.go wires them into AgentRPCHandlers so ServeRPCStream can
// dispatch.

// HandleListDir returns the entries of req.Path sorted by filename
// (the order ReadDir returns). No recursion — callers issue
// successive ListDir calls to descend.
func HandleListDir(_ context.Context, req *v2pb.ListDirRequest) *v2pb.ListDirResponse {
	if req == nil || req.Path == "" {
		return &v2pb.ListDirResponse{Error: "empty path"}
	}
	entries, err := os.ReadDir(req.Path)
	if err != nil {
		return &v2pb.ListDirResponse{Error: err.Error()}
	}
	resp := &v2pb.ListDirResponse{}
	for _, e := range entries {
		info, ierr := e.Info()
		if ierr != nil {
			// Entry disappeared between ReadDir and Info — skip it.
			continue
		}
		resp.Entries = append(resp.Entries, fileEntryFromInfo(e.Name(), info, filepath.Join(req.Path, e.Name())))
	}
	return resp
}

// HandleStat returns metadata for a single path. Symlinks: we
// follow them (os.Stat) so the Mode / Size reflect the target.
func HandleStat(_ context.Context, req *v2pb.StatRequest) *v2pb.StatResponse {
	if req == nil || req.Path == "" {
		return &v2pb.StatResponse{Error: "empty path"}
	}
	info, err := os.Lstat(req.Path)
	if err != nil {
		return &v2pb.StatResponse{Error: err.Error()}
	}
	return &v2pb.StatResponse{Entry: fileEntryFromInfo(info.Name(), info, req.Path)}
}

// HandleDelete removes a path. Recursive=true does a full tree
// delete (os.RemoveAll); false uses os.Remove which fails on
// non-empty dirs.
func HandleDelete(_ context.Context, req *v2pb.DeleteRequest) *v2pb.DeleteResponse {
	if req == nil || req.Path == "" {
		return &v2pb.DeleteResponse{Error: "empty path"}
	}
	var err error
	if req.Recursive {
		err = os.RemoveAll(req.Path)
	} else {
		err = os.Remove(req.Path)
	}
	if err != nil {
		return &v2pb.DeleteResponse{Error: err.Error()}
	}
	return &v2pb.DeleteResponse{}
}

// HandleRename moves a path. Fails if the destination exists when
// the underlying filesystem disallows overwriting.
func HandleRename(_ context.Context, req *v2pb.RenameRequest) *v2pb.RenameResponse {
	if req == nil || req.From == "" || req.To == "" {
		return &v2pb.RenameResponse{Error: "empty from/to"}
	}
	if err := os.Rename(req.From, req.To); err != nil {
		return &v2pb.RenameResponse{Error: err.Error()}
	}
	return &v2pb.RenameResponse{}
}

// HandleMkdir creates a directory. When req.Mkdirs, parent
// directories are created as needed (os.MkdirAll); otherwise only
// the leaf (os.Mkdir). Mode defaults to 0o755 if zero.
func HandleMkdir(_ context.Context, req *v2pb.MkdirRequest) *v2pb.MkdirResponse {
	if req == nil || req.Path == "" {
		return &v2pb.MkdirResponse{Error: "empty path"}
	}
	mode := os.FileMode(req.Mode) & os.ModePerm
	if mode == 0 {
		mode = 0o755
	}
	var err error
	if req.Mkdirs {
		err = os.MkdirAll(req.Path, mode)
	} else {
		err = os.Mkdir(req.Path, mode)
	}
	if err != nil {
		return &v2pb.MkdirResponse{Error: err.Error()}
	}
	return &v2pb.MkdirResponse{}
}

// HandleChmod changes a path's mode bits.
func HandleChmod(_ context.Context, req *v2pb.ChmodRequest) *v2pb.ChmodResponse {
	if req == nil || req.Path == "" {
		return &v2pb.ChmodResponse{Error: "empty path"}
	}
	if err := os.Chmod(req.Path, os.FileMode(req.Mode)&os.ModePerm); err != nil {
		return &v2pb.ChmodResponse{Error: err.Error()}
	}
	return &v2pb.ChmodResponse{}
}

// HandleSysInfo returns a rich snapshot of the agent host. The
// heavy lifting lives in CollectSysInfo; this wrapper exists so the
// RPC dispatcher's handler signature matches and so tests can stub
// the collector. Agent admins call this on-demand from the Web UI
// to refresh the Info panel. Live metrics (CPU %, memory) are
// sampled inline; static fields (kernel, platform, CPU model) are
// cheap to re-fetch so we always return a fresh snapshot.
func HandleSysInfo(ctx context.Context, _ *v2pb.SysInfoRequest) *v2pb.SysInfoResponse {
	return CollectSysInfo(ctx)
}

// HandleProcessList returns a sorted, capped process snapshot. The
// server proxies this for the dedicated Processes tab in the Web UI.
// Unlike SysInfo, this response can be large on busy hosts so we
// bound it via processListCap (500). top_n=0 means "as many as the
// cap allows"; sort_by defaults to "cpu" when empty or unrecognized.
func HandleProcessList(ctx context.Context, req *v2pb.ProcessListRequest) *v2pb.ProcessListResponse {
	top := uint32(0)
	sortBy := ""
	if req != nil {
		top = req.TopN
		sortBy = req.SortBy
	}
	return CollectProcessList(ctx, top, sortBy)
}

// fileEntryFromInfo packages an os.FileInfo into the proto shape.
// Populates SymlinkTarget only when the entry itself is a symlink
// (the caller passed the full path so we can readlink it).
func fileEntryFromInfo(name string, info os.FileInfo, fullPath string) *v2pb.FileEntry {
	entry := &v2pb.FileEntry{
		Name:          name,
		Mode:          uint32(info.Mode()),
		Size:          info.Size(),
		MtimeUnixNano: info.ModTime().UnixNano(),
	}
	if info.Mode()&os.ModeSymlink != 0 {
		if target, err := os.Readlink(fullPath); err == nil {
			entry.SymlinkTarget = target
		}
	}
	return entry
}
