package plugin

import (
	"context"
	"encoding/json"

	extism "github.com/extism/go-sdk"
	"google.golang.org/protobuf/encoding/protojson"

	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// host_process_list is the host function that wraps the agent's
// gopsutil-backed CollectProcessList. The system sys-procs plugin
// just forwards its inbound request here — the data collection
// itself is too gopsutil-heavy to reimplement in wasm/Rust today.
//
// Capability gate: CapSysInfo (process info is read-only host
// observation; same trust posture as hostname).
//
// Wire: takes a JSON {top_n, sort_by} request, returns the JSON-
// encoded protobuf for ProcessListResponse (encoded via protojson so
// the field names + casing match what the bridge wrapper expects on
// its way back into a typed *v2pb.ProcessListResponse).

// hostProcessListProvider is the indirection that lets tests inject
// a fake collector. Production wires this to agent.CollectProcessList
// in cmd/platypus-agent/main.go (via Registry construction options).
// Default returns a "not configured" envelope so a plugin built
// against the API gracefully degrades when the host wasn't wired.
type hostProcessListProvider func(ctx context.Context, topN uint32, sortBy string) *v2pb.ProcessListResponse

var hostProcessListImpl hostProcessListProvider = func(_ context.Context, _ uint32, _ string) *v2pb.ProcessListResponse {
	return &v2pb.ProcessListResponse{Error: "host_process_list not wired (agent build issue)"}
}

// SetHostProcessListProvider lets the agent main wire the gopsutil
// collector in without creating an agent → plugin → agent import
// cycle. Called once at startup.
func SetHostProcessListProvider(p hostProcessListProvider) {
	if p != nil {
		hostProcessListImpl = p
	}
}

type processListHostRequest struct {
	TopN   uint32 `json:"top_n,omitempty"`
	SortBy string `json:"sort_by,omitempty"`
}

func (pctx *pluginCtx) hostProcessList(ctx context.Context, p *extism.CurrentPlugin, stack []uint64) {
	if !pctx.granted[CapSysInfo] {
		returnEnvelope(p, stack, denied("sysinfo"))
		return
	}
	raw, err := readStringArg(p, stack[0])
	if err != nil {
		returnEnvelope(p, stack, failed("read_request: "+err.Error()))
		return
	}
	var req processListHostRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		returnEnvelope(p, stack, failed("decode_request: "+err.Error()))
		return
	}
	resp := hostProcessListImpl(ctx, req.TopN, req.SortBy)
	jsonBytes, err := protojson.Marshal(resp)
	if err != nil {
		returnEnvelope(p, stack, failed("marshal_response: "+err.Error()))
		return
	}
	returnEnvelope(p, stack, envelope{Ok: true, Data: jsonBytes})
}
