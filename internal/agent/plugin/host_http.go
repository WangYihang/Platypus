package plugin

import (
	"context"

	extism "github.com/extism/go-sdk"
)

// hostHTTP is intentionally a stub in MVP: the wire envelope is in
// place so plugin authors can target the API today, but the actual
// HTTP roundtrip lands with the marketplace work in Phase 2 (when
// per-host network capability disclosure to the operator is fleshed
// out). For now we always return capability_denied so a plugin that
// compiles against the API gracefully degrades.
//
// Capability gate (when implemented): CapNetHTTP plus a per-host
// allowlist drawn from manifest.capabilities.net.http.hosts.
func (pctx *pluginCtx) hostHTTP(_ context.Context, p *extism.CurrentPlugin, stack []uint64) {
	returnEnvelope(p, stack, denied("net.http (not implemented in MVP)"))
}
