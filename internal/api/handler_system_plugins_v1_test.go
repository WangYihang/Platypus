package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/api"
)

// stageManifest writes a minimal but well-formed plugin.yaml under
// root/<id>/<version>/ so the handler's enumeration sees it as a
// valid system plugin. The manifest validator requires at least
// one rpc OR streams entry; the test stages a no-op rpc when the
// caller doesn't pass any streams to satisfy the rule.
func stageManifest(t *testing.T, root, id, version, name string, capabilities, streams string) {
	t.Helper()
	dir := filepath.Join(root, id, version)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	rpcSection := ""
	if streams == "" {
		rpcSection = "rpc:\n  - name: noop\n    request: { proto: Empty }\n    response: { proto: Empty }\n"
	}
	manifest := `api_version: 1
id: ` + id + `
name: ` + name + `
version: ` + version + `
author: { name: Platypus Test, email: test@example.com }
license: Apache-2.0
description: synthetic plugin for the system-plugins endpoint test
runtime:
  type: wasm
  entry: x.wasm
  abi: extism/1
` + rpcSection + capabilities + streams + `
resources:
  max_memory_mb: 16
  max_invocation_ms: 5000
signature:
  algo: minisign-ed25519
  key_id: 0000000000000000
  sig_file: x.wasm.minisig
`
	if err := os.WriteFile(filepath.Join(dir, "plugin.yaml"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
}

// newSystemPluginsTestRouter mounts the handler's List directly,
// bypassing the RBAC middleware. The handler logic is purely
// filesystem read + manifest parse — auth coverage lives in the
// dedicated rbac tests, so duplicating session-bearer setup here
// would just slow down the table without testing anything new.
func newSystemPluginsTestRouter(t *testing.T, dataDir string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := api.NewSystemPluginsHandler(os.DirFS(filepath.Join(dataDir, "system-plugins")))
	r.GET("/api/v1/system-plugins", h.List)
	return r
}

func doGetSystemPlugins(t *testing.T, r *gin.Engine, path string) (*httptest.ResponseRecorder, []byte) {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	r.ServeHTTP(w, req)
	return w, w.Body.Bytes()
}

// TestSystemPlugins_EmptyDirReturnsEmptyList ensures the missing /
// empty system-plugins/ directory produces a clean 200 with an
// empty list — the wizard renders an empty-state hint, not an
// error toast. A 5xx here would push every fresh-server enroll
// flow into the failure branch.
func TestSystemPlugins_EmptyDirReturnsEmptyList(t *testing.T) {
	dataDir := t.TempDir()
	r := newSystemPluginsTestRouter(t, dataDir)
	w, body := doGetSystemPlugins(t, r, "/api/v1/system-plugins")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", w.Code)
	}
	var resp struct {
		Plugins []json.RawMessage `json:"plugins"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("parse: %v\nbody=%s", err, body)
	}
	if len(resp.Plugins) != 0 {
		t.Fatalf("Plugins = %v; want empty", resp.Plugins)
	}
}

// TestSystemPlugins_EnumeratesStagedBundles populates a fake
// publisher output (root/<id>/<version>/plugin.yaml) and verifies
// the handler returns the parsed catalog. This is the contract the
// dev publisher relies on — any change in plugin.yaml that breaks
// ParseManifest would surface here.
func TestSystemPlugins_EnumeratesStagedBundles(t *testing.T) {
	dataDir := t.TempDir()
	root := filepath.Join(dataDir, "system-plugins")

	stageManifest(t, root, "com.platypus.sys-info", "2.0.0", "System Info",
		"capabilities:\n  sysinfo: true\n", "")
	stageManifest(t, root, "com.platypus.sys-file-read", "1.0.0", "File Read",
		"capabilities:\n  fs.read:\n    paths: [\"/\"]\n",
		"streams:\n  - name: read\n    stream_type: STREAM_TYPE_FILE_READ\n    host_handler: wasm:read\n")

	r := newSystemPluginsTestRouter(t, dataDir)
	w, body := doGetSystemPlugins(t, r, "/api/v1/system-plugins")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200; body=%s", w.Code, body)
	}
	var resp struct {
		Plugins []struct {
			ID           string   `json:"id"`
			Name         string   `json:"name"`
			Version      string   `json:"version"`
			Capabilities []string `json:"capabilities"`
			Streams      []string `json:"streams"`
		} `json:"plugins"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("parse: %v\nbody=%s", err, body)
	}
	if len(resp.Plugins) != 2 {
		t.Fatalf("len = %d; want 2; got %+v", len(resp.Plugins), resp.Plugins)
	}
	// Sort is id-asc so sys-file-read appears first.
	if resp.Plugins[0].ID != "com.platypus.sys-file-read" {
		t.Fatalf("[0].ID = %q; want sys-file-read", resp.Plugins[0].ID)
	}
	if len(resp.Plugins[0].Streams) != 1 || resp.Plugins[0].Streams[0] != "STREAM_TYPE_FILE_READ" {
		t.Fatalf("[0].Streams = %v; want [STREAM_TYPE_FILE_READ]", resp.Plugins[0].Streams)
	}
	if resp.Plugins[1].ID != "com.platypus.sys-info" {
		t.Fatalf("[1].ID = %q; want sys-info", resp.Plugins[1].ID)
	}
	// sys-info declares no streams; field should be nil/empty.
	if len(resp.Plugins[1].Streams) != 0 {
		t.Fatalf("[1].Streams = %v; want empty", resp.Plugins[1].Streams)
	}
}

// TestSystemPlugins_SkipsCorruptManifest ensures a single broken
// plugin.yaml doesn't blank the entire catalogue — the operator
// should still see the healthy plugins, with the broken one
// silently excluded. (The publisher logs the parse failure on
// the server's stderr; that path is the right surface for the bug.)
func TestSystemPlugins_SkipsCorruptManifest(t *testing.T) {
	dataDir := t.TempDir()
	root := filepath.Join(dataDir, "system-plugins")

	stageManifest(t, root, "com.platypus.sys-info", "2.0.0", "System Info",
		"capabilities:\n  sysinfo: true\n", "")

	// Corrupt: not valid YAML.
	corruptDir := filepath.Join(root, "com.platypus.broken", "1.0.0")
	if err := os.MkdirAll(corruptDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(corruptDir, "plugin.yaml"), []byte("not yaml: ["), 0o600); err != nil {
		t.Fatal(err)
	}

	r := newSystemPluginsTestRouter(t, dataDir)
	w, body := doGetSystemPlugins(t, r, "/api/v1/system-plugins")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", w.Code, body)
	}
	var resp struct {
		Plugins []struct {
			ID string `json:"id"`
		} `json:"plugins"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Plugins) != 1 || resp.Plugins[0].ID != "com.platypus.sys-info" {
		t.Fatalf("Plugins = %+v; want only sys-info", resp.Plugins)
	}
}
