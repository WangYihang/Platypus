package plugin

import (
	"context"
	"encoding/json"
	"errors"

	agentplugin "github.com/WangYihang/Platypus/internal/agent/plugin"
)

// ResolveAndValidate is the single composed entry point every server
// path uses to turn an operator-authored config (with secret refs)
// into a wire-ready blob. The two underlying steps stay available
// individually for callers that need just one — but the install
// handler, the preset save path, and the rollout reconciler all go
// through this composer so the order is uniform: resolve secrets
// first, then validate against the manifest's schema.
//
// Order matters. Validating the saved (with-refs) form would have
// the schema see {"$secret":"..."} objects everywhere a string is
// expected, producing useless type errors. Resolving first means
// the schema validates the actual plaintext that the agent will
// see — including, importantly, length / format / regex
// constraints that catch a freshly-rotated secret whose value
// doesn't match the plugin's expectations.
//
// Returns the resolved + validated config bytes on success, ready
// to ship over the wire. On failure, returns an error whose message
// names either the JSON Pointer of the offending field (resolver
// path) or the schema rule that didn't match (validator path) —
// both designed for verbatim display in the UI's "fix the
// following" list.
func ResolveAndValidate(
	ctx context.Context,
	reader SecretReader,
	manifest *agentplugin.Manifest,
	overrides json.RawMessage,
	schemaVersion int,
) (json.RawMessage, error) {
	if manifest == nil {
		return nil, errors.New("plugin: ResolveAndValidate: manifest is nil")
	}
	resolved, err := ResolveSecrets(ctx, reader, overrides)
	if err != nil {
		return nil, err
	}
	if err := ValidateConfig(manifest, resolved, schemaVersion); err != nil {
		return nil, err
	}
	return resolved, nil
}
