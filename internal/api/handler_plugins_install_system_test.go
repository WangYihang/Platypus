package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/link"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// install_system reads plugin.yaml + .wasm + .minisig + the system
// publisher.pub off the server's local data-dir and streams them
// into the agent install pipeline. Test contract:
//
//   1. Happy path: data-dir has the staged bundle → agent receives
//      the install op with the on-disk publisher.pub bytes, then
//      we emit a terminal PHASE_INSTALLED so the handler renders
//      status="installed".
//   2. WithSystemBundle was never called → 503.
//   3. data-dir exists but the requested plugin isn't staged → 404.
//   4. data-dir exists but publisher.pub is missing → 424.

const systemTestManifest = `api_version: 1
id: com.platypus.sys-test
name: System Test Plugin
version: 1.0.0
author: { name: Platypus Test, email: test@example.com }
license: Apache-2.0
description: synthetic system plugin for the install_system handler test
runtime:
  type: wasm
  entry: x.wasm
  abi: extism/1
rpc:
  - name: noop
    request: { proto: Empty }
    response: { proto: Empty }
capabilities:
  log: {}
resources:
  max_memory_mb: 16
  max_invocation_ms: 5000
signature:
  algo: minisign-ed25519
  key_id: 0000000000000000
  sig_file: x.wasm.minisig
`

// stageSystemBundle creates the on-disk layout the handler reads
// from: <dataDir>/system-plugins/{publisher.pub, <id>/<v>/{plugin.yaml,
// x.wasm, x.wasm.minisig}}. Returns the wasm bytes the test wants
// to assert agent-side.
func stageSystemBundle(t *testing.T, dataDir, pluginID, version string) (wasm []byte) {
	t.Helper()
	root := filepath.Join(dataDir, "system-plugins")
	pluginRoot := filepath.Join(root, pluginID, version)
	if err := os.MkdirAll(pluginRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "publisher.pub"),
		[]byte("untrusted comment: system\nFAKE-SYSTEM-PUBKEY"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginRoot, "plugin.yaml"),
		[]byte(systemTestManifest), 0o600); err != nil {
		t.Fatal(err)
	}
	wasm = []byte("FAKE-SYSTEM-WASM-BYTES")
	if err := os.WriteFile(filepath.Join(pluginRoot, "x.wasm"), wasm, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginRoot, "x.wasm.minisig"),
		[]byte("FAKE-SIG"), 0o600); err != nil {
		t.Fatal(err)
	}
	return wasm
}

func TestInstallFromSystem_HappyPath(t *testing.T) {
	dataDir := t.TempDir()
	wasm := stageSystemBundle(t, dataDir, "com.platypus.sys-test", "1.0.0")

	a := setupPluginsAgent(t, "agent-sys-happy",
		func(req *v2pb.PluginMgmtRequest, stream io.ReadWriteCloser) {
			install := req.GetInstall()
			if install == nil {
				t.Errorf("expected install op")
				return
			}
			if install.GetPluginId() != "com.platypus.sys-test" {
				t.Errorf("plugin_id = %q", install.GetPluginId())
			}
			if string(install.GetPublisherPubkey()) == "" {
				t.Errorf("publisher pubkey not threaded through (empty)")
			}
			// install_system inlines the wasm bytes into the same
			// chunk-pump the marketplace path uses, so the agent
			// drains 3 chunks (manifest, wasm, sig).
			// install_system inlines the wasm bytes into the same
			// 3-chunk pump the marketplace path uses (manifest +
			// wasm + signature, each frame carries last=true). We
			// drain all three and assert the wasm one matches
			// what stageSystemBundle wrote.
			seenWasm := false
			for i := 0; i < 3; i++ {
				var c v2pb.PluginInstallChunk
				if err := link.ReadFrame(stream, &c); err != nil {
					t.Errorf("read chunk %d: %v", i, err)
					return
				}
				if c.GetKind() == v2pb.PluginInstallChunk_KIND_WASM &&
					string(c.GetData()) == string(wasm) {
					seenWasm = true
				}
			}
			if !seenWasm {
				t.Errorf("wasm chunk with stageSystemBundle's bytes not seen on the stream")
			}
			_ = link.WriteFrame(stream, &v2pb.PluginInstallProgress{
				Phase: v2pb.PluginInstallProgress_PHASE_INSTALLED,
			})
		},
	)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewAgentPluginsHandler(a.svc).WithSystemBundle(os.DirFS(filepath.Join(dataDir, "system-plugins")))
	RegisterV1AgentPluginRoutes(r, h, a.fixture.RBAC)
	srv := httptest.NewServer(r)
	defer srv.Close()

	body := `{"plugin_id":"com.platypus.sys-test","version":"1.0.0","granted_capabilities":["log"]}`
	resp := a.authed(t, http.MethodPost, srv.URL, "/plugins/install_system", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body = %s", resp.StatusCode, b)
	}

	var got installResponse
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Status != "installed" {
		t.Errorf("status = %q, want installed", got.Status)
	}
}

// TestInstallFromSystem_NoBundleReturns503 — handler wired without
// WithSystemBundle. Endpoint must 503; the agent must never see an
// install op (we'd be passing zero bytes around).
func TestInstallFromSystem_NoBundleReturns503(t *testing.T) {
	a := setupPluginsAgent(t, "agent-sys-nobundle",
		func(_ *v2pb.PluginMgmtRequest, _ io.ReadWriteCloser) {
			t.Errorf("agent should not receive any mgmt op when system bundle is unwired")
		},
	)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewAgentPluginsHandler(a.svc) // no WithSystemBundle.
	RegisterV1AgentPluginRoutes(r, h, a.fixture.RBAC)
	srv := httptest.NewServer(r)
	defer srv.Close()

	body := `{"plugin_id":"com.platypus.sys-test","version":"1.0.0"}`
	resp := a.authed(t, http.MethodPost, srv.URL, "/plugins/install_system", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		b, _ := io.ReadAll(resp.Body)
		t.Errorf("status = %d, want 503; body = %s", resp.StatusCode, b)
	}
}

// TestInstallFromSystem_NotStagedReturns404 — operator asked for
// a plugin that isn't on this server's disk. Helps diagnose
// "publisher hasn't run yet" / "production seeder skipped this id"
// without bouncing the operator into the agent stream.
func TestInstallFromSystem_NotStagedReturns404(t *testing.T) {
	dataDir := t.TempDir()
	// Stage publisher.pub but no plugin dir.
	if err := os.MkdirAll(filepath.Join(dataDir, "system-plugins"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(dataDir, "system-plugins", "publisher.pub"),
		[]byte("anything"), 0o600,
	); err != nil {
		t.Fatal(err)
	}

	a := setupPluginsAgent(t, "agent-sys-notstaged",
		func(_ *v2pb.PluginMgmtRequest, _ io.ReadWriteCloser) {
			t.Errorf("agent should not receive any mgmt op when plugin is missing on disk")
		},
	)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewAgentPluginsHandler(a.svc).WithSystemBundle(os.DirFS(filepath.Join(dataDir, "system-plugins")))
	RegisterV1AgentPluginRoutes(r, h, a.fixture.RBAC)
	srv := httptest.NewServer(r)
	defer srv.Close()

	body := `{"plugin_id":"com.platypus.sys-missing","version":"1.0.0"}`
	resp := a.authed(t, http.MethodPost, srv.URL, "/plugins/install_system", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		b, _ := io.ReadAll(resp.Body)
		t.Errorf("status = %d, want 404; body = %s", resp.StatusCode, b)
	}
}

// TestInstallFromSystem_NoPublisherKeyReturns424 — the system
// bundle directory exists but its trust anchor (publisher.pub at
// the catalog root) is missing. Without it the agent would refuse
// every install with signature_mismatch, so we short-circuit.
func TestInstallFromSystem_NoPublisherKeyReturns424(t *testing.T) {
	dataDir := t.TempDir()
	// Stage the plugin but NOT the publisher.pub.
	pluginRoot := filepath.Join(dataDir, "system-plugins", "com.platypus.sys-test", "1.0.0")
	if err := os.MkdirAll(pluginRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginRoot, "plugin.yaml"),
		[]byte(systemTestManifest), 0o600); err != nil {
		t.Fatal(err)
	}

	a := setupPluginsAgent(t, "agent-sys-nopub",
		func(_ *v2pb.PluginMgmtRequest, _ io.ReadWriteCloser) {
			t.Errorf("agent should not receive any mgmt op when publisher.pub is missing")
		},
	)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewAgentPluginsHandler(a.svc).WithSystemBundle(os.DirFS(filepath.Join(dataDir, "system-plugins")))
	RegisterV1AgentPluginRoutes(r, h, a.fixture.RBAC)
	srv := httptest.NewServer(r)
	defer srv.Close()

	body := `{"plugin_id":"com.platypus.sys-test","version":"1.0.0"}`
	resp := a.authed(t, http.MethodPost, srv.URL, "/plugins/install_system", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusFailedDependency {
		b, _ := io.ReadAll(resp.Body)
		t.Errorf("status = %d, want 424; body = %s", resp.StatusCode, b)
	}
}
