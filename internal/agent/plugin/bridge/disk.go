package bridge

import (
	"context"
	"runtime"

	"github.com/WangYihang/Platypus/internal/agent/plugin"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// DiskPluginID returns the per-OS sys-disk plugin id.
func DiskPluginID(goos string) string {
	switch goos {
	case "darwin":
		return "com.platypus.sys-disk-darwin"
	case "windows":
		return "com.platypus.sys-disk-windows"
	default:
		return "com.platypus.sys-disk-linux"
	}
}

// FilesystemList forwards to the per-OS sys-disk plugin's
// list_filesystems RPC.
func FilesystemList(reg *plugin.Registry) func(ctx context.Context, req *v2pb.FilesystemListRequest) *v2pb.FilesystemListResponse {
	return func(ctx context.Context, req *v2pb.FilesystemListRequest) *v2pb.FilesystemListResponse {
		var out v2pb.FilesystemListResponse
		errStr, err := invokeProto(ctx, reg, DiskPluginID(runtime.GOOS), "list_filesystems", req, &out)
		if err != nil {
			return &v2pb.FilesystemListResponse{Error: err.Error()}
		}
		if errStr != "" {
			return &v2pb.FilesystemListResponse{Error: errStr}
		}
		return &out
	}
}
