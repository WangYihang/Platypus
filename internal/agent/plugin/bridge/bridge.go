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
	payload, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("bridge: marshal request: %w", err)
	}
	r := reg.Invoke(ctx, &v2pb.PluginCallRequest{
		PluginId: pluginID,
		Method:   method,
		Payload:  payload,
	})
	if r.GetError() != "" {
		return r.GetError(), nil
	}
	if err := json.Unmarshal(r.GetPayload(), resp); err != nil {
		return "", fmt.Errorf("bridge: unmarshal response: %w", err)
	}
	return "", nil
}
