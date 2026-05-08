package bridge

import (
	"context"
	"runtime"

	"github.com/WangYihang/Platypus/internal/agent/plugin"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// NetPluginID returns the per-OS sys-net plugin id.
func NetPluginID(goos string) string {
	switch goos {
	case "darwin":
		return "com.platypus.sys-net-darwin"
	case "windows":
		return "com.platypus.sys-net-windows"
	default:
		return "com.platypus.sys-net-linux"
	}
}

// ListListeners forwards to the per-OS sys-net plugin's
// list_listeners RPC.
func ListListeners(reg *plugin.Registry) func(ctx context.Context, req *v2pb.ListListenersRequest) *v2pb.ListListenersResponse {
	return func(ctx context.Context, req *v2pb.ListListenersRequest) *v2pb.ListListenersResponse {
		var out v2pb.ListListenersResponse
		errStr, err := invokeProto(ctx, reg, NetPluginID(runtime.GOOS), "list_listeners", req, &out)
		if err != nil {
			return &v2pb.ListListenersResponse{Error: err.Error()}
		}
		if errStr != "" {
			return &v2pb.ListListenersResponse{Error: errStr}
		}
		return &out
	}
}

// ListConnections forwards to list_connections on the same per-OS
// plugin.
func ListConnections(reg *plugin.Registry) func(ctx context.Context, req *v2pb.ListConnectionsRequest) *v2pb.ListConnectionsResponse {
	return func(ctx context.Context, req *v2pb.ListConnectionsRequest) *v2pb.ListConnectionsResponse {
		var out v2pb.ListConnectionsResponse
		errStr, err := invokeProto(ctx, reg, NetPluginID(runtime.GOOS), "list_connections", req, &out)
		if err != nil {
			return &v2pb.ListConnectionsResponse{Error: err.Error()}
		}
		if errStr != "" {
			return &v2pb.ListConnectionsResponse{Error: errStr}
		}
		return &out
	}
}
