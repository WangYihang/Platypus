package bridge

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/encoding/protojson"

	"github.com/WangYihang/Platypus/internal/agent/plugin"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// sysInfoPluginID owns the SysInfo RPC. Like sys-procs, the plugin
// forwards to a host fn (host_collect_sysinfo) and the response is
// already protojson — no JSON-struct intermediate needed in the
// bridge.
const sysInfoPluginID = "com.platypus.sys-info"

// SysInfo is the plugin-backed replacement for agent.HandleSysInfo.
// SysInfoRequest is currently a single empty message, so the wire
// payload is empty bytes — we still send it through Invoke for
// consistent audit + capability accounting.
func SysInfo(reg *plugin.Registry) func(ctx context.Context, req *v2pb.SysInfoRequest) *v2pb.SysInfoResponse {
	return func(ctx context.Context, _ *v2pb.SysInfoRequest) *v2pb.SysInfoResponse {
		r := reg.Invoke(ctx, &v2pb.PluginCallRequest{
			PluginId: sysInfoPluginID, Method: "sys_info",
		})
		if r.GetError() != "" {
			return &v2pb.SysInfoResponse{Error: r.GetError()}
		}
		var out v2pb.SysInfoResponse
		if err := protojson.Unmarshal(r.GetPayload(), &out); err != nil {
			return &v2pb.SysInfoResponse{
				Error: fmt.Sprintf("bridge: unmarshal protojson: %v", err),
			}
		}
		return &out
	}
}
