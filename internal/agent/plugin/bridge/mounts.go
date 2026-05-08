package bridge

import (
	"context"
	"runtime"

	"github.com/WangYihang/Platypus/internal/agent/plugin"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// MountsPluginID returns the per-OS sys-mounts plugin id.
func MountsPluginID(goos string) string {
	switch goos {
	case "darwin":
		return "com.platypus.sys-mounts-darwin"
	case "windows":
		return "com.platypus.sys-mounts-windows"
	default:
		return "com.platypus.sys-mounts-linux"
	}
}

// MountList forwards to the per-OS sys-mounts plugin's list_mounts
// RPC. Companion to FilesystemList: that RPC reports usage (sizes),
// this one reports topology (mount options, fstab cross-reference).
func MountList(reg *plugin.Registry) func(ctx context.Context, req *v2pb.MountListRequest) *v2pb.MountListResponse {
	return func(ctx context.Context, req *v2pb.MountListRequest) *v2pb.MountListResponse {
		var out v2pb.MountListResponse
		errStr, err := invokeProto(ctx, reg, MountsPluginID(runtime.GOOS), "list_mounts", req, &out)
		if err != nil {
			return &v2pb.MountListResponse{Error: err.Error()}
		}
		if errStr != "" {
			return &v2pb.MountListResponse{Error: errStr}
		}
		return &out
	}
}
