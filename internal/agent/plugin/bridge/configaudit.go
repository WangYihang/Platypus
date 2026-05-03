package bridge

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/encoding/protojson"

	"github.com/WangYihang/Platypus/internal/agent/plugin"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// configAuditPluginID owns ConfigAudit + ListConfigAuditors. Same
// forwarding shape as sys-security; the gitleaks-backed auditor
// registry stays in agent/config_audit.
const configAuditPluginID = "com.platypus.sys-config-audit"

// ConfigAudit is the plugin-backed replacement for
// agent.HandleConfigAudit.
func ConfigAudit(reg *plugin.Registry) func(ctx context.Context, req *v2pb.ConfigAuditRequest) *v2pb.ConfigAuditResponse {
	return func(ctx context.Context, req *v2pb.ConfigAuditRequest) *v2pb.ConfigAuditResponse {
		body, err := protojson.Marshal(req)
		if err != nil {
			return &v2pb.ConfigAuditResponse{Error: "bridge: marshal request: " + err.Error()}
		}
		r := reg.Invoke(ctx, &v2pb.PluginCallRequest{
			PluginId: configAuditPluginID, Method: "config_audit", Payload: body,
		})
		if r.GetError() != "" {
			return &v2pb.ConfigAuditResponse{Error: r.GetError()}
		}
		var out v2pb.ConfigAuditResponse
		if err := protojson.Unmarshal(r.GetPayload(), &out); err != nil {
			return &v2pb.ConfigAuditResponse{
				Error: fmt.Sprintf("bridge: unmarshal protojson: %v", err),
			}
		}
		return &out
	}
}

// ListConfigAuditors is the plugin-backed replacement for
// agent.HandleListConfigAuditors.
func ListConfigAuditors(reg *plugin.Registry) func(ctx context.Context, req *v2pb.ListConfigAuditorsRequest) *v2pb.ListConfigAuditorsResponse {
	return func(ctx context.Context, _ *v2pb.ListConfigAuditorsRequest) *v2pb.ListConfigAuditorsResponse {
		r := reg.Invoke(ctx, &v2pb.PluginCallRequest{
			PluginId: configAuditPluginID, Method: "list_config_auditors",
		})
		if r.GetError() != "" {
			return &v2pb.ListConfigAuditorsResponse{Error: r.GetError()}
		}
		var out v2pb.ListConfigAuditorsResponse
		if err := protojson.Unmarshal(r.GetPayload(), &out); err != nil {
			return &v2pb.ListConfigAuditorsResponse{
				Error: fmt.Sprintf("bridge: unmarshal protojson: %v", err),
			}
		}
		return &out
	}
}
