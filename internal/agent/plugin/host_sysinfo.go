package plugin

import (
	"context"
	"os"

	extism "github.com/extism/go-sdk"
)

// hostSysInfo returns a tiny snapshot — hostname only for MVP — without
// pulling gopsutil into the plugin runtime. Plugins that need the full
// snapshot should ask the server to invoke the SysInfo RPC instead;
// this host function exists for cheap intra-call platform branching.
//
// Capability gate: CapSysInfo. Even though the data is non-sensitive,
// we still gate to give operators a single audit-visible point — "did
// this plugin read host identity?".
func (pctx *pluginCtx) hostSysInfo(_ context.Context, p *extism.CurrentPlugin, stack []uint64) {
	if !pctx.granted[CapSysInfo] {
		returnEnvelope(p, stack, denied("sysinfo"))
		return
	}
	host, _ := os.Hostname()
	returnEnvelope(p, stack, okData(map[string]string{
		"hostname": host,
	}))
}
