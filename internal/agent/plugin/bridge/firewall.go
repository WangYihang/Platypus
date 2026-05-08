package bridge

import (
	"context"
	"runtime"

	"github.com/WangYihang/Platypus/internal/agent/plugin"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// FirewallPluginID returns the per-OS sys-firewall plugin id.
func FirewallPluginID(goos string) string {
	switch goos {
	case "darwin":
		return "com.platypus.sys-firewall-darwin"
	case "windows":
		return "com.platypus.sys-firewall-windows"
	default:
		return "com.platypus.sys-firewall-linux"
	}
}

// FirewallList forwards to the per-OS sys-firewall plugin's
// list_firewall_rules RPC. The response carries the detected
// backend ("iptables" / "nftables" / "ufw" / "firewalld" / "pf" /
// "windows-firewall") so the UI can show "this host runs ufw"
// without an extra round trip.
func FirewallList(reg *plugin.Registry) func(ctx context.Context, req *v2pb.FirewallListRequest) *v2pb.FirewallListResponse {
	return func(ctx context.Context, req *v2pb.FirewallListRequest) *v2pb.FirewallListResponse {
		var out v2pb.FirewallListResponse
		errStr, err := invokeProto(ctx, reg, FirewallPluginID(runtime.GOOS), "list_firewall_rules", req, &out)
		if err != nil {
			return &v2pb.FirewallListResponse{Error: err.Error()}
		}
		if errStr != "" {
			return &v2pb.FirewallListResponse{Error: errStr}
		}
		return &out
	}
}
