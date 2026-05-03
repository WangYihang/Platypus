package bridge

import (
	"context"
	"encoding/json"
	"fmt"

	"google.golang.org/protobuf/encoding/protojson"

	"github.com/WangYihang/Platypus/internal/agent/plugin"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// procsPluginID owns the ProcessList RPC. The plugin's response is
// already in protojson form (the host fn marshals
// v2pb.ProcessListResponse with protojson, the plugin forwards
// straight through), so the bridge only does protojson.Unmarshal —
// no intermediate JSON struct.
const procsPluginID = "com.platypus.sys-procs"

// ProcessList is the plugin-backed replacement for
// agent.HandleProcessList.
func ProcessList(reg *plugin.Registry) func(ctx context.Context, req *v2pb.ProcessListRequest) *v2pb.ProcessListResponse {
	return func(ctx context.Context, req *v2pb.ProcessListRequest) *v2pb.ProcessListResponse {
		payload, err := json.Marshal(processListJSON{
			TopN: req.GetTopN(), SortBy: req.GetSortBy(),
		})
		if err != nil {
			return &v2pb.ProcessListResponse{Error: "bridge: " + err.Error()}
		}
		r := reg.Invoke(ctx, &v2pb.PluginCallRequest{
			PluginId: procsPluginID, Method: "process_list", Payload: payload,
		})
		if r.GetError() != "" {
			return &v2pb.ProcessListResponse{Error: r.GetError()}
		}
		var out v2pb.ProcessListResponse
		if err := protojson.Unmarshal(r.GetPayload(), &out); err != nil {
			return &v2pb.ProcessListResponse{
				Error: fmt.Sprintf("bridge: unmarshal protojson: %v", err),
			}
		}
		return &out
	}
}

type processListJSON struct {
	TopN   uint32 `json:"top_n"`
	SortBy string `json:"sort_by"`
}
