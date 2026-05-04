package plugin_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/WangYihang/Platypus/internal/agent/plugin"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// installHelloGo wires the staged hello-go (TinyGo) wasm plugin into
// a fresh registry. The plugin needs no capabilities (CapLog is
// implicit). Same install path as the Rust plugins — agent has no
// idea the .wasm came from a different source language.
func installHelloGo(t *testing.T) *plugin.Registry {
	t.Helper()
	wasm := stagedWasmBytes(t, "com.platypus.hello-go", "1.0.0", "hello.wasm")
	manifestBytes := stagedManifestBytes(t, "com.platypus.hello-go", "1.0.0")

	pluginRoot := t.TempDir()
	paths := plugin.NewPaths(pluginRoot)
	sk, pk, err := plugin.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(paths.PublishersDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.PublisherKeyFile(plugin.HumanKeyID(pk)),
		[]byte(plugin.EncodePublicKey(pk, "")), 0o600); err != nil {
		t.Fatal(err)
	}
	manifestStr := rewriteManifestKeyID(string(manifestBytes), plugin.HumanKeyID(pk))
	sig, err := plugin.Sign(sk, wasm, plugin.DefaultTrustedComment("hello.wasm"))
	if err != nil {
		t.Fatal(err)
	}
	reg, err := plugin.New(plugin.Options{Paths: paths})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { reg.Close(context.Background()) })

	if err := reg.InstallFromBytes(context.Background(), plugin.InstallParams{
		PluginID:        "com.platypus.hello-go",
		Version:         "1.0.0",
		PublisherPubkey: []byte(plugin.EncodePublicKey(pk, "")),
		Manifest:        []byte(manifestStr),
		Wasm:            wasm,
		Signature:       []byte(plugin.EncodeSignature(sig)),
		Actor:           "test",
		// CapLog is implicit; no other caps needed.
		GrantedCapabilities: nil,
	}, nil); err != nil {
		t.Fatal(err)
	}
	return reg
}

// TestHelloGo_InvokeRoundTrip is the G0 smoke test: install the
// TinyGo-built hello-go plugin, invoke its `hello` export with a
// name, assert the response is the formatted greeting. Proves the
// extism plugin contract round-trips through Go source.
func TestHelloGo_InvokeRoundTrip(t *testing.T) {
	reg := installHelloGo(t)

	resp := reg.Invoke(context.Background(), &v2pb.PluginCallRequest{
		PluginId: "com.platypus.hello-go",
		Method:   "hello",
		Payload:  []byte("operator"),
	})
	if resp.GetError() != "" {
		t.Fatalf("plugin error: %s", resp.GetError())
	}
	got := string(resp.GetPayload())
	const want = "Hello, operator from Go!"
	if got != want {
		t.Errorf("payload = %q, want %q", got, want)
	}
}

// TestHelloGo_DefaultsToWorld verifies the no-input branch: the
// plugin substitutes "world" when input is empty. Tests both the
// pdk.Input -> string round-trip and the SDK's host_log call (which
// fires unconditionally — failure would surface as a plugin trap).
func TestHelloGo_DefaultsToWorld(t *testing.T) {
	reg := installHelloGo(t)

	resp := reg.Invoke(context.Background(), &v2pb.PluginCallRequest{
		PluginId: "com.platypus.hello-go",
		Method:   "hello",
		Payload:  nil,
	})
	if resp.GetError() != "" {
		t.Fatalf("plugin error: %s", resp.GetError())
	}
	if !strings.Contains(string(resp.GetPayload()), "world") {
		t.Errorf("default greeting missing 'world': %q", resp.GetPayload())
	}
}
