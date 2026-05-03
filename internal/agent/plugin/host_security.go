package plugin

import (
	"context"
	"encoding/json"

	extism "github.com/extism/go-sdk"
	"google.golang.org/protobuf/encoding/protojson"

	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// host_security_scan / host_list_security_checks wrap the agent's
// hardening-check registry behind the plugin runtime. Same
// forwarding shape as host_process_list / host_collect_sysinfo: the
// check definitions live in agent/security and the host fn delegates
// there.
//
// Capability gate: CapSysInfo. Hardening checks are observation only
// (no host mutations); the same trust posture as other read-only
// inventory RPCs.

type securityScanProvider func(ctx context.Context, req *v2pb.SecurityScanRequest) *v2pb.SecurityScanResponse
type listSecurityChecksProvider func(ctx context.Context, req *v2pb.ListSecurityChecksRequest) *v2pb.ListSecurityChecksResponse

var (
	securityScanImpl securityScanProvider = func(_ context.Context, _ *v2pb.SecurityScanRequest) *v2pb.SecurityScanResponse {
		return &v2pb.SecurityScanResponse{Error: "host_security_scan not wired"}
	}
	listSecurityChecksImpl listSecurityChecksProvider = func(_ context.Context, _ *v2pb.ListSecurityChecksRequest) *v2pb.ListSecurityChecksResponse {
		return &v2pb.ListSecurityChecksResponse{Error: "host_list_security_checks not wired"}
	}
)

func SetHostSecurityScanProvider(p securityScanProvider) {
	if p != nil {
		securityScanImpl = p
	}
}

func SetHostListSecurityChecksProvider(p listSecurityChecksProvider) {
	if p != nil {
		listSecurityChecksImpl = p
	}
}

func (pctx *pluginCtx) hostSecurityScan(ctx context.Context, p *extism.CurrentPlugin, stack []uint64) {
	if !pctx.granted[CapSysInfo] {
		returnEnvelope(p, stack, denied("sysinfo"))
		return
	}
	raw, err := readStringArg(p, stack[0])
	if err != nil {
		returnEnvelope(p, stack, failed("read_request: "+err.Error()))
		return
	}
	var req v2pb.SecurityScanRequest
	if err := protojson.Unmarshal([]byte(raw), &req); err != nil {
		// Tolerate old-shape requests: empty body should produce a
		// default-everything scan rather than crash.
		var alt struct{}
		if err2 := json.Unmarshal([]byte(raw), &alt); err2 != nil {
			returnEnvelope(p, stack, failed("decode_request: "+err.Error()))
			return
		}
	}
	resp := securityScanImpl(ctx, &req)
	jsonBytes, err := protojson.Marshal(resp)
	if err != nil {
		returnEnvelope(p, stack, failed("marshal_response: "+err.Error()))
		return
	}
	returnEnvelope(p, stack, envelope{Ok: true, Data: jsonBytes})
}

func (pctx *pluginCtx) hostListSecurityChecks(ctx context.Context, p *extism.CurrentPlugin, stack []uint64) {
	if !pctx.granted[CapSysInfo] {
		returnEnvelope(p, stack, denied("sysinfo"))
		return
	}
	resp := listSecurityChecksImpl(ctx, &v2pb.ListSecurityChecksRequest{})
	jsonBytes, err := protojson.Marshal(resp)
	if err != nil {
		returnEnvelope(p, stack, failed("marshal_response: "+err.Error()))
		return
	}
	returnEnvelope(p, stack, envelope{Ok: true, Data: jsonBytes})
}
