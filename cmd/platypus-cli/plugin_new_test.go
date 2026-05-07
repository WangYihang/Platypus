package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	agentplugin "github.com/WangYihang/Platypus/internal/agent/plugin"
	coreplugin "github.com/WangYihang/Platypus/internal/core/plugin"
)

// scaffoldArgs centralises the non-interactive flag set used by the
// happy-path tests so the individual cases focus on what they're
// pinning rather than re-spelling defaults.
func scaffoldArgs(dir, lang, id string) pluginNewCmd {
	return pluginNewCmd{
		Dir:          dir,
		Lang:         lang,
		ID:           id,
		Name:         "Smoke",
		Version:      "1.0.0",
		AuthorName:   "Tester",
		AuthorEmail:  "test@example.com",
		License:      "Apache-2.0",
		Description:  "Smoke-test plugin produced by the scaffolder TDD harness.",
		Capabilities: []string{"log", "sysinfo"},
		WithConfig:   true,
		Force:        false,
	}
}

func runScaffold(t *testing.T, c pluginNewCmd) {
	t.Helper()
	if err := c.Run(&runContext{Context: context.Background()}); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

// TestPluginNew_Rust_GeneratesValidProject: the scaffolder produces
// a Rust project whose plugin.yaml round-trips through the actual
// manifest validator. If the validator accepts our scaffold, every
// author's first attempt will too — no surprise rejections after
// they've copy-pasted the build commands.
func TestPluginNew_Rust_GeneratesValidProject(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "smoke")
	runScaffold(t, scaffoldArgs(dir, "rust", "com.example.smoke"))

	for _, want := range []string{
		"plugin.yaml", "Cargo.toml", "src/lib.rs", "README.md", ".gitignore",
	} {
		if _, err := os.Stat(filepath.Join(dir, want)); err != nil {
			t.Fatalf("missing %s: %v", want, err)
		}
	}

	manifestBytes, err := os.ReadFile(filepath.Join(dir, "plugin.yaml"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if _, err := agentplugin.ParseManifest(manifestBytes); err != nil {
		t.Fatalf("scaffolded manifest fails validator: %v", err)
	}
}

// TestPluginNew_Go_GeneratesValidProject: same shape against the Go
// template. Go projects use TinyGo + sdk/go/platypus-plugin, which
// the README walks the author through.
func TestPluginNew_Go_GeneratesValidProject(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "smoke")
	runScaffold(t, scaffoldArgs(dir, "go", "com.example.smoke"))

	for _, want := range []string{
		"plugin.yaml", "go.mod", "main.go", "README.md", ".gitignore",
	} {
		if _, err := os.Stat(filepath.Join(dir, want)); err != nil {
			t.Fatalf("missing %s: %v", want, err)
		}
	}
	manifestBytes, _ := os.ReadFile(filepath.Join(dir, "plugin.yaml"))
	if _, err := agentplugin.ParseManifest(manifestBytes); err != nil {
		t.Fatalf("scaffolded manifest fails validator: %v", err)
	}
}

// TestPluginNew_WithConfig_ProducesValidSchema: the canonical
// `greeting + shout` config block must validate against itself —
// passing the schema's own defaults to the validator is the
// strictest test that the rendered YAML parses as JSON Schema.
// Cross-checks against the same gojsonschema-backed validator the
// install path uses.
func TestPluginNew_WithConfig_ProducesValidSchema(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "smoke")
	args := scaffoldArgs(dir, "rust", "com.example.smoke")
	args.WithConfig = true
	runScaffold(t, args)

	manifestBytes, _ := os.ReadFile(filepath.Join(dir, "plugin.yaml"))
	manifest, err := agentplugin.ParseManifest(manifestBytes)
	if err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	if manifest.Config.Schema.IsZero() {
		t.Fatalf("with-config requested but schema absent in manifest")
	}
	// The defaults block (greeting + shout) re-rendered as a
	// resolved config must validate cleanly against the schema.
	defaults, err := json.Marshal(map[string]any{
		"greeting": "Hello",
		"shout":    false,
	})
	if err != nil {
		t.Fatalf("marshal defaults: %v", err)
	}
	if err := coreplugin.ValidateConfig(manifest, defaults, manifest.Config.SchemaVersion); err != nil {
		t.Fatalf("scaffolded config defaults fail validator: %v", err)
	}
}

// TestPluginNew_WithoutConfig_OmitsConfigBlock: when the author
// opts out of config, the generated manifest carries no `config:`
// block at all (the validator treats schema-absent as "no config
// allowed", which is the legacy posture every existing plugin
// matches).
func TestPluginNew_WithoutConfig_OmitsConfigBlock(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "smoke")
	args := scaffoldArgs(dir, "rust", "com.example.smoke")
	args.WithConfig = false
	runScaffold(t, args)

	manifestBytes, _ := os.ReadFile(filepath.Join(dir, "plugin.yaml"))
	manifest, err := agentplugin.ParseManifest(manifestBytes)
	if err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	if !manifest.Config.Schema.IsZero() {
		t.Fatalf("--with-config=false but schema is present: %+v",
			manifest.Config)
	}
}

// TestPluginNew_WithCapabilities_RendersAllowlists: every
// non-implicit capability family the scaffolder lists must produce
// a YAML allowlist with at least one placeholder entry, AND the
// resulting manifest must parse. Pinning per-family means a future
// capability addition that forgets to register a renderer fails
// fast.
//
// `log` is the one exception — it's implicit (every plugin gets
// it without a manifest entry), so the YAML check is skipped for
// it but the manifest still has to parse.
func TestPluginNew_WithCapabilities_RendersAllowlists(t *testing.T) {
	cases := []struct {
		family    string
		expectKey string // empty = skip key check (implicit cap)
	}{
		{"log", ""}, // implicit — no manifest entry expected
		{"kv", "kv"},
		{"sysinfo", "sysinfo"},
		{"exec", "exec"},
		{"fs.read", "fs.read"},
		{"fs.write", "fs.write"},
		{"net.http", "net.http"},
		{"process", "process"},
		{"net.dial", "net.dial"},
		{"net.listen", "net.listen"},
	}
	for _, c := range cases {
		t.Run(c.family, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "smoke")
			args := scaffoldArgs(dir, "rust", "com.example.smoke")
			args.Capabilities = []string{c.family}
			runScaffold(t, args)
			yaml, err := os.ReadFile(filepath.Join(dir, "plugin.yaml"))
			if err != nil {
				t.Fatalf("read manifest: %v", err)
			}
			if c.expectKey != "" && !strings.Contains(string(yaml), c.expectKey) {
				t.Fatalf("capability %q missing from manifest:\n%s",
					c.family, string(yaml))
			}
			if _, err := agentplugin.ParseManifest(yaml); err != nil {
				t.Fatalf("manifest with %q fails validator: %v",
					c.family, err)
			}
		})
	}
}

// TestPluginNew_RefusesOverwrite: writing into a non-empty directory
// without --force is a hard refusal. The scaffolder is meant to
// bootstrap fresh projects; clobbering an in-flight one is the
// kind of error that ruins an author's morning.
func TestPluginNew_RefusesOverwrite(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "existing.txt"),
		[]byte("hello"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	args := scaffoldArgs(dir, "rust", "com.example.smoke")
	if err := args.Run(&runContext{Context: context.Background()}); err == nil {
		t.Fatalf("expected refusal on non-empty dir, got nil")
	}
	// --force lets the author opt in.
	args.Force = true
	if err := args.Run(&runContext{Context: context.Background()}); err != nil {
		t.Fatalf("--force should succeed: %v", err)
	}
	// The pre-existing file is preserved (we add files, not nuke).
	if _, err := os.Stat(filepath.Join(dir, "existing.txt")); err != nil {
		t.Fatalf("--force should not delete pre-existing files: %v", err)
	}
}

// TestPluginNew_PluginIDValidation: a malformed id is rejected
// before any file is written. Pinning this stops a future "we'll
// just fix the id at write time" shortcut from silently producing
// invalid manifests.
func TestPluginNew_PluginIDValidation(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "smoke")
	cases := []string{
		"",                  // empty
		"NotLowercase",      // uppercase
		"missingdotsegment", // no dot
		".leadingdot.x",     // leading dot
		"trailing.dot.",     // trailing dot
		"two..dots.x",       // empty segment
	}
	for _, bad := range cases {
		t.Run(bad, func(t *testing.T) {
			args := scaffoldArgs(dir, "rust", bad)
			if err := args.Run(&runContext{Context: context.Background()}); err == nil {
				t.Fatalf("expected failure for id=%q", bad)
			}
			if _, err := os.Stat(dir); err == nil {
				t.Fatalf("dir should not exist after failed validation: id=%q", bad)
			}
		})
	}
}

// TestPluginNew_CapabilityRegistries_CoverAllFamilies pins the
// init-time exhaustiveness check: every CapabilityID the agent
// recognises must have an entry in every renderer registry. The
// init() in plugin_new_caps.go panics on startup if any are
// missing — this test asserts the contract by inspecting the
// registries directly, so a future capability addition that's
// half-wired surfaces a clear failure here in CI rather than at
// `platypus-cli` startup time on someone's workstation.
func TestPluginNew_CapabilityRegistries_CoverAllFamilies(t *testing.T) {
	registries := map[string]map[agentplugin.CapabilityID]string{
		"capYAMLEntries":  capYAMLEntries,
		"capHintsRust":    capHintsRust,
		"capHintsGo":      capHintsGo,
		"capDescriptions": capDescriptions,
	}
	for name, m := range registries {
		for _, c := range agentplugin.AllCapabilityIDs {
			if _, ok := m[c]; !ok {
				t.Errorf("registry %s missing capability %q",
					name, c)
			}
		}
	}
}

// TestPluginNew_RejectsUnknownCapability: a typo or stale capability
// at the CLI boundary fails before any file is written, with a
// helpful "valid: ..." list. The typed parseCapabilities is the
// load-bearing piece — without it a typo would silently produce
// a manifest with an invented family name that the validator
// later rejects mysteriously.
func TestPluginNew_RejectsUnknownCapability(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "smoke")
	args := scaffoldArgs(dir, "rust", "com.example.smoke")
	args.Capabilities = []string{"log", "fs.write-but-typoed"}
	err := args.Run(&runContext{Context: context.Background()})
	if err == nil {
		t.Fatalf("expected unknown-capability rejection")
	}
	if !strings.Contains(err.Error(), "fs.write-but-typoed") {
		t.Fatalf("err should name the bad capability: %v", err)
	}
	if _, statErr := os.Stat(dir); statErr == nil {
		t.Fatalf("dir should not exist after capability rejection")
	}
}

// TestPluginNew_RejectsUnknownLang pins the typed-Lang behaviour:
// a wire string that isn't one of the known languages fails at
// parse time with the valid set listed.
func TestPluginNew_RejectsUnknownLang(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "smoke")
	args := scaffoldArgs(dir, "python", "com.example.smoke")
	err := args.Run(&runContext{Context: context.Background()})
	if err == nil {
		t.Fatalf("expected unknown-lang rejection")
	}
	if !strings.Contains(err.Error(), "python") ||
		!strings.Contains(err.Error(), "valid:") {
		t.Fatalf("err should name the bad lang AND list valid: %v", err)
	}
}

// TestPluginNew_DefaultsDirToPluginID: when --dir is empty, the
// scaffolder picks a sane default (the rightmost segment of the
// reverse-DNS id, e.g. "com.example.my-plugin" → "./my-plugin").
// Mirrors `cargo new <name>` ergonomics.
func TestPluginNew_DefaultsDirToPluginID(t *testing.T) {
	prev, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(prev) })
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	args := scaffoldArgs("", "rust", "com.example.my-plugin")
	runScaffold(t, args)
	if _, err := os.Stat(filepath.Join(tmp, "my-plugin", "plugin.yaml")); err != nil {
		t.Fatalf("default dir not used: %v", err)
	}
}
