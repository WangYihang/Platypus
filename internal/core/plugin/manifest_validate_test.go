package plugin

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	agentplugin "github.com/WangYihang/Platypus/internal/agent/plugin"
)

// makeManifest is a tiny test helper that parses a YAML snippet into
// a Manifest. Tests construct the snippet inline to exercise the
// validator against representative schemas.
func makeManifest(t *testing.T, src string) *agentplugin.Manifest {
	t.Helper()
	var m agentplugin.Manifest
	if err := yaml.Unmarshal([]byte(src), &m); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	return &m
}

func TestValidateConfig_NoSchemaAcceptsEmpty(t *testing.T) {
	// Plugins that don't declare a config schema accept an empty
	// config — this is the legacy path most plugins live on today.
	m := makeManifest(t, `
api_version: 1
id: com.example.no-config
`)
	if err := ValidateConfig(m, nil, 0); err != nil {
		t.Fatalf("nil config: %v", err)
	}
	if err := ValidateConfig(m, []byte("{}"), 0); err != nil {
		t.Fatalf("empty config: %v", err)
	}
}

func TestValidateConfig_NoSchemaRejectsNonEmpty(t *testing.T) {
	// If a plugin doesn't declare a schema, we refuse non-empty
	// configs — they would be silently dropped on the agent side
	// and are usually a copy-paste / typo error.
	m := makeManifest(t, `
api_version: 1
id: com.example.no-config
`)
	err := ValidateConfig(m, []byte(`{"extra":"value"}`), 0)
	if err == nil {
		t.Fatalf("non-empty config without schema should fail")
	}
	if !strings.Contains(err.Error(), "no config schema") {
		t.Fatalf("err text = %q, want mention of missing schema", err.Error())
	}
}

func TestValidateConfig_RequiresFieldsAndValidatesTypes(t *testing.T) {
	m := makeManifest(t, `
api_version: 1
id: com.example.syslog
config:
  schema_version: 1
  schema:
    type: object
    required: [destination, port]
    properties:
      destination:
        type: string
        format: uri
      port:
        type: integer
        minimum: 1
        maximum: 65535
      tls:
        type: boolean
        default: true
`)
	// Missing destination + port: validator names both fields.
	err := ValidateConfig(m, []byte(`{}`), 1)
	if err == nil {
		t.Fatalf("empty config should fail required check")
	}
	if !strings.Contains(err.Error(), "destination") {
		t.Fatalf("err missing 'destination': %v", err)
	}

	// Port out of range.
	err = ValidateConfig(m, []byte(`{"destination":"udp://x","port":99999}`), 1)
	if err == nil {
		t.Fatalf("out-of-range port should fail")
	}

	// Happy path.
	if err := ValidateConfig(m, []byte(`{"destination":"udp://10.0.0.1:514","port":514}`), 1); err != nil {
		t.Fatalf("valid config: %v", err)
	}
}

func TestValidateConfig_SchemaVersionMismatch(t *testing.T) {
	m := makeManifest(t, `
api_version: 1
id: com.example.versioned
config:
  schema_version: 2
  schema:
    type: object
`)
	err := ValidateConfig(m, []byte(`{}`), 1)
	if err == nil {
		t.Fatalf("schema_version mismatch should fail")
	}
	if !strings.Contains(err.Error(), "schema_version") {
		t.Fatalf("err text = %q, want mention of schema_version", err.Error())
	}
}
