package bridge

import (
	"context"
	"fmt"
	"time"

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
//
// Wall-clock metadata (started_at_unix, elapsed_ms) is filled in by
// the bridge, not the plugin: wasm has no host_clock host fn, and
// asking every check author to thread time through the wasm boundary
// is the wrong layer. The bridge captures `time.Now()` around the
// invocation and patches both fields when the plugin left them at
// zero — preserves any future plugin that does report its own timing.
func SecurityScan(reg *plugin.Registry) func(ctx context.Context, req *v2pb.SecurityScanRequest) *v2pb.SecurityScanResponse {
	return func(ctx context.Context, req *v2pb.SecurityScanRequest) *v2pb.SecurityScanResponse {
		body, err := protojson.Marshal(req)
		if err != nil {
			return &v2pb.SecurityScanResponse{Error: "bridge: marshal request: " + err.Error()}
		}
		started := time.Now()
		r := reg.Invoke(ctx, &v2pb.PluginCallRequest{
			PluginId: securityPluginID, Method: "security_scan", Payload: body,
		})
		elapsed := time.Since(started)
		if r.GetError() != "" {
			return &v2pb.SecurityScanResponse{Error: r.GetError()}
		}
		var out v2pb.SecurityScanResponse
		if err := protojson.Unmarshal(r.GetPayload(), &out); err != nil {
			return &v2pb.SecurityScanResponse{
				Error: fmt.Sprintf("bridge: unmarshal protojson: %v", err),
			}
		}
		if out.StartedAtUnix == 0 {
			out.StartedAtUnix = started.Unix()
		}
		if out.ElapsedMs == 0 {
			out.ElapsedMs = uint64(elapsed.Milliseconds())
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
