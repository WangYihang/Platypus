package bridge

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/encoding/protojson"

	"github.com/WangYihang/Platypus/internal/agent/plugin"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// securityPluginID owns SecurityScan + ListSecurityChecks. Both are
// forwarders: the request is marshalled as protojson on the way in,
// the response is decoded as protojson on the way out.
const securityPluginID = "com.platypus.sys-security"

// SecurityScan is the plugin-backed replacement for
// agent.HandleSecurityScan.
func SecurityScan(reg *plugin.Registry) func(ctx context.Context, req *v2pb.SecurityScanRequest) *v2pb.SecurityScanResponse {
	return func(ctx context.Context, req *v2pb.SecurityScanRequest) *v2pb.SecurityScanResponse {
		body, err := protojson.Marshal(req)
		if err != nil {
			return &v2pb.SecurityScanResponse{Error: "bridge: marshal request: " + err.Error()}
		}
		r := reg.Invoke(ctx, &v2pb.PluginCallRequest{
			PluginId: securityPluginID, Method: "security_scan", Payload: body,
		})
		if r.GetError() != "" {
			return &v2pb.SecurityScanResponse{Error: r.GetError()}
		}
		var out v2pb.SecurityScanResponse
		if err := protojson.Unmarshal(r.GetPayload(), &out); err != nil {
			return &v2pb.SecurityScanResponse{
				Error: fmt.Sprintf("bridge: unmarshal protojson: %v", err),
			}
		}
		return &out
	}
}

// ListSecurityChecks is the plugin-backed replacement for
// agent.HandleListSecurityChecks.
func ListSecurityChecks(reg *plugin.Registry) func(ctx context.Context, req *v2pb.ListSecurityChecksRequest) *v2pb.ListSecurityChecksResponse {
	return func(ctx context.Context, _ *v2pb.ListSecurityChecksRequest) *v2pb.ListSecurityChecksResponse {
		r := reg.Invoke(ctx, &v2pb.PluginCallRequest{
			PluginId: securityPluginID, Method: "list_security_checks",
		})
		if r.GetError() != "" {
			return &v2pb.ListSecurityChecksResponse{Error: r.GetError()}
		}
		var out v2pb.ListSecurityChecksResponse
		if err := protojson.Unmarshal(r.GetPayload(), &out); err != nil {
			return &v2pb.ListSecurityChecksResponse{
				Error: fmt.Sprintf("bridge: unmarshal protojson: %v", err),
			}
		}
		return &out
	}
}
