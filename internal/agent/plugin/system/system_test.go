package system

import (
	"context"
	"errors"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/WangYihang/Platypus/internal/agent/plugin"
)

// noopWasm is the minimal valid WebAssembly module exporting one
// no-op function. Same fixture the agent's integration test uses;
// duplicated here to keep the system subpackage self-contained.
var noopWasm = []byte{
	0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00,
	0x01, 0x04, 0x01, 0x60, 0x00, 0x00,
	0x03, 0x02, 0x01, 0x00,
	0x07, 0x08, 0x01, 0x04, 'n', 'o', 'o', 'p', 0x00, 0x00,
	0x0a, 0x04, 0x01, 0x02, 0x00, 0x0b,
}

const noopManifestTmpl = `
api_version: 1
id: PLUGIN_ID
name: Sys Noop
version: VERSION
author: { name: Platypus, email: dev@example.com }
license: Apache-2.0
runtime:
  type: wasm
  entry: noop.wasm
  abi: extism/1
rpc:
  - name: noop
    request:  { proto: Empty }
    response: { proto: Empty }
resources:
  max_memory_mb: 16
  max_invocation_ms: 5000
signature:
  algo: minisign-ed25519
  key_id: PLACEHOLDER
  sig_file: noop.wasm.minisig
`

// buildEmbeddedFS produces an fstest.MapFS populated with a single
// signed system plugin. Returns the FS plus the keypair so the test
// can introduce tampered variants.
func buildEmbeddedFS(t *testing.T, pluginID, version string) (fstest.MapFS, plugin.SecretKey) {
	t.Helper()
	sk, pk, err := plugin.GenerateKeyPair()
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	manifest := strings.NewReplacer(
		"PLUGIN_ID", pluginID,
		"VERSION", version,
		"PLACEHOLDER", plugin.HumanKeyID(pk),
	).Replace(noopManifestTmpl)
	sig, err := plugin.Sign(sk, noopWasm, plugin.DefaultTrustedComment("noop.wasm"))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	fs := fstest.MapFS{
		"publisher.pub":                                       {Data: []byte(plugin.EncodePublicKey(pk, ""))},
		pluginID + "/" + version + "/plugin.yaml":             {Data: []byte(manifest)},
		pluginID + "/" + version + "/noop.wasm":               {Data: noopWasm},
		pluginID + "/" + version + "/noop.wasm.minisig":       {Data: []byte(plugin.EncodeSignature(sig))},
	}
	return fs, sk
}

// freshRegistry returns a Registry rooted at a per-test temp dir.
func freshRegistry(t *testing.T) *plugin.Registry {
	t.Helper()
	root := t.TempDir()
	reg, err := plugin.New(plugin.Options{Paths: plugin.NewPaths(root)})
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	return reg
}

func TestSystem_FreshInstall(t *testing.T) {
	embFS, _ := buildEmbeddedFS(t, "com.platypus.sys-noop", "1.0.0")
	reg := freshRegistry(t)
	defer reg.Close(context.Background())

	res := EnsureInstalled(context.Background(), reg, embFS, EnsureOptions{AllowAll: true})
	if res.SetupError != nil {
		t.Fatalf("setup err: %v", res.SetupError)
	}
	if len(res.Installed) != 1 || res.Installed[0].ID != "com.platypus.sys-noop" {
		t.Fatalf("installed = %+v", res.Installed)
	}
	if len(res.Skipped) != 0 || len(res.Failed) != 0 {
		t.Errorf("skipped=%v failed=%v", res.Skipped, res.Failed)
	}

	// Plugin is now installed and marked System.
	if !reg.HasInstalledVersion("com.platypus.sys-noop", "1.0.0") {
		t.Errorf("expected plugin to be in catalog")
	}
}

func TestSystem_AlreadyInstalledSkipsOnSecondCall(t *testing.T) {
	embFS, _ := buildEmbeddedFS(t, "com.platypus.sys-noop", "1.0.0")
	reg := freshRegistry(t)
	defer reg.Close(context.Background())

	if r := EnsureInstalled(context.Background(), reg, embFS, EnsureOptions{AllowAll: true}); len(r.Installed) != 1 {
		t.Fatalf("first call should install: %+v", r)
	}
	r := EnsureInstalled(context.Background(), reg, embFS, EnsureOptions{AllowAll: true})
	if len(r.Skipped) != 1 || len(r.Installed) != 0 {
		t.Errorf("second call should skip: %+v", r)
	}
}

func TestSystem_TamperedWasmFails(t *testing.T) {
	embFS, _ := buildEmbeddedFS(t, "com.platypus.sys-noop", "1.0.0")
	// Replace the wasm bytes after signing. The signature was over the
	// original; this corrupted body should fail VerifyWasm.
	embFS["com.platypus.sys-noop/1.0.0/noop.wasm"] = &fstest.MapFile{Data: []byte("garbage")}

	reg := freshRegistry(t)
	defer reg.Close(context.Background())

	res := EnsureInstalled(context.Background(), reg, embFS, EnsureOptions{AllowAll: true})
	if len(res.Failed) != 1 {
		t.Fatalf("expected 1 failure, got %+v", res)
	}
	if !strings.Contains(res.Failed[0].Err.Error(), "signature_mismatch") {
		t.Errorf("err = %v, want signature_mismatch", res.Failed[0].Err)
	}
	if reg.HasInstalledVersion("com.platypus.sys-noop", "1.0.0") {
		t.Errorf("tampered plugin should not be installed")
	}
}

func TestSystem_MissingPublisherFile(t *testing.T) {
	embFS, _ := buildEmbeddedFS(t, "com.platypus.sys-noop", "1.0.0")
	delete(embFS, "publisher.pub")

	reg := freshRegistry(t)
	defer reg.Close(context.Background())

	res := EnsureInstalled(context.Background(), reg, embFS, EnsureOptions{AllowAll: true})
	if res.SetupError == nil {
		t.Fatalf("expected setup error")
	}
	if !strings.Contains(res.SetupError.Error(), "publisher.pub") {
		t.Errorf("err = %v, want publisher.pub mention", res.SetupError)
	}
}

// buildEmbeddedFSMulti stages N signed plugins under one common
// publisher key so an allowlist test can pick out a subset.
func buildEmbeddedFSMulti(t *testing.T, ids []string) fstest.MapFS {
	t.Helper()
	sk, pk, err := plugin.GenerateKeyPair()
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	out := fstest.MapFS{
		"publisher.pub": {Data: []byte(plugin.EncodePublicKey(pk, ""))},
	}
	for _, id := range ids {
		manifest := strings.NewReplacer(
			"PLUGIN_ID", id,
			"VERSION", "1.0.0",
			"PLACEHOLDER", plugin.HumanKeyID(pk),
		).Replace(noopManifestTmpl)
		sig, err := plugin.Sign(sk, noopWasm, plugin.DefaultTrustedComment("noop.wasm"))
		if err != nil {
			t.Fatalf("sign: %v", err)
		}
		out[id+"/1.0.0/plugin.yaml"] = &fstest.MapFile{Data: []byte(manifest)}
		out[id+"/1.0.0/noop.wasm"] = &fstest.MapFile{Data: noopWasm}
		out[id+"/1.0.0/noop.wasm.minisig"] = &fstest.MapFile{Data: []byte(plugin.EncodeSignature(sig))}
	}
	return out
}

// TestEnsureInstalled_RespectsAllowlist exercises the operator
// allowlist filter: with three plugins staged under the embedded FS,
// passing Allowlist=[id1,id2] must install exactly id1+id2 and
// surface id3 in Result.Filtered. AllowAll=false with an empty
// Allowlist must filter all of them (mandatory-core merge happens
// in the agent main, not here).
func TestEnsureInstalled_RespectsAllowlist(t *testing.T) {
	ids := []string{
		"com.platypus.sys-info",
		"com.platypus.sys-listdir",
		"com.platypus.sys-procs",
	}
	embFS := buildEmbeddedFSMulti(t, ids)

	t.Run("partial allowlist", func(t *testing.T) {
		reg := freshRegistry(t)
		defer reg.Close(context.Background())
		res := EnsureInstalled(context.Background(), reg, embFS, EnsureOptions{
			Allowlist: []string{"com.platypus.sys-info", "com.platypus.sys-listdir"},
		})
		if res.SetupError != nil {
			t.Fatalf("setup: %v", res.SetupError)
		}
		if len(res.Installed) != 2 {
			t.Fatalf("Installed = %+v; want 2", res.Installed)
		}
		if len(res.Filtered) != 1 || res.Filtered[0].ID != "com.platypus.sys-procs" {
			t.Fatalf("Filtered = %+v; want sys-procs only", res.Filtered)
		}
		if reg.HasInstalledVersion("com.platypus.sys-procs", "1.0.0") {
			t.Errorf("sys-procs should not be installed (filtered)")
		}
	})

	t.Run("empty allowlist filters everything", func(t *testing.T) {
		reg := freshRegistry(t)
		defer reg.Close(context.Background())
		res := EnsureInstalled(context.Background(), reg, embFS, EnsureOptions{})
		if res.SetupError != nil {
			t.Fatalf("setup: %v", res.SetupError)
		}
		if len(res.Installed) != 0 {
			t.Fatalf("Installed = %+v; want empty", res.Installed)
		}
		if len(res.Filtered) != 3 {
			t.Fatalf("Filtered = %+v; want all 3", res.Filtered)
		}
	})

	t.Run("AllowAll overrides allowlist", func(t *testing.T) {
		reg := freshRegistry(t)
		defer reg.Close(context.Background())
		res := EnsureInstalled(context.Background(), reg, embFS, EnsureOptions{
			AllowAll:  true,
			Allowlist: []string{"com.platypus.sys-info"},
		})
		if len(res.Installed) != 3 {
			t.Fatalf("Installed = %+v; want all 3 (AllowAll=true ignores Allowlist)", res.Installed)
		}
		if len(res.Filtered) != 0 {
			t.Fatalf("Filtered = %+v; want empty when AllowAll=true", res.Filtered)
		}
	})
}

func TestSystem_RemoveRefusedForSystemPlugin(t *testing.T) {
	embFS, _ := buildEmbeddedFS(t, "com.platypus.sys-noop", "1.0.0")
	reg := freshRegistry(t)
	defer reg.Close(context.Background())

	if r := EnsureInstalled(context.Background(), reg, embFS, EnsureOptions{AllowAll: true}); len(r.Installed) != 1 {
		t.Fatalf("install: %+v", r)
	}
	err := reg.Remove(context.Background(), "com.platypus.sys-noop", false)
	if !errors.Is(err, plugin.ErrPluginIsSystem) {
		t.Errorf("err = %v, want ErrPluginIsSystem", err)
	}
}
