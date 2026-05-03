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

// TestFileRead_RustPluginRoundTrip drives the legacy-wasm bridge end
// to end against the real sys-file-read plugin. The wire format the
// plugin produces is byte-for-byte the same as the legacy Go
// HandleFileReadStream's output: one length-prefixed FileReadResponse
// followed by length-prefixed FileChunk frames.
//
// Skips when the plugin's .wasm hasn't been built — same posture as
// the existing echo-stream integration test next door.
func TestFileRead_RustPluginRoundTrip(t *testing.T) {
	wasmPath := fileReadWasmPath(t)
	wasm, err := os.ReadFile(wasmPath)
	if err != nil {
		t.Skipf("sys_file_read.wasm not built (%v) — run `cargo build --release --target wasm32-unknown-unknown` in example/plugins/sys-file-read/", err)
	}
	manifestBytes, err := os.ReadFile(filepath.Join("..", "..", "..", "example", "plugins", "sys-file-read", "plugin.yaml"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}

	tmpFile := filepath.Join(t.TempDir(), "data.bin")
	want := []byte(strings.Repeat("Platypus file_read migration smoke test.\n", 4096))
	if err := os.WriteFile(tmpFile, want, 0o644); err != nil {
		t.Fatalf("write data: %v", err)
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

	sig, err := Sign(sk, wasm, DefaultTrustedComment("sys_file_read.wasm"))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	reg, err := New(Options{Paths: paths})
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	defer reg.Close(context.Background())

	if err := reg.InstallFromBytes(context.Background(), InstallParams{
		PluginID:            "com.platypus.sys-file-read",
		Version:             "1.0.0",
		PublisherPubkey:     []byte(EncodePublicKey(pk, "")),
		Manifest:            []byte(manifestStr),
		Wasm:                wasm,
		Signature:           []byte(EncodeSignature(sig)),
		Actor:               "test",
		GrantedCapabilities: []string{"fs.read"},
	}, nil); err != nil {
		t.Fatalf("install: %v", err)
	}

	streamA, streamB := net.Pipe()
	defer streamA.Close()
	defer streamB.Close()

	meta, err := proto.Marshal(&v2pb.FileReadRequest{
		Path: tmpFile,
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	dispatchDone := make(chan error, 1)
	go func() {
		// DispatchStream is the agent's serve_link.go entry point;
		// we exercise the same fn so the legacy-wasm-bridge routing
		// + the wasm dispatch are both in scope.
		_, err := reg.DispatchStream(context.Background(),
			v2pb.StreamType_STREAM_TYPE_FILE_READ, streamA, meta)
		dispatchDone <- err
	}()

	// Read the wire as a sequence of length-prefixed proto frames.
	// First: FileReadResponse header.
	var hdr v2pb.FileReadResponse
	if err := link.ReadFrame(streamB, &hdr); err != nil {
		t.Fatalf("read header: %v", err)
	}
	if hdr.GetError() != "" {
		t.Fatalf("header error: %s", hdr.GetError())
	}
	if hdr.GetSize() != int64(len(want)) {
		t.Errorf("header size = %d, want %d", hdr.GetSize(), len(want))
	}

	// Then: FileChunk frames until eof.
	var got []byte
	deadline := time.Now().Add(15 * time.Second)
	sawEOF := false
	for time.Now().Before(deadline) && !sawEOF {
		var chunk v2pb.FileChunk
		if err := link.ReadFrame(streamB, &chunk); err != nil {
			t.Fatalf("read chunk: %v", err)
		}
		if chunk.GetError() != "" {
			t.Fatalf("chunk error: %s", chunk.GetError())
		}
		got = append(got, chunk.GetData()...)
		if chunk.GetEof() {
			sawEOF = true
		}
	}
	if !sawEOF {
		t.Fatalf("never saw eof — got %d bytes so far", len(got))
	}
	if string(got) != string(want) {
		t.Errorf("data mismatch: got %d bytes, want %d", len(got), len(want))
	}

	select {
	case err := <-dispatchDone:
		if err != nil {
			t.Errorf("dispatch err: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("DispatchStream did not return")
	}
}

func fileReadWasmPath(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "..", "..", "example", "plugins", "sys-file-read",
		"target", "wasm32-unknown-unknown", "release", "sys_file_read.wasm")
}
