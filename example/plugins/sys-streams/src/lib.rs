// sys-streams is a claim-only system plugin: its manifest declares
// ownership of every wire stream type the legacy agent serves
// (PROCESS_OPEN, FILE_READ, FILE_WRITE, FILE_SCAN, FILE_ARCHIVE,
// TUNNEL_PULL) and the agent's stream dispatcher delegates to the
// matching named StreamProvider — wired in
// cmd/platypus-agent/stream_adapters.go.
//
// The wasm itself is intentionally minimal because no in-wasm
// dispatch happens for streams in MVP. The single `noop` export
// exists so the manifest validator's "at least one rpc OR streams
// entry" check passes for plugins without rpc methods, and so
// extism's plugin instantiation has something to load.
//
// True wasm-mediated stream IO (bytes flowing through the plugin's
// linear memory mid-stream) is the bigger Phase 2 work described in
// docs/plugins/STREAMING_ABI.md. When that lands the host_handler
// values in the manifest will switch from "agent.X" host fn names
// to "wasm:method" markers and this file will gain real per-stream
// methods.

use extism_pdk::*;

#[plugin_fn]
pub fn noop(_: ()) -> FnResult<()> {
    Ok(())
}
