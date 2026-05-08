package plugin

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/WangYihang/Platypus/internal/link"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// TestEchoStream_RustExamplePluginRoundTrip drives the wasm-streaming
// path end-to-end using the real Rust example plugin from
// examples/plugins/echo-stream/. Same skip semantics as the RPC echo
// integration test next door — without the .wasm built we can't
// exercise the runtime, so we skip rather than fail.
//
// Verifies:
//   - the Rust extism PDK's host_stream_read / host_stream_write /
//     host_stream_close calls cross the wasm boundary correctly
//   - DispatchPluginStream parses the metadata, instantiates the
//     plugin, runs the pumps, and joins everything cleanly
//   - bytes written on the inbound side come back on the outbound
//     side, terminated with KIND_EOF
func TestEchoStream_RustExamplePluginRoundTrip(t *testing.T) {
	wasmPath := echoStreamWasmPath(t)
	wasm, err := os.ReadFile(wasmPath)
	if err != nil {
		t.Skipf("echo_stream.wasm not built (%v) — run `cargo build --release --target wasm32-unknown-unknown` in examples/plugins/echo-stream/", err)
	}
	manifestBytes, err := os.ReadFile(filepath.Join("..", "..", "..", "example", "plugins", "echo-stream", "plugin.yaml"))
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

	manifestStr := strings.Replace(string(manifestBytes), "REPLACE_WITH_YOUR_KEY_ID", HumanKeyID(pk), 1)

	sig, err := Sign(sk, wasm, DefaultTrustedComment("echo_stream.wasm"))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	reg, err := New(Options{Paths: paths})
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	defer reg.Close(context.Background())

	if err := reg.InstallFromBytes(context.Background(), InstallParams{
		PluginID:        "com.platypus.example-echo-stream",
		Version:         "1.0.0",
		PublisherPubkey: []byte(EncodePublicKey(pk, "")),
		Manifest:        []byte(manifestStr),
		Wasm:            wasm,
		Signature:       []byte(EncodeSignature(sig)),
		Actor:           "test",
	}, nil); err != nil {
		t.Fatalf("install: %v", err)
	}

	// Wire up the synchronous in-memory pipe. streamA is the agent
	// side (handed to DispatchPluginStream); streamB is the wire-peer
	// side (the test plays server).
	streamA, streamB := net.Pipe()
	defer streamA.Close()
	defer streamB.Close()

	meta, err := proto.Marshal(&v2pb.PluginStreamRequest{
		PluginId:   "com.platypus.example-echo-stream",
		StreamName: "echo",
	})
	if err != nil {
		t.Fatalf("marshal meta: %v", err)
	}

	// Run the dispatcher in a goroutine; the test goroutine plays the
	// peer (writes inbound, reads outbound).
	dispatchDone := make(chan error, 1)
	go func() {
		dispatchDone <- reg.DispatchPluginStream(context.Background(), streamA, meta)
	}()

	want := "hello wasm-streaming world"

	// Push one inbound DATA frame + a terminal INBOUND EOF.
	if err := link.WriteFrame(streamB, &v2pb.PluginStreamFrame{
		Source: v2pb.PluginStreamFrame_SOURCE_INBOUND,
		Kind:   v2pb.PluginStreamFrame_KIND_DATA,
		Data:   []byte(want),
	}); err != nil {
		t.Fatalf("write inbound DATA: %v", err)
	}
	if err := link.WriteFrame(streamB, &v2pb.PluginStreamFrame{
		Source: v2pb.PluginStreamFrame_SOURCE_INBOUND,
		Kind:   v2pb.PluginStreamFrame_KIND_EOF,
	}); err != nil {
		t.Fatalf("write inbound EOF: %v", err)
	}

	// Read frames until we see OUTBOUND DATA matching the input + a
	// terminal OUTBOUND EOF. Tolerate frame splitting (the plugin may
	// chunk differently than the input).
	var got []byte
	deadline := time.Now().Add(15 * time.Second)
	sawEOF := false
	for time.Now().Before(deadline) {
		var frame v2pb.PluginStreamFrame
		if err := link.ReadFrame(streamB, &frame); err != nil {
			t.Fatalf("read outbound frame: %v", err)
		}
		if frame.GetSource() != v2pb.PluginStreamFrame_SOURCE_OUTBOUND {
			t.Errorf("unexpected source: %s", frame.GetSource())
			continue
		}
		switch frame.GetKind() {
		case v2pb.PluginStreamFrame_KIND_DATA:
			got = append(got, frame.GetData()...)
		case v2pb.PluginStreamFrame_KIND_EOF:
			sawEOF = true
		case v2pb.PluginStreamFrame_KIND_ERROR:
			t.Fatalf("error frame: %s: %s", frame.GetErrorCode(), frame.GetErrorMessage())
		}
		if sawEOF {
			break
		}
	}
	if !sawEOF {
		t.Fatalf("never saw OUTBOUND EOF; got so far = %q", got)
	}
	if string(got) != want {
		t.Errorf("echoed bytes = %q, want %q", got, want)
	}

	// Wasm method should have returned cleanly.
	select {
	case err := <-dispatchDone:
		if err != nil {
			t.Errorf("dispatch err: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("DispatchPluginStream did not return")
	}
}

func echoStreamWasmPath(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "..", "..", "example", "plugins", "echo-stream",
		"target", "wasm32-unknown-unknown", "release", "echo_stream.wasm")
}
