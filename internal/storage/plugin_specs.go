package storage

import (
	"encoding/json"
	"fmt"

	agentplugin "github.com/WangYihang/Platypus/internal/agent/plugin"
)

// PluginSpec is the atomic deployment unit for a plugin: which plugin,
// at which version, with which capabilities granted, and with which
// configuration overrides on top of the manifest's defaults. The same
// struct serialises across every layer that carries plugin deployment
// intent — enrollment_presets.plugin_specs, install_download_tokens.
// plugin_specs, hosts.plugin_specs, host_plugins, and the wire-side
// PluginInstallRequest.PluginSpec — so each layer's encode/decode goes
// through one helper rather than its own ad-hoc translation.
//
// Two flavours of ConfigOverrides values exist in flight:
//
//   - "saved" (in storage / API request bodies): values for fields
//     marked secret in the manifest are SecretRef objects shaped as
//     {"$secret": "sec_<id>"}. Plaintext never lives at rest in
//     anything outside project_secrets.
//   - "resolved" (after secrets are substituted, before being sent to
//     an agent and recorded in host_plugins.config_resolved): values
//     are inline literals of the type the schema requires.
//
// The struct itself can't tell the two apart by type — both forms are
// valid JSON objects — so resolution status is an out-of-band fact
// the caller tracks.
type PluginSpec struct {
	PluginID            string                     `json:"plugin_id"`
	Version             string                     `json:"version,omitempty"`
	// GrantedCapabilities uses the typed CapabilityID so the
	// compiler catches unknown families everywhere a PluginSpec
	// is constructed in Go. The on-the-wire JSON shape is the
	// same `["fs.read", ...]` array — encoding/json marshals
	// string-derived types as plain strings, so existing
	// consumers see no diff.
	GrantedCapabilities []agentplugin.CapabilityID `json:"granted_capabilities,omitempty"`
	// ConfigOverrides holds the operator's deltas over the manifest's
	// schema defaults. json.RawMessage so we never round-trip through
	// a Go-typed AST: the schema is plugin-author-controlled, so the
	// server passes the bytes through and validates against the schema
	// elsewhere.
	ConfigOverrides json.RawMessage `json:"config_overrides,omitempty"`
	// SchemaVersion pins which version of the manifest's config_schema
	// these overrides were authored against. The agent and the server
	// validator both refuse to apply a spec whose schema_version
	// doesn't match the manifest currently published — without that,
	// a manifest update that adds a required field would silently
	// produce undefined behaviour on every previously-deployed host.
	SchemaVersion int `json:"schema_version,omitempty"`
}

// SecretRef is the canonical shape a config-override value takes when
// the field is marked secret in the manifest. The agent never sees
// this — secrets are resolved on the server before the wire request.
//
// Authored as a struct (not just a map) so misuses are caught at
// compile time inside the server.
type SecretRef struct {
	SecretID string `json:"$secret"`
}

// IsSecretRef reports whether a JSON value matches the SecretRef shape.
// Used by the resolver to pick out fields that need substitution.
func IsSecretRef(value json.RawMessage) (string, bool) {
	if len(value) == 0 {
		return "", false
	}
	var ref SecretRef
	if err := json.Unmarshal(value, &ref); err != nil {
		return "", false
	}
	if ref.SecretID == "" {
		return "", false
	}
	// Reject objects that have $secret PLUS extra keys — only
	// {"$secret":"..."} is a valid ref. Otherwise an operator could
	// hide a real value under a $secret key by accident.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(value, &raw); err != nil {
		return "", false
	}
	if len(raw) != 1 {
		return "", false
	}
	return ref.SecretID, true
}

// EncodePluginSpecs marshals a slice of PluginSpec for storage in a
// TEXT column. Empty slices serialise as NULL so an unset field stays
// NULL rather than "[]" — keeps the "this preset has no plugins"
// signal crisp at the SQL level.
func EncodePluginSpecs(specs []PluginSpec) (interface{}, error) {
	if len(specs) == 0 {
		return nil, nil
	}
	b, err := json.Marshal(specs)
	if err != nil {
		return nil, fmt.Errorf("encode plugin specs: %w", err)
	}
	return string(b), nil
}

// DecodePluginSpecs unmarshals a stored TEXT column back into a slice.
// NULL or empty round-trips to nil — the inverse of EncodePluginSpecs.
func DecodePluginSpecs(s string) ([]PluginSpec, error) {
	if s == "" || s == "[]" {
		return nil, nil
	}
	var out []PluginSpec
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil, fmt.Errorf("decode plugin specs: %w", err)
	}
	return out, nil
}
