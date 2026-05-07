package platypus

import (
	"encoding/json"
	"errors"

	"github.com/extism/go-pdk"
)

// configKey is the well-known Extism config map key the agent uses to
// surface the operator's resolved plugin config to the plugin. See
// internal/agent/plugin/loader.go for the host-side write. The key
// is a stable wire-style name so a future tooling that inspects the
// config map (e.g., `extism inspect`) can recognise it.
const configKey = "platypus_config"

// ErrNoConfig is returned by Config / ConfigInto when the host did
// not surface any plugin config to this plugin. Distinct from a
// JSON parse error so callers can branch cleanly: "operator
// didn't supply overrides — use my schema defaults" vs "operator
// supplied a config but it failed to parse, log + bail".
var ErrNoConfig = errors.New("platypus: no plugin config surfaced by host")

// Config returns the raw bytes of the operator's resolved plugin
// config blob, exactly as the agent received it on the wire (which
// is exactly what the operator authored, after secret refs were
// substituted server-side). The bytes are JSON shaped to match the
// plugin's manifest config.schema.
//
// Returns ErrNoConfig if the plugin declared no config block, or if
// the operator installed the plugin without overrides — both cases
// are common and not error conditions, so the caller should
// usually fall back to its built-in defaults rather than aborting.
//
// Most plugins should call ConfigInto(&myConfig) instead of
// reaching for the raw bytes; the typed variant is one line and
// catches schema drift via Go's encoding/json.
func Config() ([]byte, error) {
	raw, ok := pdk.GetConfig(configKey)
	if !ok || raw == "" {
		return nil, ErrNoConfig
	}
	return []byte(raw), nil
}

// ConfigInto unmarshals the operator's resolved plugin config into
// the supplied struct (or map). The struct's JSON tags must mirror
// the manifest's config.schema property names. Returns ErrNoConfig
// if no config was surfaced — callers typically do:
//
//	var cfg Config
//	if err := platypus.ConfigInto(&cfg); err != nil &&
//	    !errors.Is(err, platypus.ErrNoConfig) {
//	    return err
//	}
//	// cfg is either populated from the operator's input or
//	// left at zero values; treat zero values as "use defaults".
//
// JSON validation is the operator's burden at install time (the
// server validates against the schema before resolving secrets);
// this function trusts the bytes and only catches structural
// decode errors. Plugins that need stricter shape checks can grab
// Config() and run their own validator.
func ConfigInto(out any) error {
	raw, err := Config()
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, out)
}
