package plugin

import (
	"context"

	extism "github.com/extism/go-sdk"
	"google.golang.org/protobuf/encoding/protojson"

	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// host_collect_sysinfo wraps the agent's gopsutil-backed
// CollectSysInfo behind a host function. Same forwarding pattern as
// host_process_list — the data collection has too many platform
// branches (CPU, mem, disks, GPUs, network, virtualization) to
// reimplement under wasm/Rust today.
//
// Capability gate: CapSysInfo. Same trust posture as host_sysinfo
// (which only returns hostname); this is the rich variant returning
// the full SysInfoResponse.
//
// Wire: takes no input, returns the JSON envelope with `data` set to
// the protojson encoding of v2pb.SysInfoResponse. The bridge
// re-decodes via protojson.

type collectSysInfoProvider func(ctx context.Context) *v2pb.SysInfoResponse

var collectSysInfoImpl collectSysInfoProvider = func(_ context.Context) *v2pb.SysInfoResponse {
	return &v2pb.SysInfoResponse{Error: "host_collect_sysinfo not wired (agent build issue)"}
}

// SetHostCollectSysInfoProvider lets the agent main inject the
// gopsutil collector at startup, mirroring SetHostProcessListProvider.
func SetHostCollectSysInfoProvider(p collectSysInfoProvider) {
	if p != nil {
		collectSysInfoImpl = p
	}
}

func (pctx *pluginCtx) hostCollectSysInfo(ctx context.Context, p *extism.CurrentPlugin, stack []uint64) {
	if !pctx.granted[CapSysInfo] {
		returnEnvelope(p, stack, denied("sysinfo"))
		return
	}
	resp := collectSysInfoImpl(ctx)
	jsonBytes, err := protojson.Marshal(resp)
	if err != nil {
		returnEnvelope(p, stack, failed("marshal_response: "+err.Error()))
		return
	}
	returnEnvelope(p, stack, envelope{Ok: true, Data: jsonBytes})
}
