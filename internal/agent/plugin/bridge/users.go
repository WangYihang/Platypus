package bridge

import (
	"context"
	"runtime"

	"github.com/WangYihang/Platypus/internal/agent/plugin"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// UsersPluginID returns the per-OS sys-users plugin id.
func UsersPluginID(goos string) string {
	switch goos {
	case "darwin":
		return "com.platypus.sys-users-darwin"
	case "windows":
		return "com.platypus.sys-users-windows"
	default:
		return "com.platypus.sys-users-linux"
	}
}

// UserList forwards to the per-OS sys-users plugin's list_users RPC.
// Returns local accounts + groups + sudo / Administrators escalation
// rows so operators can audit "who has access" in one round trip.
func UserList(reg *plugin.Registry) func(ctx context.Context, req *v2pb.UserListRequest) *v2pb.UserListResponse {
	return func(ctx context.Context, req *v2pb.UserListRequest) *v2pb.UserListResponse {
		var out v2pb.UserListResponse
		errStr, err := invokeProto(ctx, reg, UsersPluginID(runtime.GOOS), "list_users", req, &out)
		if err != nil {
			return &v2pb.UserListResponse{Error: err.Error()}
		}
		if errStr != "" {
			return &v2pb.UserListResponse{Error: errStr}
		}
		return &out
	}
}
