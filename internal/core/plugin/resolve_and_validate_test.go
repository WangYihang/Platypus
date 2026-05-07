package plugin

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	agentplugin "github.com/WangYihang/Platypus/internal/agent/plugin"
)

// TestResolveAndValidate_ResolvesThenValidates: the happy path —
// a config with a SecretRef gets resolved against the secret store,
// then the resolved value validates against the manifest's schema.
// Pinning this composition keeps the order stable: any future
// change that flips it would let invalid plaintext slip through
// (or reject a config because the JSON Schema sees the
// unresolved SecretRef shape).
func TestResolveAndValidate_ResolvesThenValidates(t *testing.T) {
	manifest := makeManifest(t, `
api_version: 1
id: com.example.syslog
config:
  schema_version: 1
  schema:
    type: object
    required: [destination, auth_token]
    properties:
      destination: {type: string, format: uri}
      auth_token:  {type: string, minLength: 8}
`)
	reader := &stubSecretReader{
		values: map[string][]byte{"sec_dd": []byte("dd-token-supersecret")},
	}
	overrides := json.RawMessage(`{
		"destination": "udp://10.0.0.1:514",
		"auth_token": {"$secret":"sec_dd"}
	}`)

	resolved, err := ResolveAndValidate(context.Background(), reader, manifest, overrides, 1)
	if err != nil {
		t.Fatalf("ResolveAndValidate: %v", err)
	}
	// Resolved blob carries the substituted plaintext.
	want := json.RawMessage(`{
		"destination": "udp://10.0.0.1:514",
		"auth_token": "dd-token-supersecret"
	}`)
	if !jsonEqual(resolved, want) {
		t.Fatalf("resolved drift:\n  got:  %s\n  want: %s", resolved, want)
	}
}

// TestResolveAndValidate_ValidationCatchesPlaintextValue: validation
// runs on the RESOLVED config, not the saved one. So a secret whose
// resolved value violates the schema (e.g., too short) gets rejected
// at install time. This is the "config from rotation went through
// but the new value doesn't fit the schema" scenario; the operator
// sees a clear error rather than the plugin booting with garbage.
func TestResolveAndValidate_ValidationCatchesResolvedValue(t *testing.T) {
	manifest := makeManifest(t, `
api_version: 1
id: com.example.api
config:
  schema_version: 1
  schema:
    type: object
    required: [token]
    properties:
      token: {type: string, minLength: 16}
`)
	reader := &stubSecretReader{
		values: map[string][]byte{"sec_short": []byte("too-short")},
	}
	overrides := json.RawMessage(`{"token":{"$secret":"sec_short"}}`)
	_, err := ResolveAndValidate(context.Background(), reader, manifest, overrides, 1)
	if err == nil {
		t.Fatalf("short token should fail validation")
	}
	if !strings.Contains(err.Error(), "minLength") &&
		!strings.Contains(err.Error(), "min length") {
		t.Fatalf("err = %q, want minLength constraint mention", err.Error())
	}
}

// TestResolveAndValidate_MissingSecretSurfacedFromResolver: an
// error from the resolver (missing secret) propagates with the JSON
// Pointer naming the field. Tests the wrapping/propagation rather
// than the underlying behaviour, which the resolver tests cover.
func TestResolveAndValidate_MissingSecretSurfacedFromResolver(t *testing.T) {
	manifest := makeManifest(t, `
api_version: 1
id: com.example.x
config:
  schema_version: 1
  schema:
    type: object
`)
	reader := &stubSecretReader{missing: map[string]bool{"sec_gone": true}}
	overrides := json.RawMessage(`{"k":{"$secret":"sec_gone"}}`)
	_, err := ResolveAndValidate(context.Background(), reader, manifest, overrides, 1)
	errorContains(t, err, "/k")
	errorContains(t, err, "sec_gone")
}

// TestResolveAndValidate_NilManifestRejected: a defensive check —
// we should never be asked to validate against nothing. Clearer than
// a nil-pointer panic at install time when the manifest fetch
// silently failed upstream.
func TestResolveAndValidate_NilManifestRejected(t *testing.T) {
	_, err := ResolveAndValidate(
		context.Background(), &stubSecretReader{}, nil, nil, 0,
	)
	errorContains(t, err, "manifest")
}

// TestResolveAndValidate_NoConfigSchemaAcceptsEmpty: composes the
// "no schema declared, empty overrides" leg of the validator with
// the resolver's no-op path — should pass cleanly.
func TestResolveAndValidate_NoConfigSchemaAcceptsEmpty(t *testing.T) {
	manifest := makeManifest(t, `
api_version: 1
id: com.example.legacy
`)
	resolved, err := ResolveAndValidate(
		context.Background(), &stubSecretReader{}, manifest, nil, 0,
	)
	if err != nil {
		t.Fatalf("nil overrides + no schema: %v", err)
	}
	if len(resolved) != 0 {
		t.Fatalf("expected empty resolved, got %s", resolved)
	}
}

// nodeFromYAML is a tiny helper used only by tests that need to
// construct a yaml.Node literal for the manifest. Keeps the manifest
// fixtures readable.
func nodeFromYAML(t *testing.T, src string) yaml.Node {
	t.Helper()
	var n yaml.Node
	if err := yaml.Unmarshal([]byte(src), &n); err != nil {
		t.Fatalf("yaml: %v", err)
	}
	return n
}

// reference the helper to silence unused warnings if a future test
// drops it; cheap insurance for the test file's stability.
var _ = nodeFromYAML
var _ = agentplugin.Manifest{}
