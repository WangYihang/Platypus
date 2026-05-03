package plugin

import (
	"context"

	extism "github.com/extism/go-sdk"
	"google.golang.org/protobuf/encoding/protojson"

	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// host_config_audit / host_list_config_auditors wrap the agent's
// gitleaks-backed credential-audit registry. Same forwarding pattern
// as host_security_scan; the auditor implementations live in
// agent/config_audit and the host fn delegates there.
//
// Capability gate: CapSysInfo. Auditors are observation-only (they
// read host files via the agent's existing file access, never
// modify); same trust posture as the other read-only inventory
// RPCs.

type configAuditProvider func(ctx context.Context, req *v2pb.ConfigAuditRequest) *v2pb.ConfigAuditResponse
type listConfigAuditorsProvider func(ctx context.Context, req *v2pb.ListConfigAuditorsRequest) *v2pb.ListConfigAuditorsResponse

var (
	configAuditImpl configAuditProvider = func(_ context.Context, _ *v2pb.ConfigAuditRequest) *v2pb.ConfigAuditResponse {
		return &v2pb.ConfigAuditResponse{Error: "host_config_audit not wired"}
	}
	listConfigAuditorsImpl listConfigAuditorsProvider = func(_ context.Context, _ *v2pb.ListConfigAuditorsRequest) *v2pb.ListConfigAuditorsResponse {
		return &v2pb.ListConfigAuditorsResponse{Error: "host_list_config_auditors not wired"}
	}
)

func SetHostConfigAuditProvider(p configAuditProvider) {
	if p != nil {
		configAuditImpl = p
	}
}

func SetHostListConfigAuditorsProvider(p listConfigAuditorsProvider) {
	if p != nil {
		listConfigAuditorsImpl = p
	}
}

func (pctx *pluginCtx) hostConfigAudit(ctx context.Context, p *extism.CurrentPlugin, stack []uint64) {
	if !pctx.granted[CapSysInfo] {
		returnEnvelope(p, stack, denied("sysinfo"))
		return
	}
	raw, err := readStringArg(p, stack[0])
	if err != nil {
		returnEnvelope(p, stack, failed("read_request: "+err.Error()))
		return
	}
	var req v2pb.ConfigAuditRequest
	if err := protojson.Unmarshal([]byte(raw), &req); err != nil {
		// Tolerate empty / minimal requests — same accommodation as
		// host_security_scan.
		req = v2pb.ConfigAuditRequest{}
	}
	resp := configAuditImpl(ctx, &req)
	jsonBytes, err := protojson.Marshal(resp)
	if err != nil {
		returnEnvelope(p, stack, failed("marshal_response: "+err.Error()))
		return
	}
	returnEnvelope(p, stack, envelope{Ok: true, Data: jsonBytes})
}

func (pctx *pluginCtx) hostListConfigAuditors(ctx context.Context, p *extism.CurrentPlugin, stack []uint64) {
	if !pctx.granted[CapSysInfo] {
		returnEnvelope(p, stack, denied("sysinfo"))
		return
	}
	resp := listConfigAuditorsImpl(ctx, &v2pb.ListConfigAuditorsRequest{})
	jsonBytes, err := protojson.Marshal(resp)
	if err != nil {
		returnEnvelope(p, stack, failed("marshal_response: "+err.Error()))
		return
	}
	returnEnvelope(p, stack, envelope{Ok: true, Data: jsonBytes})
}
