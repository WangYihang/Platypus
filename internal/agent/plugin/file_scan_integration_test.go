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

// TestFileScan_RustPluginRoundTrip exercises sys-file-scan against a
// fixture directory tree, asserting the totals match what the legacy
// Go HandleFileScanStream would have computed.
func TestFileScan_RustPluginRoundTrip(t *testing.T) {
	wasmPath := fileScanWasmPath(t)
	wasm, err := os.ReadFile(wasmPath)
	if err != nil {
		t.Skipf("sys_file_scan.wasm not built (%v) — run `cargo build --release --target wasm32-unknown-unknown` in example/plugins/sys-file-scan/", err)
	}
	manifestBytes, err := os.ReadFile(filepath.Join("..", "..", "..", "example", "plugins", "sys-file-scan", "plugin.yaml"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}

	// Build a tiny tree:
	//   root/
	//     a.txt        — 100 bytes
	//     sub/
	//       b.txt      — 200 bytes
	//       deep/
	//         c.txt    — 50 bytes
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), make([]byte, 100), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "sub")
	if err := os.MkdirAll(filepath.Join(sub, "deep"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "b.txt"), make([]byte, 200), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "deep", "c.txt"), make([]byte, 50), 0o644); err != nil {
		t.Fatal(err)
	}

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
	sig, err := Sign(sk, wasm, DefaultTrustedComment("sys_file_scan.wasm"))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	reg, err := New(Options{Paths: paths})
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	defer reg.Close(context.Background())

	if err := reg.InstallFromBytes(context.Background(), InstallParams{
		PluginID:            "com.platypus.sys-file-scan",
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

	meta, err := proto.Marshal(&v2pb.FileScanRequest{Paths: []string{root}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	dispatchDone := make(chan error, 1)
	go func() {
		_, err := reg.DispatchStream(context.Background(),
			v2pb.StreamType_STREAM_TYPE_FILE_SCAN, streamA, meta)
		dispatchDone <- err
	}()

	var resp v2pb.FileScanResponse
	if err := link.ReadFrame(streamB, &resp); err != nil {
		t.Fatalf("read response: %v", err)
	}
	if resp.GetError() != "" {
		t.Fatalf("scan error: %s", resp.GetError())
	}
	// Expected totals: 3 regular files (a.txt + b.txt + c.txt),
	// 3 directories (root + sub + deep), 350 bytes.
	if resp.GetFileCount() != 3 {
		t.Errorf("file_count = %d, want 3", resp.GetFileCount())
	}
	if resp.GetDirCount() != 3 {
		t.Errorf("dir_count = %d, want 3", resp.GetDirCount())
	}
	if resp.GetTotalBytes() != 350 {
		t.Errorf("total_bytes = %d, want 350", resp.GetTotalBytes())
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

func fileScanWasmPath(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "..", "..", "example", "plugins", "sys-file-scan",
		"target", "wasm32-unknown-unknown", "release", "sys_file_scan.wasm")
}
