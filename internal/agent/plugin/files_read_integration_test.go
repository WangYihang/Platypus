package plugin_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/WangYihang/Platypus/internal/agent/plugin"
	"github.com/WangYihang/Platypus/internal/agent/plugin/bridge"
	"github.com/WangYihang/Platypus/internal/link"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// installSysFilesRead is the shared fixture for every sys-files-read
// test: builds a fresh Registry rooted at a temp dir, signs the
// freshly-built wasm with a per-test keypair, and installs it with
// the fs.read capability granted. Returns the registry; the caller
// owns Close.
//
// Tests are TDD-driven: the wasm itself is the SUT, this helper just
// wires it into a real registry so we exercise the same install +
// dispatch + host-fn path the production agent uses.
func installSysFilesRead(t *testing.T) *plugin.Registry {
	t.Helper()
	wasm, err := os.ReadFile(sysFilesReadWasmPath())
	if err != nil {
		t.Skipf("sys_files_read.wasm not built (%v) — run `cargo build --release --target wasm32-unknown-unknown` in example/plugins/system/sys-files-read/", err)
	}
	manifestBytes, err := os.ReadFile(filepath.Join("..", "..", "..",
		"example", "plugins", "system", "sys-files-read", "plugin.yaml"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}

	pluginRoot := t.TempDir()
	paths := plugin.NewPaths(pluginRoot)
	sk, pk, err := plugin.GenerateKeyPair()
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	if err := os.MkdirAll(paths.PublishersDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.PublisherKeyFile(plugin.HumanKeyID(pk)),
		[]byte(plugin.EncodePublicKey(pk, "")), 0o600); err != nil {
		t.Fatal(err)
	}
	manifestStr := strings.Replace(string(manifestBytes),
		"REPLACE_WITH_YOUR_KEY_ID", plugin.HumanKeyID(pk), 1)
	sig, err := plugin.Sign(sk, wasm, plugin.DefaultTrustedComment("sys_files_read.wasm"))
	if err != nil {
		t.Fatal(err)
	}

	reg, err := plugin.New(plugin.Options{Paths: paths})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { reg.Close(context.Background()) })

	if err := reg.InstallFromBytes(context.Background(), plugin.InstallParams{
		PluginID:            "com.platypus.sys-files-read",
		Version:             "1.0.1",
		PublisherPubkey:     []byte(plugin.EncodePublicKey(pk, "")),
		Manifest:            []byte(manifestStr),
		Wasm:                wasm,
		Signature:           []byte(plugin.EncodeSignature(sig)),
		Actor:               "test",
		GrantedCapabilities: []string{"fs.read"},
	}, nil); err != nil {
		t.Fatal(err)
	}
	return reg
}

// TestFilesRead_ListDir walks a fixture directory and asserts the
// merged plugin reports the three entries with the right is_dir
// classification (encoded as POSIX S_IFDIR in the mode field, the
// same convention sys-listdir used pre-merge).
func TestFilesRead_ListDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX-mode listdir behaviour is unix-only")
	}
	reg := installSysFilesRead(t)

	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "alpha.txt"), []byte("hello"))
	mustMkdir(t, filepath.Join(dir, "beta"))

	resp := bridge.ListDir(reg)(context.Background(), &v2pb.ListDirRequest{Path: dir})
	if resp.GetError() != "" {
		t.Fatalf("list_dir error: %s", resp.GetError())
	}
	got := map[string]uint32{}
	for _, e := range resp.GetEntries() {
		got[e.GetName()] = e.GetMode()
	}
	if mode := got["alpha.txt"]; mode == 0 || mode&0o040000 != 0 {
		t.Errorf("alpha.txt mode = %o; want regular-file (no dir bit)", mode)
	}
	if mode := got["beta"]; mode&0o040000 == 0 {
		t.Errorf("beta mode = %o; want dir bit set", mode)
	}
}

// TestFilesRead_ListDir_MissingPath: the wrapper surfaces the host
// error in ListDirResponse.error rather than crashing the wasm.
func TestFilesRead_ListDir_MissingPath(t *testing.T) {
	reg := installSysFilesRead(t)
	resp := bridge.ListDir(reg)(context.Background(),
		&v2pb.ListDirRequest{Path: "/this/path/definitely/does/not/exist"})
	if resp.GetError() == "" {
		t.Errorf("expected error for missing path, got entries=%d", len(resp.GetEntries()))
	}
}

// TestFilesRead_Stat_File: a regular file's stat reports its byte
// size + the regular-file mode marker (no dir bit).
func TestFilesRead_Stat_File(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX mode bits are unix-only")
	}
	reg := installSysFilesRead(t)

	dir := t.TempDir()
	target := filepath.Join(dir, "hello.txt")
	mustWrite(t, target, []byte("hello world"))

	resp := bridge.Stat(reg)(context.Background(), &v2pb.StatRequest{Path: target})
	if resp.GetError() != "" {
		t.Fatalf("stat err: %s", resp.GetError())
	}
	e := resp.GetEntry()
	if e == nil {
		t.Fatalf("expected non-nil entry")
	}
	if e.GetName() != "hello.txt" {
		t.Errorf("name = %q; want hello.txt", e.GetName())
	}
	if e.GetSize() != int64(len("hello world")) {
		t.Errorf("size = %d; want %d", e.GetSize(), len("hello world"))
	}
	if e.GetMode()&0o040000 != 0 {
		t.Errorf("regular file should not have dir bit; mode = %o", e.GetMode())
	}
}

// TestFilesRead_Stat_Directory: directory entries carry the dir bit.
func TestFilesRead_Stat_Directory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX mode bits are unix-only")
	}
	reg := installSysFilesRead(t)

	dir := t.TempDir()
	resp := bridge.Stat(reg)(context.Background(), &v2pb.StatRequest{Path: dir})
	if resp.GetError() != "" {
		t.Fatalf("stat err: %s", resp.GetError())
	}
	if resp.GetEntry().GetMode()&0o040000 == 0 {
		t.Errorf("directory should have dir bit set; mode = %o", resp.GetEntry().GetMode())
	}
}

// TestFilesRead_Read_StreamRoundTrip: STREAM_TYPE_FILE_READ pipes the
// header (size + mode) followed by FileChunk frames; the last one
// carries eof=true. Asserts both the header and the byte content
// reassembled across chunks match the on-disk file verbatim.
func TestFilesRead_Read_StreamRoundTrip(t *testing.T) {
	reg := installSysFilesRead(t)

	dir := t.TempDir()
	target := filepath.Join(dir, "payload.bin")
	// 600 KB so we exercise the multi-chunk path (FILE_CHUNK_SIZE
	// is 256 KB inside the wasm — three chunks total here).
	payload := bytes.Repeat([]byte("xy"), 300*1024)
	mustWrite(t, target, payload)

	streamA, streamB := net.Pipe()
	defer streamA.Close()
	defer streamB.Close()

	meta, err := proto.Marshal(&v2pb.FileReadRequest{Path: target})
	if err != nil {
		t.Fatal(err)
	}

	dispatchDone := make(chan error, 1)
	go func() {
		_, err := reg.DispatchStream(context.Background(),
			v2pb.StreamType_STREAM_TYPE_FILE_READ, streamA, meta)
		dispatchDone <- err
	}()

	// Header: size + mode (no error).
	var hdr v2pb.FileReadResponse
	if err := link.ReadFrame(streamB, &hdr); err != nil {
		t.Fatalf("read header: %v", err)
	}
	if hdr.GetError() != "" {
		t.Fatalf("header error: %s", hdr.GetError())
	}
	if hdr.GetSize() != int64(len(payload)) {
		t.Errorf("header size = %d; want %d", hdr.GetSize(), len(payload))
	}

	// Drain chunks until eof.
	var got bytes.Buffer
	deadline := time.Now().Add(15 * time.Second)
	sawEOF := false
	for time.Now().Before(deadline) && !sawEOF {
		var c v2pb.FileChunk
		if err := link.ReadFrame(streamB, &c); err != nil {
			t.Fatalf("read chunk: %v", err)
		}
		if c.GetError() != "" {
			t.Fatalf("chunk error: %s", c.GetError())
		}
		got.Write(c.GetData())
		if c.GetEof() {
			sawEOF = true
		}
	}
	if !sawEOF {
		t.Fatal("never saw eof chunk")
	}
	if !bytes.Equal(got.Bytes(), payload) {
		t.Errorf("reassembled %d bytes != source %d bytes", got.Len(), len(payload))
	}

	select {
	case err := <-dispatchDone:
		if err != nil {
			t.Errorf("dispatch err: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("DispatchStream did not return")
	}
}

// TestFilesRead_Scan_CountsTreeRoots walks a small fixture tree and
// asserts the FileScanResponse aggregates files / dirs / bytes
// correctly across nested directories.
func TestFilesRead_Scan_CountsTreeRoots(t *testing.T) {
	reg := installSysFilesRead(t)

	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.txt"), []byte("aaaa"))      // 4 bytes
	mustWrite(t, filepath.Join(root, "b.txt"), []byte("bbbbbb"))    // 6 bytes
	sub := filepath.Join(root, "sub")
	mustMkdir(t, sub)
	mustWrite(t, filepath.Join(sub, "c.txt"), []byte("cccccccccc")) // 10 bytes

	streamA, streamB := net.Pipe()
	defer streamA.Close()
	defer streamB.Close()

	meta, err := proto.Marshal(&v2pb.FileScanRequest{Paths: []string{root}})
	if err != nil {
		t.Fatal(err)
	}
	dispatchDone := make(chan error, 1)
	go func() {
		_, err := reg.DispatchStream(context.Background(),
			v2pb.StreamType_STREAM_TYPE_FILE_SCAN, streamA, meta)
		dispatchDone <- err
	}()

	var resp v2pb.FileScanResponse
	if err := link.ReadFrame(streamB, &resp); err != nil {
		t.Fatalf("read scan resp: %v", err)
	}
	if resp.GetError() != "" {
		t.Fatalf("scan error: %s", resp.GetError())
	}
	if resp.GetFileCount() != 3 {
		t.Errorf("file_count = %d; want 3", resp.GetFileCount())
	}
	// Whether the root dir is counted depends on convention. We
	// require AT LEAST 1 (the `sub` subdir); the root itself may or
	// may not be in the tally. Assert the lower bound to keep the
	// test robust against either choice.
	if resp.GetDirCount() < 1 {
		t.Errorf("dir_count = %d; want >= 1", resp.GetDirCount())
	}
	if resp.GetTotalBytes() != 20 {
		t.Errorf("total_bytes = %d; want 20", resp.GetTotalBytes())
	}

	select {
	case <-dispatchDone:
	case <-time.After(5 * time.Second):
		t.Fatal("DispatchStream did not return")
	}
}

// TestFilesRead_Archive_TarRoundTrip writes a small directory, asks
// the plugin to archive it as TAR, reassembles the FileChunk frames
// into a buffer, and re-parses with archive/tar. The test verifies
// every original file is present in the archive with byte-identical
// content. Catches the kind of memory-bound corruption that bit
// downloads on the 32 MB cap (see fix(plugins): bump max_memory_mb).
func TestFilesRead_Archive_TarRoundTrip(t *testing.T) {
	reg := installSysFilesRead(t)

	root := t.TempDir()
	files := map[string][]byte{
		"alpha.txt":     []byte("hello alpha"),
		"sub/beta.bin":  bytes.Repeat([]byte{0x42}, 1024),
		"sub/gamma.dat": []byte("gamma payload"),
	}
	for rel, content := range files {
		full := filepath.Join(root, rel)
		mustMkdir(t, filepath.Dir(full))
		mustWrite(t, full, content)
	}

	got := drainArchive(t, reg, &v2pb.FileArchiveRequest{
		Paths:  []string{root},
		Format: v2pb.ArchiveFormat_ARCHIVE_FORMAT_TAR,
	})
	tr := tar.NewReader(bytes.NewReader(got))
	seen := map[string][]byte{}
	for {
		hdr, err := tr.Next()
		if err != nil {
			break
		}
		if hdr.Typeflag == tar.TypeDir {
			continue
		}
		var buf bytes.Buffer
		if _, err := buf.ReadFrom(tr); err != nil {
			t.Fatalf("tar payload: %v", err)
		}
		seen[filepath.Base(hdr.Name)] = buf.Bytes()
	}
	for rel, want := range files {
		base := filepath.Base(rel)
		got, ok := seen[base]
		if !ok {
			t.Errorf("missing entry %s in archive (saw %v)", base, keys(seen))
			continue
		}
		if !bytes.Equal(got, want) {
			t.Errorf("entry %s payload mismatch: got %d bytes, want %d", base, len(got), len(want))
		}
	}
}

// TestFilesRead_Archive_TarGzRoundTrip is the gzip variant. Verifies
// the bytes round-trip through a real gzip + tar reader, exercising
// the GzEncoder branch in the wasm implementation.
func TestFilesRead_Archive_TarGzRoundTrip(t *testing.T) {
	reg := installSysFilesRead(t)

	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "single.txt"), []byte("one file"))

	got := drainArchive(t, reg, &v2pb.FileArchiveRequest{
		Paths:            []string{root},
		Format:           v2pb.ArchiveFormat_ARCHIVE_FORMAT_TAR_GZ,
		CompressionLevel: 6,
	})
	gzr, err := gzip.NewReader(bytes.NewReader(got))
	if err != nil {
		t.Fatalf("gzip: %v", err)
	}
	defer gzr.Close()
	tr := tar.NewReader(gzr)
	// First entry might be the root dir, the file might not be first.
	// Walk to find single.txt + verify its body round-trips through the
	// gzip layer.
	found := false
	for {
		hdr, err := tr.Next()
		if err != nil {
			break
		}
		if !strings.HasSuffix(hdr.Name, "single.txt") {
			continue
		}
		var buf bytes.Buffer
		if _, err := buf.ReadFrom(tr); err != nil {
			t.Fatalf("tar payload: %v", err)
		}
		if buf.String() != "one file" {
			t.Errorf("payload = %q; want %q", buf.String(), "one file")
		}
		found = true
		break
	}
	if !found {
		t.Error("single.txt not present in tar.gz output")
	}
}

// TestFilesRead_Archive_ZipUnsupported: the merged plugin still
// declares ZIP as out-of-scope (TAR_GZ covers the desktop UX). The
// agent must reject ZIP cleanly rather than silently producing an
// empty file.
func TestFilesRead_Archive_ZipUnsupported(t *testing.T) {
	reg := installSysFilesRead(t)
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "x.txt"), []byte("x"))

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

	// Read the header (FileArchiveResponse) — error string must
	// mention zip / unsupported.
	var hdr v2pb.FileArchiveResponse
	if err := link.ReadFrame(streamB, &hdr); err != nil {
		t.Fatalf("read header: %v", err)
	}
	if hdr.GetError() == "" || !strings.Contains(strings.ToLower(hdr.GetError()), "zip") {
		t.Errorf("error = %q; want to mention zip / unsupported", hdr.GetError())
	}
	// Drain the remaining (likely empty) chunk so dispatch returns.
	for {
		var c v2pb.FileChunk
		if err := link.ReadFrame(streamB, &c); err != nil {
			break
		}
		if c.GetEof() {
			break
		}
	}
	select {
	case <-dispatchDone:
	case <-time.After(5 * time.Second):
		t.Fatal("DispatchStream did not return")
	}
}

// drainArchive runs an archive request and reassembles every
// FileChunk into one buffer. Asserts no chunk-level error;
// fails the test outright on the first one. Returns the concatenated
// payload (TAR or TAR.GZ bytes depending on req.Format).
func drainArchive(t *testing.T, reg *plugin.Registry, req *v2pb.FileArchiveRequest) []byte {
	t.Helper()
	streamA, streamB := net.Pipe()
	defer streamA.Close()
	defer streamB.Close()
	meta, err := proto.Marshal(req)
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
		t.Fatalf("read archive header: %v", err)
	}
	if hdr.GetError() != "" {
		t.Fatalf("archive error: %s", hdr.GetError())
	}
	var got bytes.Buffer
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		var c v2pb.FileChunk
		if err := link.ReadFrame(streamB, &c); err != nil {
			t.Fatalf("read chunk: %v", err)
		}
		if c.GetError() != "" {
			t.Fatalf("chunk error: %s", c.GetError())
		}
		got.Write(c.GetData())
		if c.GetEof() {
			select {
			case <-dispatchDone:
			case <-time.After(5 * time.Second):
				t.Fatal("DispatchStream did not return after eof")
			}
			return got.Bytes()
		}
	}
	t.Fatal("never saw eof chunk")
	return nil
}

func sysFilesReadWasmPath() string {
	return filepath.Join("..", "..", "..", "example", "plugins", "system", "sys-files-read",
		"target", "wasm32-unknown-unknown", "release", "sys_files_read.wasm")
}

func mustWrite(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func keys(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
