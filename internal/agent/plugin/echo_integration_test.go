package plugin

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// TestEcho_RustExamplePluginRoundTrip drives the runtime through the
// real Rust example plugin from examples/plugins/echo/. Skipped when
// the .wasm hasn't been built (so CI without rustup still passes —
// the canonical "does the runtime accept a real Rust extism plugin"
// answer comes from running this locally after `cargo build` in the
// example dir).
//
// Verifies:
//   - real Rust extism PDK output loads via wazero + extism without
//     manual ABI massaging
//   - install pipeline accepts a freshly-signed real plugin
//   - Invoke routes input bytes to the plugin and echoes them back
//
// To run: see examples/plugins/echo/README.md.
func TestEcho_RustExamplePluginRoundTrip(t *testing.T) {
	wasmPath := echoWasmPath(t)
	wasm, err := os.ReadFile(wasmPath)
	if err != nil {
		t.Skipf("echo.wasm not built (%v) — run `cargo build --release --target wasm32-unknown-unknown` in examples/plugins/echo/", err)
	}
	manifestBytes, err := os.ReadFile(filepath.Join("..", "..", "..", "example", "plugins", "echo", "plugin.yaml"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}

	root := t.TempDir()
	paths := NewPaths(root)
	sk, pk, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	if err := os.MkdirAll(paths.PublishersDir(), 0o700); err != nil {
		t.Fatalf("mkdir publishers: %v", err)
	}
	if err := os.WriteFile(paths.PublisherKeyFile(HumanKeyID(pk)),
		[]byte(EncodePublicKey(pk, "")), 0o600); err != nil {
		t.Fatalf("write pubkey: %v", err)
	}

	// Patch the manifest's REPLACE_WITH_YOUR_KEY_ID placeholder so it
	// validates against the freshly-generated test key.
	manifestStr := strings.Replace(string(manifestBytes), "REPLACE_WITH_YOUR_KEY_ID", HumanKeyID(pk), 1)

	sig, err := Sign(sk, wasm, DefaultTrustedComment("echo.wasm"))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	reg, err := New(Options{Paths: paths})
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	defer reg.Close(context.Background())

	if err := reg.InstallFromBytes(context.Background(), InstallParams{
		PluginID:        "com.platypus.example-echo",
		Version:         "1.0.0",
		PublisherPubkey: []byte(EncodePublicKey(pk, "")),
		Manifest:        []byte(manifestStr),
		Wasm:            wasm,
		Signature:       []byte(EncodeSignature(sig)),
		Actor:           "test",
	}, nil); err != nil {
		t.Fatalf("install: %v", err)
	}

	resp := reg.Invoke(context.Background(), &v2pb.PluginCallRequest{
		PluginId: "com.platypus.example-echo",
		Method:   "echo",
		Payload:  []byte("hello world"),
	})
	if resp.GetError() != "" {
		t.Fatalf("invoke err: %s", resp.GetError())
	}
	if string(resp.GetPayload()) != "hello world" {
		t.Errorf("payload = %q, want %q", resp.GetPayload(), "hello world")
	}
}

func echoWasmPath(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "..", "..", "example", "plugins", "echo",
		"target", "wasm32-unknown-unknown", "release", "echo.wasm")
}
