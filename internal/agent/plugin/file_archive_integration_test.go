package plugin

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/WangYihang/Platypus/internal/link"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// TestFileArchive_TarRoundTrip drives sys-file-archive end to end
// in TAR (uncompressed) mode against a fixture tree, asserts the
// produced bytes parse cleanly with archive/tar, and that every
// fixture file is present with the correct content.
func TestFileArchive_TarRoundTrip(t *testing.T) {
	rawArchive := runArchive(t, v2pb.ArchiveFormat_ARCHIVE_FORMAT_TAR)
	verifyTar(t, rawArchive)
}

// TestFileArchive_TarGzRoundTrip drives the same flow with gzip
// wrapping. Decompresses then reuses the same archive/tar verifier.
func TestFileArchive_TarGzRoundTrip(t *testing.T) {
	rawArchive := runArchive(t, v2pb.ArchiveFormat_ARCHIVE_FORMAT_TAR_GZ)
	gz, err := gzip.NewReader(bytes.NewReader(rawArchive))
	if err != nil {
		t.Fatalf("gzip.NewReader: %v", err)
	}
	defer gz.Close()
	plain, err := io.ReadAll(gz)
	if err != nil {
		t.Fatalf("gunzip: %v", err)
	}
	verifyTar(t, plain)
}

// TestFileArchive_ZipReturnsClearError documents the wasm parity gap:
// the legacy Go handler supported ZIP via archive/zip; the wasm
// replacement does not (rust zip crate is too heavy in wasm). The
// plugin returns a clear error in the response header so operators
// see the actionable message.
func TestFileArchive_ZipReturnsClearError(t *testing.T) {
	wasmPath := fileArchiveWasmPath()
	wasm, err := os.ReadFile(wasmPath)
	if err != nil {
		t.Skipf("sys_file_archive.wasm not built (%v)", err)
	}
	manifestBytes, err := os.ReadFile(filepath.Join("..", "..", "..",
		"example", "plugins", "sys-file-archive", "plugin.yaml"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}

	pluginRoot := t.TempDir()
	paths := NewPaths(pluginRoot)
	sk, pk, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	if err := os.MkdirAll(paths.PublishersDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.PublisherKeyFile(HumanKeyID(pk)),
		[]byte(EncodePublicKey(pk, "")), 0o600); err != nil {
		t.Fatal(err)
	}
	manifestStr := strings.Replace(string(manifestBytes),
		"REPLACE_WITH_YOUR_KEY_ID", HumanKeyID(pk), 1)
	sig, err := Sign(sk, wasm, DefaultTrustedComment("sys_file_archive.wasm"))
	if err != nil {
		t.Fatal(err)
	}

	reg, err := New(Options{Paths: paths})
	if err != nil {
		t.Fatal(err)
	}
	defer reg.Close(context.Background())

	if err := reg.InstallFromBytes(context.Background(), InstallParams{
		PluginID:            "com.platypus.sys-file-archive",
		Version:             "1.0.0",
		PublisherPubkey:     []byte(EncodePublicKey(pk, "")),
		Manifest:            []byte(manifestStr),
		Wasm:                wasm,
		Signature:           []byte(EncodeSignature(sig)),
		Actor:               "test",
		GrantedCapabilities: []string{"fs.read"},
	}, nil); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "x.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}

	streamA, streamB := net.Pipe()
	defer streamA.Close()
	defer streamB.Close()

	meta, err := proto.Marshal(&v2pb.FileArchiveRequest{
		Paths:  []string{root},
		Format: v2pb.ArchiveFormat_ARCHIVE_FORMAT_ZIP,
	})
	if err != nil {
		t.Fatal(err)
	}

	dispatchDone := make(chan error, 1)
	go func() {
		_, err := reg.DispatchStream(context.Background(),
			v2pb.StreamType_STREAM_TYPE_FILE_ARCHIVE, streamA, meta)
		dispatchDone <- err
	}()

	var hdr v2pb.FileArchiveResponse
	if err := link.ReadFrame(streamB, &hdr); err != nil {
		t.Fatalf("read header: %v", err)
	}
	if !strings.Contains(strings.ToLower(hdr.GetError()), "zip") {
		t.Errorf("header error = %q, want to mention zip", hdr.GetError())
	}
	// Drain the terminal eof chunk so the dispatcher returns.
	var trail v2pb.FileChunk
	if err := link.ReadFrame(streamB, &trail); err != nil {
		t.Logf("read trailing chunk (ok if EOF): %v", err)
	}

	select {
	case <-dispatchDone:
	case <-time.After(5 * time.Second):
		t.Fatal("DispatchStream did not return")
	}
}

// runArchive installs the plugin, builds a fixture tree, drives a
// STREAM_TYPE_FILE_ARCHIVE through DispatchStream, drains all
// FileChunk frames, returns the concatenated archive bytes.
func runArchive(t *testing.T, format v2pb.ArchiveFormat) []byte {
	t.Helper()
	wasmPath := fileArchiveWasmPath()
	wasm, err := os.ReadFile(wasmPath)
	if err != nil {
		t.Skipf("sys_file_archive.wasm not built (%v) — run `cargo build --release --target wasm32-unknown-unknown` in example/plugins/sys-file-archive/", err)
	}
	manifestBytes, err := os.ReadFile(filepath.Join("..", "..", "..",
		"example", "plugins", "sys-file-archive", "plugin.yaml"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}

	// Fixture tree:
	//   root/
	//     a.txt        — "alpha bytes"
	//     sub/
	//       b.txt      — "beta bytes"
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("alpha bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "b.txt"), []byte("beta bytes"), 0o644); err != nil {
		t.Fatal(err)
	}

	pluginRoot := t.TempDir()
	paths := NewPaths(pluginRoot)
	sk, pk, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	if err := os.MkdirAll(paths.PublishersDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.PublisherKeyFile(HumanKeyID(pk)),
		[]byte(EncodePublicKey(pk, "")), 0o600); err != nil {
		t.Fatal(err)
	}
	manifestStr := strings.Replace(string(manifestBytes),
		"REPLACE_WITH_YOUR_KEY_ID", HumanKeyID(pk), 1)
	sig, err := Sign(sk, wasm, DefaultTrustedComment("sys_file_archive.wasm"))
	if err != nil {
		t.Fatal(err)
	}

	reg, err := New(Options{Paths: paths})
	if err != nil {
		t.Fatal(err)
	}
	defer reg.Close(context.Background())

	if err := reg.InstallFromBytes(context.Background(), InstallParams{
		PluginID:            "com.platypus.sys-file-archive",
		Version:             "1.0.0",
		PublisherPubkey:     []byte(EncodePublicKey(pk, "")),
		Manifest:            []byte(manifestStr),
		Wasm:                wasm,
		Signature:           []byte(EncodeSignature(sig)),
		Actor:               "test",
		GrantedCapabilities: []string{"fs.read"},
	}, nil); err != nil {
		t.Fatal(err)
	}

	streamA, streamB := net.Pipe()
	defer streamA.Close()
	defer streamB.Close()

	meta, err := proto.Marshal(&v2pb.FileArchiveRequest{
		Paths:  []string{root},
		Format: format,
	})
	if err != nil {
		t.Fatal(err)
	}

	dispatchDone := make(chan error, 1)
	go func() {
		_, err := reg.DispatchStream(context.Background(),
			v2pb.StreamType_STREAM_TYPE_FILE_ARCHIVE, streamA, meta)
		dispatchDone <- err
	}()

	var hdr v2pb.FileArchiveResponse
	if err := link.ReadFrame(streamB, &hdr); err != nil {
		t.Fatalf("read header: %v", err)
	}
	if hdr.GetError() != "" {
		t.Fatalf("archive error: %s", hdr.GetError())
	}

	var assembled []byte
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		var chunk v2pb.FileChunk
		if err := link.ReadFrame(streamB, &chunk); err != nil {
			t.Fatalf("read chunk: %v", err)
		}
		assembled = append(assembled, chunk.GetData()...)
		if chunk.GetEof() {
			if chunk.GetError() != "" {
				t.Fatalf("trailer error: %s", chunk.GetError())
			}
			select {
			case <-dispatchDone:
			case <-time.After(5 * time.Second):
				t.Fatal("DispatchStream did not return")
			}
			return assembled
		}
	}
	t.Fatal("never saw eof chunk")
	return nil
}

// verifyTar parses an uncompressed TAR archive and asserts the
// fixture's two regular files are present with their original
// content. Order of entries is implementation-defined (we sort).
func verifyTar(t *testing.T, raw []byte) {
	t.Helper()
	tr := tar.NewReader(bytes.NewReader(raw))
	type entry struct {
		name string
		body string
		mode int64
	}
	var got []entry
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar.Next: %v", err)
		}
		body, err := io.ReadAll(tr)
		if err != nil {
			t.Fatalf("tar body: %v", err)
		}
		got = append(got, entry{
			name: hdr.Name,
			body: string(body),
			mode: hdr.Mode,
		})
	}
	sort.Slice(got, func(i, j int) bool { return got[i].name < got[j].name })

	// Names use the basename of the root + relative paths underneath.
	// Don't pin the literal root basename (it's a temp-dir suffix);
	// look up by suffix instead.
	var aTxt, bTxt *entry
	for i := range got {
		switch {
		case strings.HasSuffix(got[i].name, "a.txt") && !strings.Contains(got[i].name, "sub/"):
			aTxt = &got[i]
		case strings.HasSuffix(got[i].name, "sub/b.txt"):
			bTxt = &got[i]
		}
	}
	if aTxt == nil {
		t.Fatalf("a.txt not found in archive; got %v", got)
	}
	if aTxt.body != "alpha bytes" {
		t.Errorf("a.txt body = %q, want %q", aTxt.body, "alpha bytes")
	}
	if bTxt == nil {
		t.Fatalf("sub/b.txt not found in archive; got %v", got)
	}
	if bTxt.body != "beta bytes" {
		t.Errorf("sub/b.txt body = %q, want %q", bTxt.body, "beta bytes")
	}
}

func fileArchiveWasmPath() string {
	return filepath.Join("..", "..", "..", "example", "plugins", "sys-file-archive",
		"target", "wasm32-unknown-unknown", "release", "sys_file_archive.wasm")
}
