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

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/WangYihang/Platypus/internal/agent/plugin"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// invokeJSON marshals `req` to JSON, calls the named plugin method
// via reg, and unmarshals the response JSON into `resp`. Returns the
// plugin-level error string (empty on success) plus any wire-level
// error the bridge couldn't paper over.
//
// When the plugin isn't installed the registry returns
// `plugin_not_installed: <id>` in r.GetError(); the bridge passes
// that through unchanged so the server REST + frontend can humanize
// it ("This feature needs plugin X — install it from the Plugins tab").
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
	if errStr := r.GetError(); errStr != "" {
		return errStr, nil
	}
	if err := json.Unmarshal(r.GetPayload(), resp); err != nil {
		return "", fmt.Errorf("bridge: unmarshal response: %w", err)
	}
	return "", nil
}

// invokeProto is the typed-message variant of invokeJSON. It uses
// protojson with UseProtoNames=true so the request goes out as the
// proto field's snake_case name — which matches what every Rust
// plugin's serde-default Deserialize expects (the plugins were
// authored against the snake_case JSON shape long before we added
// proto definitions for them). The response is decoded with default
// protojson Unmarshal which accepts both snake_case and lowerCamelCase
// — so a plugin emitting `currentVersion` and a plugin emitting
// `current_version` both round-trip into the same proto field.
//
// Use this for any plugin where you've defined a real proto message
// (sys-pkg, sys-disk, sys-net, sys-services, sys-journald, sys-info,
// sys-procs, sys-security, sys-config-audit). Use invokeJSON only
// for ad-hoc payloads that don't have a proto.
var (
	protoMarshalSnakeCase  = protojson.MarshalOptions{UseProtoNames: true}
	protoUnmarshalLenient  = protojson.UnmarshalOptions{DiscardUnknown: true}
)

func invokeProto(ctx context.Context, reg *plugin.Registry, pluginID, method string, req, resp proto.Message) (string, error) {
	payload, err := protoMarshalSnakeCase.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("bridge: marshal protojson request: %w", err)
	}
	r := reg.Invoke(ctx, &v2pb.PluginCallRequest{
		PluginId: pluginID,
		Method:   method,
		Payload:  payload,
	})
	if errStr := r.GetError(); errStr != "" {
		return errStr, nil
	}
	if err := protoUnmarshalLenient.Unmarshal(r.GetPayload(), resp); err != nil {
		return "", fmt.Errorf("bridge: unmarshal protojson response: %w", err)
	}
	return "", nil
}
