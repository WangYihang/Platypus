package plugin_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stagedSystemRoot is the in-repo path to the system-plugin tree the
// server binary embeds at compile time. Integration tests read .wasm
// + plugin.yaml directly out of it; a missing artefact here is now a
// CI failure (was a silent t.Skipf before D-tests).
//
// Refresh via `go run ./hack/stage_system_plugins` from the repo root
// after editing rust source under example/plugins/system/.
const stagedSystemRoot = "../../../internal/server/sysplugins/embedded/system-plugins"

// stagedWasmBytes reads the .wasm artefact for the given (plugin id,
// version, entry) tuple. Calls t.Fatalf with a stage-helper hint on
// missing files — silent skip on missing wasm hides a real bug
// (forgot to run the staging helper after a rust change).
func stagedWasmBytes(t *testing.T, pluginID, version, entry string) []byte {
	t.Helper()
	p := filepath.Join(stagedSystemRoot, pluginID, version, entry)
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("staged wasm %s missing (%v) — run `go run ./hack/stage_system_plugins` from the repo root", p, err)
	}
	return data
}

// stagedManifestBytes is the sibling for plugin.yaml.
func stagedManifestBytes(t *testing.T, pluginID, version string) []byte {
	t.Helper()
	p := filepath.Join(stagedSystemRoot, pluginID, version, "plugin.yaml")
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("staged manifest %s missing (%v)", p, err)
	}
	return data
}

// rewriteManifestKeyID swaps the manifest's signature.key_id field for
// the supplied hex string, preserving comments and surrounding
// formatting. Integration tests sign with a fresh per-test keypair;
// the staged manifest's key_id is whatever hack/stage_system_plugins
// minted at build time, so the keys won't match without rewriting.
//
// String-level rewrite (rather than yaml round-trip) so we don't lose
// comments or blank lines that yaml.Marshal would drop.
func rewriteManifestKeyID(src, keyID string) string {
	const marker = "key_id:"
	idx := strings.Index(src, marker)
	if idx < 0 {
		return src
	}
	tail := src[idx+len(marker):]
	nl := strings.IndexByte(tail, '\n')
	if nl < 0 {
		return src // unterminated manifest? leave as-is
	}
	return src[:idx+len(marker)] + " " + keyID + tail[nl:]
}
