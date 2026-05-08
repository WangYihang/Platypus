package plugin

import (
	"context"
	"io"
	"net"
	"os"
	"strings"
	"testing"

	"github.com/jedisct1/go-minisign"

	"github.com/WangYihang/Platypus/internal/link"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// noopWasm is the minimal valid WebAssembly module exporting one
// function `noop` that takes no params, returns nothing, and just
// executes `end`. Hand-assembled to avoid pulling Rust into the
// integration test toolchain.
//
// Layout (per WebAssembly Core 1 spec):
//   magic + version (8 bytes)
//   type section:    one ()->() func type
//   function section: one function of type 0
//   export section:  "noop" -> function 0
//   code section:    one body, no locals, just `end`
var noopWasm = []byte{
	0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00, // \0asm v1
	0x01, 0x04, 0x01, 0x60, 0x00, 0x00, // type: func ()->()
	0x03, 0x02, 0x01, 0x00, // function: 1 func of type 0
	0x07, 0x08, 0x01, 0x04, 'n', 'o', 'o', 'p', 0x00, 0x00, // export "noop" func 0
	0x0a, 0x04, 0x01, 0x02, 0x00, 0x0b, // code: 1 body, 0 locals, end
}

const noopManifestTmpl = `
api_version: 1
id: com.example.noop
name: Noop
version: 1.0.0
author: { name: Test, email: test@example.com }
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

// TestEndToEnd_InstallAndInvoke exercises the runtime's full happy path
// without any wire transport: produce a fresh keypair + signed wasm,
// run handleInstall through an in-memory pipe, then call Invoke and
// assert success. Smoke test that proves manifest parsing, signature
// verification, atomic extract, hot-load, and dispatch all fit
// together end-to-end.
func TestEndToEnd_InstallAndInvoke(t *testing.T) {
	root := t.TempDir()
	paths := NewPaths(root)

	sk, pk, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	if err := writePubkeyFile(paths, pk); err != nil {
		t.Fatalf("write pubkey: %v", err)
	}

	manifestBytes := []byte(strings.Replace(noopManifestTmpl, "PLACEHOLDER", HumanKeyID(pk), 1))
	sig, err := Sign(sk, noopWasm, DefaultTrustedComment("noop.wasm"))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	sigBytes := []byte(EncodeSignature(sig))
	pubBytes := []byte(EncodePublicKey(pk, ""))

	reg, err := New(Options{Paths: paths})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer reg.Close(context.Background())

	stream := newPipeStream()
	defer stream.Close()

	req := &v2pb.PluginInstallRequest{
		PluginId:        "com.example.noop",
		Version:         "1.0.0",
		PublisherPubkey: pubBytes,
		Source: &v2pb.PluginInstallRequest_Inline{
			Inline: &v2pb.PluginInlineSource{WasmSizeBytes: uint64(len(noopWasm))},
		},
		Actor: "user:test",
	}

	// Goroutine 1: drain progress frames so the agent's writes don't
	// block on the synchronous net.Pipe.
	progressCh := make(chan []*v2pb.PluginInstallProgress, 1)
	go func() { progressCh <- drainProgress(stream.client) }()

	// Goroutine 2: feed the three install chunks.
	go writeInstallChunks(stream.client, manifestBytes, noopWasm, sigBytes)

	if err := reg.handleInstall(context.Background(), stream.server, req); err != nil {
		t.Fatalf("handleInstall: %v", err)
	}
	progress := <-progressCh

	if len(progress) == 0 {
		t.Fatalf("no progress frames received")
	}
	if last := progress[len(progress)-1]; last.GetPhase() != v2pb.PluginInstallProgress_PHASE_INSTALLED {
		t.Fatalf("final phase = %v, want INSTALLED. all = %+v", last.GetPhase(), progress)
	}

	if entries := reg.catalog.All(); len(entries) != 1 || entries[0].ID != "com.example.noop" {
		t.Fatalf("catalog after install: %+v", entries)
	}
	for _, p := range []string{
		paths.ManifestFile("com.example.noop", "1.0.0"),
		paths.WasmFile("com.example.noop", "1.0.0", "noop.wasm"),
		paths.SignatureFile("com.example.noop", "1.0.0", "noop.wasm"),
	} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected file at %s: %v", p, err)
		}
	}

	// Invoke the noop export — should succeed cleanly.
	resp := reg.Invoke(context.Background(), &v2pb.PluginCallRequest{
		PluginId: "com.example.noop",
		Method:   "noop",
	})
	if resp.GetError() != "" {
		t.Errorf("invoke error: %q", resp.GetError())
	}

	// Undeclared method must be rejected without entering wasm.
	resp = reg.Invoke(context.Background(), &v2pb.PluginCallRequest{
		PluginId: "com.example.noop",
		Method:   "nonexistent",
	})
	if !strings.Contains(resp.GetError(), "method_not_declared") {
		t.Errorf("undeclared method error = %q, want method_not_declared", resp.GetError())
	}

	// Unknown plugin id must return plugin_not_installed.
	resp = reg.Invoke(context.Background(), &v2pb.PluginCallRequest{
		PluginId: "com.example.missing",
		Method:   "noop",
	})
	if !strings.Contains(resp.GetError(), "plugin_not_installed") {
		t.Errorf("missing plugin error = %q, want plugin_not_installed", resp.GetError())
	}
}

// TestEndToEnd_BadSignatureRejected proves that a tampered .wasm can't
// install: handleInstall must return signature_mismatch, the catalog
// stays empty, and no version dir is left on disk.
func TestEndToEnd_BadSignatureRejected(t *testing.T) {
	root := t.TempDir()
	paths := NewPaths(root)

	sk, pk, _ := GenerateKeyPair()
	if err := writePubkeyFile(paths, pk); err != nil {
		t.Fatalf("write pubkey: %v", err)
	}

	manifestBytes := []byte(strings.Replace(noopManifestTmpl, "PLACEHOLDER", HumanKeyID(pk), 1))
	// Sign one body; ship a different one.
	sig, _ := Sign(sk, []byte("not the wasm we ship"), DefaultTrustedComment("noop.wasm"))
	sigBytes := []byte(EncodeSignature(sig))
	pubBytes := []byte(EncodePublicKey(pk, ""))

	reg, _ := New(Options{Paths: paths})
	defer reg.Close(context.Background())

	stream := newPipeStream()
	defer stream.Close()

	req := &v2pb.PluginInstallRequest{
		PluginId:        "com.example.noop",
		Version:         "1.0.0",
		PublisherPubkey: pubBytes,
		Source: &v2pb.PluginInstallRequest_Inline{
			Inline: &v2pb.PluginInlineSource{WasmSizeBytes: uint64(len(noopWasm))},
		},
	}
	go drainProgress(stream.client) // discard
	go writeInstallChunks(stream.client, manifestBytes, noopWasm, sigBytes)

	err := reg.handleInstall(context.Background(), stream.server, req)
	if err == nil || !strings.Contains(err.Error(), "signature_mismatch") {
		t.Errorf("install err = %v, want signature_mismatch", err)
	}
	if entries := reg.catalog.All(); len(entries) != 0 {
		t.Errorf("catalog should be empty after rejected install: %+v", entries)
	}
	if _, err := os.Stat(paths.WasmFile("com.example.noop", "1.0.0", "noop.wasm")); err == nil {
		t.Errorf("wasm file present after rejected install")
	}
}

// ---- in-memory stream + frame helpers ---------------------------------

// pipeStream is a bidirectional in-memory stream pair built on
// net.Pipe. The agent reads chunks + writes progress frames on
// `server`; the test writes chunks + reads progress frames on
// `client`. Both ends are synchronous, so all writes need a concurrent
// reader to avoid deadlock — the helpers below run their reads/writes
// in goroutines.
type pipeStream struct {
	server net.Conn
	client net.Conn
}

func newPipeStream() *pipeStream {
	s, c := net.Pipe()
	return &pipeStream{server: s, client: c}
}

func (s *pipeStream) Close() {
	_ = s.server.Close()
	_ = s.client.Close()
}

// drainProgress collects every PluginInstallProgress frame the agent
// emits, stopping at the first terminal phase or io.EOF.
func drainProgress(r io.Reader) []*v2pb.PluginInstallProgress {
	var out []*v2pb.PluginInstallProgress
	for {
		var p v2pb.PluginInstallProgress
		if err := link.ReadFrame(r, &p); err != nil {
			return out
		}
		out = append(out, &p)
		if p.GetPhase() == v2pb.PluginInstallProgress_PHASE_INSTALLED ||
			p.GetPhase() == v2pb.PluginInstallProgress_PHASE_FAILED {
			return out
		}
	}
}

// writeInstallChunks frames the three install segments (MANIFEST →
// WASM → SIGNATURE), one chunk per segment with last=true.
func writeInstallChunks(w io.Writer, manifest, wasm, sig []byte) {
	for _, seg := range []struct {
		kind v2pb.PluginInstallChunk_Kind
		data []byte
	}{
		{v2pb.PluginInstallChunk_KIND_MANIFEST, manifest},
		{v2pb.PluginInstallChunk_KIND_WASM, wasm},
		{v2pb.PluginInstallChunk_KIND_SIGNATURE, sig},
	} {
		_ = link.WriteFrame(w, &v2pb.PluginInstallChunk{
			Kind: seg.kind, Data: seg.data, Last: true,
		})
	}
}

// writePubkeyFile drops a pubkey under publishers/<keyid>.pub so a
// future Registry.Load() can resolve it from disk. Inline installs use
// the pubkey passed in PluginInstallRequest directly; this exercises
// the production trust path that survives an agent restart.
func writePubkeyFile(paths Paths, pk minisign.PublicKey) error {
	if err := os.MkdirAll(paths.PublishersDir(), 0o700); err != nil {
		return err
	}
	return os.WriteFile(paths.PublisherKeyFile(HumanKeyID(pk)),
		[]byte(EncodePublicKey(pk, "")), 0o600)
}
