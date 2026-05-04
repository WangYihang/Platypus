// Package bridge wires the agent's wire-level RPC handlers to
// system-plugin invocations. Each migrated built-in handler keeps its
// existing AgentRPCHandlers signature so the dispatcher in
// internal/agent/rpc_stream.go doesn't change; the body of the
// handler is replaced by a call into pluginRegistry.Invoke,
// translating the typed proto request to a JSON payload the plugin
// can decode + the JSON response back into a typed proto.
//
// JSON is the bridge wire format on purpose: it lets plugin authors
// in any language target the same shape without owning Platypus's
// proto definitions, at the cost of two extra serialisations per
// call. Hot paths can switch to passing proto bytes directly later
// (extism-convert supports prost on the Rust side); MVP optimises
// for plugin-author ergonomics.
//
// One file per migrated handler family. Adding a migration =
// dropping a new file here + flipping main.go to call the bridge
// instead of agent.HandleX.
package bridge

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/WangYihang/Platypus/internal/agent/plugin"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// invokeJSON marshals `req` to JSON, calls the named plugin method
// via reg, and unmarshals the response JSON into `resp`. Returns the
// plugin-level error string (empty on success) plus any wire-level
// error the bridge couldn't paper over.
func invokeJSON(ctx context.Context, reg *plugin.Registry, pluginID, method string, req, resp any) (string, error) {
	return invokeJSONFallback(ctx, reg, []string{pluginID}, method, req, resp)
}

// invokeJSONFallback tries each plugin id in order until one
// responds without a "plugin not installed" error. Used by the
// post-merge bridges to support BOTH the merged plugin id (e.g.
// com.platypus.sys-files-read) AND the pre-merge legacy id
// (com.platypus.sys-listdir) in the same agent build.
//
// Why: the embed FS is signed with a publisher key whose private
// half isn't checked in. Until that pipeline gets re-run we can't
// drop new merged-plugin wasm into the embed FS, so production
// agents (which fall back to the embed FS) only have the legacy
// plugin ids — sys-listdir, sys-fs-write, sys-exec — installed.
// Dev agents (publisher-staged override-FS) install the merged
// ids. The bridge prefers the merged id (matches what dev / future
// builds ship) and falls back to the legacy id when it isn't
// installed.
//
// This fallback is transitional: once the embed FS pipeline ships
// merged-id wasm, the legacy ids can be dropped from the bridge
// constants. Tests pinning behaviour against either id stay valid
// during the transition.
func invokeJSONFallback(ctx context.Context, reg *plugin.Registry, pluginIDs []string, method string, req, resp any) (string, error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("bridge: marshal request: %w", err)
	}
	var lastErrStr string
	for _, id := range pluginIDs {
		r := reg.Invoke(ctx, &v2pb.PluginCallRequest{
			PluginId: id,
			Method:   method,
			Payload:  payload,
		})
		errStr := r.GetError()
		if errStr != "" {
			lastErrStr = errStr
			// Only fall through to the next id when the failure was
			// "this plugin/method isn't installed here". Other errors
			// (capability denied, timeout, malformed input) are
			// authoritative — re-trying a different plugin id would
			// just produce a noisier failure or worse, route to a
			// different plugin's logic.
			if isPluginNotInstalled(errStr) && len(pluginIDs) > 1 {
				continue
			}
			return errStr, nil
		}
		if err := json.Unmarshal(r.GetPayload(), resp); err != nil {
			return "", fmt.Errorf("bridge: unmarshal response: %w", err)
		}
		return "", nil
	}
	return lastErrStr, nil
}

// isPluginNotInstalled inspects the agent-side error string for the
// "plugin_not_installed: <id>" or "method <m> not exported by ..."
// shapes the registry emits when a lookup misses. Anything matching
// is a candidate for retrying against the next fallback id;
// everything else (capability denied, timeout, malformed input)
// stops the chain.
func isPluginNotInstalled(errStr string) bool {
	for _, marker := range []string{
		"plugin_not_installed",
		"plugin not installed",
		"not exported",
		"unknown plugin",
		"unknown method",
	} {
		if containsCI(errStr, marker) {
			return true
		}
	}
	return false
}

// containsCI is a tiny case-insensitive containment check so we
// don't need to pull strings.ToLower into the hot path of every
// bridge call. The marker set above is small and stable, so the
// nested loop cost is negligible.
func containsCI(haystack, needle string) bool {
	if len(needle) == 0 {
		return true
	}
	if len(haystack) < len(needle) {
		return false
	}
	hl := lowerASCII(haystack)
	nl := lowerASCII(needle)
	for i := 0; i+len(nl) <= len(hl); i++ {
		if hl[i:i+len(nl)] == nl {
			return true
		}
	}
	return false
}

func lowerASCII(s string) string {
	out := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if 'A' <= c && c <= 'Z' {
			c += 'a' - 'A'
		}
		out[i] = c
	}
	return string(out)
}
