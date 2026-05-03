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

// TestFileWrite_RustPluginRoundTrip exercises sys-file-write end to
// end via the legacy-wasm bridge. Test acts as the "uploader": opens
// a STREAM_TYPE_FILE_WRITE, reads the ack, pushes a couple of
// FileChunk frames + a terminal eof chunk, reads the result trailer.
// Asserts the destination file matches the bytes pushed.
func TestFileWrite_RustPluginRoundTrip(t *testing.T) {
	wasmPath := fileWriteWasmPath(t)
	wasm, err := os.ReadFile(wasmPath)
	if err != nil {
		t.Skipf("sys_file_write.wasm not built (%v) — run `cargo build --release --target wasm32-unknown-unknown` in example/plugins/sys-file-write/", err)
	}
	manifestBytes, err := os.ReadFile(filepath.Join("..", "..", "..", "example", "plugins", "sys-file-write", "plugin.yaml"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}

	dst := filepath.Join(t.TempDir(), "dst.bin")
	chunkA := []byte(strings.Repeat("Aa", 5000))
	chunkB := []byte(strings.Repeat("Bb", 5000))
	want := append(append([]byte{}, chunkA...), chunkB...)

	pluginRoot := t.TempDir()
	paths := NewPaths(pluginRoot)
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
	sig, err := Sign(sk, wasm, DefaultTrustedComment("sys_file_write.wasm"))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	reg, err := New(Options{Paths: paths})
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	defer reg.Close(context.Background())

	if err := reg.InstallFromBytes(context.Background(), InstallParams{
		PluginID:            "com.platypus.sys-file-write",
		Version:             "1.0.0",
		PublisherPubkey:     []byte(EncodePublicKey(pk, "")),
		Manifest:            []byte(manifestStr),
		Wasm:                wasm,
		Signature:           []byte(EncodeSignature(sig)),
		Actor:               "test",
		GrantedCapabilities: []string{"fs.write"},
	}, nil); err != nil {
		t.Fatalf("install: %v", err)
	}

	streamA, streamB := net.Pipe()
	defer streamA.Close()
	defer streamB.Close()

	meta, err := proto.Marshal(&v2pb.FileWriteRequest{
		Path: dst,
		Mode: 0o644,
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	dispatchDone := make(chan error, 1)
	go func() {
		_, err := reg.DispatchStream(context.Background(),
			v2pb.StreamType_STREAM_TYPE_FILE_WRITE, streamA, meta)
		dispatchDone <- err
	}()

	// Read ack.
	var ack v2pb.FileWriteResponse
	if err := link.ReadFrame(streamB, &ack); err != nil {
		t.Fatalf("read ack: %v", err)
	}
	if ack.GetError() != "" {
		t.Fatalf("ack error: %s", ack.GetError())
	}

	// Push two data chunks then a terminal eof chunk.
	if err := link.WriteFrame(streamB, &v2pb.FileChunk{Data: chunkA}); err != nil {
		t.Fatalf("write chunk A: %v", err)
	}
	if err := link.WriteFrame(streamB, &v2pb.FileChunk{Data: chunkB}); err != nil {
		t.Fatalf("write chunk B: %v", err)
	}
	if err := link.WriteFrame(streamB, &v2pb.FileChunk{Eof: true}); err != nil {
		t.Fatalf("write eof: %v", err)
	}

	// Read trailer.
	var result v2pb.FileWriteResult
	if err := link.ReadFrame(streamB, &result); err != nil {
		t.Fatalf("read result: %v", err)
	}
	if result.GetError() != "" {
		t.Fatalf("result error: %s", result.GetError())
	}
	if result.GetBytesWritten() != int64(len(want)) {
		t.Errorf("bytes_written = %d, want %d", result.GetBytesWritten(), len(want))
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dst: %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("dst content = %d bytes, want %d", len(got), len(want))
	}

	select {
	case err := <-dispatchDone:
		if err != nil {
			t.Errorf("dispatch err: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("DispatchStream did not return")
	}
}

func fileWriteWasmPath(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "..", "..", "example", "plugins", "sys-file-write",
		"target", "wasm32-unknown-unknown", "release", "sys_file_write.wasm")
}
