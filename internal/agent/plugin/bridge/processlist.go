package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"

	"google.golang.org/protobuf/encoding/protojson"

	"github.com/WangYihang/Platypus/internal/agent/plugin"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// procsPluginIDByOS maps the agent's runtime.GOOS to the per-OS
// plugin id that owns the ProcessList RPC. Renamed from a single
// flat `com.platypus.sys-procs` constant in Sprint 2's M0 to align
// with the per-OS naming convention; M1a/M1b will populate the
// darwin/windows entries with their own /proc / ps / Get-Process
// shell-out plugins.
//
// The plugin's response is already in protojson form (its wasm
// marshals v2pb.ProcessListResponse straight through), so the
// bridge only does protojson.Unmarshal — no intermediate JSON struct.
var procsPluginIDByOS = map[string]string{
	"linux": "com.platypus.sys-procs-linux",
	// "darwin":  "com.platypus.sys-procs-darwin",  // M1a
	// "windows": "com.platypus.sys-procs-windows", // M1b
}

// procsPluginIDFor picks the OS-specific plugin id; falls back to
// the linux plugin on unknown OSes (the plugin will fail at
// host_fs_read of /proc which is the cleanest "not supported"
// surface).
func procsPluginIDFor(goos string) string {
	if id, ok := procsPluginIDByOS[goos]; ok {
		return id
	}
	return procsPluginIDByOS["linux"]
}

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
			PluginId: procsPluginIDFor(runtime.GOOS), Method: "process_list", Payload: payload,
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
