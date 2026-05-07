package plugin

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v5"
	"gopkg.in/yaml.v3"

	agentplugin "github.com/WangYihang/Platypus/internal/agent/plugin"
)

// ValidateConfig checks an operator-supplied config blob against a
// plugin manifest's declared schema. The function is the single
// shared callsite the server uses to enforce config correctness:
// the install handler runs it before pushing to an agent, the preset
// handler runs it before storing a PluginSpec, and the host-plugins
// repo treats validate-then-write as one atomic step. Sharing one
// validator means every entry point — REST API, CLI, future
// reconcilers — agrees on what counts as valid.
//
// Returns nil if the config validates. Returns an error whose
// message names the JSON pointer of the failing field plus the
// schema rule it violated (e.g. `/destination: required`,
// `/port: must be <= 65535`). Callers surface those verbatim to the
// operator — the messages are designed to read naturally in a UI
// "fix the following" list.
//
// configJSON is the resolved (post-secret-substitution) bytes; this
// function deliberately doesn't see secret refs. The caller is
// responsible for substitution before validation. That ordering is
// what lets the validator produce useful errors against actual
// values rather than against the {"$secret":"..."} placeholders.
//
// schemaVersion is the version the config was authored against. The
// validator refuses configs whose version doesn't match the
// manifest's current schema_version — schema migrations are a future
// feature, and silently applying a v1 config against a v2 schema
// risks security regressions (a removed required field, a tightened
// allowlist).
func ValidateConfig(manifest *agentplugin.Manifest, configJSON []byte, schemaVersion int) error {
	if manifest == nil {
		return errors.New("plugin: manifest is nil")
	}
	// The config block is optional. A plugin that doesn't declare a
	// schema accepts any (or no) config. PR 1 lands the validator
	// API; later PRs add stricter "no schema means no config
	// allowed" enforcement once the install path can carry rich
	// PluginSpecs end-to-end.
	if manifest.Config.Schema.IsZero() {
		if len(configJSON) > 0 && string(configJSON) != "null" && string(configJSON) != "{}" {
			return fmt.Errorf("plugin %q declares no config schema but a non-empty config was supplied", manifest.ID)
		}
		return nil
	}
	if manifest.Config.SchemaVersion != schemaVersion {
		return fmt.Errorf(
			"plugin %q config schema_version mismatch: spec has %d, manifest declares %d (re-author the config against the current schema)",
			manifest.ID, schemaVersion, manifest.Config.SchemaVersion,
		)
	}
	schemaJSON, err := yamlNodeToJSON(&manifest.Config.Schema)
	if err != nil {
		return fmt.Errorf("plugin %q: serialise config schema: %w", manifest.ID, err)
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("plugin-config-schema.json", bytes.NewReader(schemaJSON)); err != nil {
		return fmt.Errorf("plugin %q: compile config schema: %w", manifest.ID, err)
	}
	schema, err := compiler.Compile("plugin-config-schema.json")
	if err != nil {
		return fmt.Errorf("plugin %q: compile config schema: %w", manifest.ID, err)
	}
	var configValue interface{}
	if len(configJSON) == 0 {
		// jsonschema treats nil and {} differently for `required:` —
		// passing nil makes the validator generate "expected object,
		// got null" which isn't what we want. Decode as the empty
		// object so missing required fields surface as the actual
		// "missing X" error.
		configValue = map[string]interface{}{}
	} else if err := json.Unmarshal(configJSON, &configValue); err != nil {
		return fmt.Errorf("plugin %q: parse config JSON: %w", manifest.ID, err)
	}
	if err := schema.Validate(configValue); err != nil {
		// jsonschema.ValidationError formats nicely by default; we
		// wrap it with the plugin id so callers can route the error
		// to the right "fix this plugin's config" UI surface.
		return fmt.Errorf("plugin %q: config validation failed: %w", manifest.ID, err)
	}
	return nil
}

// yamlNodeToJSON serialises a YAML node tree into JSON bytes. JSON
// Schema is conventionally JSON, but plugin manifests are YAML — we
// take the YAML path through and convert at the boundary so plugin
// authors don't have to switch encodings mid-file.
func yamlNodeToJSON(n *yaml.Node) ([]byte, error) {
	var v interface{}
	if err := n.Decode(&v); err != nil {
		return nil, err
	}
	v = normaliseYAMLValue(v)
	return json.Marshal(v)
}

// normaliseYAMLValue rewrites map[interface{}]interface{} (which
// gopkg.in/yaml.v3 emits for nested maps) into
// map[string]interface{} so encoding/json can serialise it. JSON
// keys must be strings; YAML keys can be anything, so we coerce
// non-string keys via fmt.Sprintf — schemas with non-string keys
// are pathological and very rare, but we don't want to panic.
func normaliseYAMLValue(v interface{}) interface{} {
	switch t := v.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(t))
		for k, vv := range t {
			out[k] = normaliseYAMLValue(vv)
		}
		return out
	case map[interface{}]interface{}:
		out := make(map[string]interface{}, len(t))
		for k, vv := range t {
			out[fmt.Sprintf("%v", k)] = normaliseYAMLValue(vv)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(t))
		for i, vv := range t {
			out[i] = normaliseYAMLValue(vv)
		}
		return out
	default:
		return v
	}
}

