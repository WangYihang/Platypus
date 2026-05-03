package plugin

import (
	"context"
	"encoding/json"
	"runtime"

	extism "github.com/extism/go-sdk"
)

// host_uname returns the host's os/arch/kernel info — primitives the
// plugin can't derive itself because it executes as wasm32, so
// runtime.GOOS / runtime.GOARCH from inside the wasm sandbox would
// say "wasi"/"wasm32" not "linux"/"amd64".
//
// This is intentionally NOT a domain-shaped response (no parsing
// logic, no field combinations) — it's a syscall-equivalent
// primitive. Plugins that need richer info compose it themselves
// from this + host_fs_read of /proc/version, /proc/cpuinfo, etc.
//
// Capability gate: CapSysInfo. Returned shape:
//
//	{
//	  "os":             "linux"  | "darwin" | "windows" | ...
//	  "arch":           "amd64"  | "arm64"  | ...
//	  "go_version":     "go1.25.9"
//	}

type unameJSON struct {
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	GoVersion string `json:"go_version"`
}

func (pctx *pluginCtx) hostUname(_ context.Context, p *extism.CurrentPlugin, stack []uint64) {
	if !pctx.granted[CapSysInfo] {
		returnEnvelope(p, stack, denied("sysinfo"))
		return
	}
	body, _ := json.Marshal(unameJSON{
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		GoVersion: runtime.Version(),
	})
	returnEnvelope(p, stack, envelope{Ok: true, Data: body})
}
