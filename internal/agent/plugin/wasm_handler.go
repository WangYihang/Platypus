package plugin

import "strings"

// wasmHandlerPrefix is the manifest marker that flips a streams[]
// entry from "claim-only host provider dispatch" (host_handler =
// "agent.X") to "wasm-mediated dispatch through this plugin's wasm"
// (host_handler = "wasm:<method_name>"). Centralised so the
// dispatcher + the manifest validator stay in sync.
const wasmHandlerPrefix = "wasm:"

// parseWasmHandler returns (method, true) when handler is the
// wasm-dispatch marker — and (zero, false) otherwise. method is the
// wasm export name to invoke for this stream.
//
// Rejects "wasm:" with empty method, the bare "wasm" string, and
// any typo of the prefix; the dispatcher treats those as
// non-wasm so the stream falls through to the legacy host-provider
// claim path (or, if no provider is registered either, the
// errStreamUnsupported error path).
func parseWasmHandler(handler string) (method string, isWasm bool) {
	if !strings.HasPrefix(handler, wasmHandlerPrefix) {
		return "", false
	}
	method = handler[len(wasmHandlerPrefix):]
	if method == "" {
		return "", false
	}
	return method, true
}
