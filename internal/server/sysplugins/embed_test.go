package sysplugins

import (
	"io/fs"
	"strings"
	"testing"
)

// TestPrebuiltFSContainsExpectedShape asserts the staged tree
// embedded by go:embed has the publisher.pub trust anchor and at
// least the mandatory sys-info bundle. If this fails the binary
// either ships an empty system-plugins set (operator dropped a binary
// on a fresh box → no plugins → broken UI) or hack/stage_system_plugins
// produced a malformed tree.
func TestPrebuiltFSContainsExpectedShape(t *testing.T) {
	fsys := PrebuiltFS()

	pub, err := fs.ReadFile(fsys, "publisher.pub")
	if err != nil {
		t.Fatalf("publisher.pub: %v", err)
	}
	if !strings.Contains(string(pub), "untrusted comment:") {
		t.Errorf("publisher.pub missing minisign comment header")
	}

	// sys-info is the one mandatory plugin per
	// internal/api/plugin_sync.go:mandatorySystemPlugins. Its
	// absence is a release blocker.
	const sysInfoVersion = "2.0.0"
	manifestPath := "com.platypus.sys-info/" + sysInfoVersion + "/plugin.yaml"
	manifest, err := fs.ReadFile(fsys, manifestPath)
	if err != nil {
		t.Fatalf("read %s: %v", manifestPath, err)
	}
	if !strings.Contains(string(manifest), "id: com.platypus.sys-info") {
		t.Errorf("manifest missing expected id: %s", manifest)
	}

	// The wasm + sig must be present too, otherwise reconcile would
	// hand the agent a manifest with no payload.
	wasmPath := "com.platypus.sys-info/" + sysInfoVersion + "/sys_info_plugin.wasm"
	wasm, err := fs.ReadFile(fsys, wasmPath)
	if err != nil {
		t.Fatalf("read %s: %v", wasmPath, err)
	}
	if len(wasm) < 1024 {
		t.Errorf("wasm suspiciously small: %d bytes", len(wasm))
	}
	if _, err := fs.ReadFile(fsys, wasmPath+".minisig"); err != nil {
		t.Fatalf("read sig: %v", err)
	}
}

// TestResolvePrefersDiskWhenPopulated asserts the resolver picks the
// operator's disk override when publisher.pub is present at the
// expected path. We don't stage a full bundle — Resolve only probes
// for the trust-anchor file.
func TestResolvePrefersDiskWhenPopulated(t *testing.T) {
	dir := t.TempDir()
	// Empty mkdir doesn't count — Resolve falls back to embed.
	if got := Resolve(dir); got == nil {
		t.Fatal("Resolve returned nil")
	}
	// Now seed publisher.pub. Resolve should switch to the disk fs.
	{
		// Need to avoid filepath here for windows? The resolver
		// uses filepath.Join internally, this test is unix-shaped
		// but t.TempDir produces os-native paths and filepath
		// handles them.
		if err := writeFile(dir+"/system-plugins/publisher.pub", "stub"); err != nil {
			t.Fatalf("seed publisher.pub: %v", err)
		}
	}
	got := Resolve(dir)
	if got == nil {
		t.Fatal("Resolve returned nil with populated dir")
	}
	pub, err := fs.ReadFile(got, "publisher.pub")
	if err != nil {
		t.Fatalf("ReadFile from disk override: %v", err)
	}
	if string(pub) != "stub" {
		t.Errorf("disk override not honoured; got %q", pub)
	}
}
