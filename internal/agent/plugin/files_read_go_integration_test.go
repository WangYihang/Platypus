package plugin_test

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/WangYihang/Platypus/internal/agent/plugin"
	"github.com/WangYihang/Platypus/internal/link"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

func installSysFilesReadGo(t *testing.T) *plugin.Registry {
	t.Helper()
	wasm := stagedWasmBytes(t, "com.platypus.sys-files-read-go", "1.0.0", "sys_files_read.wasm")
	manifestBytes := stagedManifestBytes(t, "com.platypus.sys-files-read-go", "1.0.0")

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
		PluginID:            "com.platypus.sys-files-read-go",
		Version:             "1.0.0",
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

func TestFilesReadGo_ListDir(t *testing.T) {
	reg := installSysFilesReadGo(t)
	dir := t.TempDir()
	for _, name := range []string{"a.txt", "b.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}

	body, _ := json.Marshal(struct {
		Path string `json:"path"`
	}{Path: dir})
	resp := reg.Invoke(context.Background(), &v2pb.PluginCallRequest{
		PluginId: "com.platypus.sys-files-read-go",
		Method:   "list_dir",
		Payload:  body,
	})
	if resp.GetError() != "" {
		t.Fatalf("list_dir error: %s", resp.GetError())
	}
	var out struct {
		Entries []struct {
			Name string `json:"name"`
			Mode uint32 `json:"mode"`
		} `json:"entries"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(resp.GetPayload(), &out); err != nil {
		t.Fatalf("unmarshal: %v\npayload: %s", err, resp.GetPayload())
	}
	if out.Error != "" {
		t.Fatalf("plugin error: %s", out.Error)
	}
	if len(out.Entries) != 3 {
		t.Errorf("entries = %d, want 3 (a.txt, b.txt, sub): %+v", len(out.Entries), out.Entries)
	}
}

func TestFilesReadGo_Stat_File(t *testing.T) {
	reg := installSysFilesReadGo(t)
	dir := t.TempDir()
	target := filepath.Join(dir, "f.txt")
	content := []byte("hello world")
	if err := os.WriteFile(target, content, 0o644); err != nil {
		t.Fatal(err)
	}

	body, _ := json.Marshal(struct {
		Path string `json:"path"`
	}{Path: target})
	resp := reg.Invoke(context.Background(), &v2pb.PluginCallRequest{
		PluginId: "com.platypus.sys-files-read-go",
		Method:   "stat",
		Payload:  body,
	})
	if resp.GetError() != "" {
		t.Fatalf("stat error: %s", resp.GetError())
	}
	var out struct {
		Entry *struct {
			Name string `json:"name"`
			Size int64  `json:"size"`
		} `json:"entry"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(resp.GetPayload(), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Error != "" {
		t.Fatalf("plugin error: %s", out.Error)
	}
	if out.Entry == nil {
		t.Fatalf("entry is nil")
	}
	if out.Entry.Size != int64(len(content)) {
		t.Errorf("size = %d, want %d", out.Entry.Size, len(content))
	}
}

func TestFilesReadGo_Read_StreamRoundTrip(t *testing.T) {
	reg := installSysFilesReadGo(t)
	dir := t.TempDir()
	target := filepath.Join(dir, "f.txt")
	content := bytes.Repeat([]byte("hello\n"), 100)
	if err := os.WriteFile(target, content, 0o644); err != nil {
		t.Fatal(err)
	}

	streamA, streamB := net.Pipe()
	defer streamA.Close()
	defer streamB.Close()

	meta, err := proto.Marshal(&v2pb.FileReadRequest{Path: target})
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_, _ = reg.DispatchStream(context.Background(),
			v2pb.StreamType_STREAM_TYPE_FILE_READ, streamA, meta)
	}()

	var hdr v2pb.FileReadResponse
	if err := link.ReadFrame(streamB, &hdr); err != nil {
		t.Fatalf("read header: %v", err)
	}
	if hdr.GetError() != "" {
		t.Fatalf("header error: %s", hdr.GetError())
	}
	if hdr.GetSize() != int64(len(content)) {
		t.Errorf("size = %d, want %d", hdr.GetSize(), len(content))
	}

	if err := streamB.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	var got []byte
	for {
		var ch v2pb.FileChunk
		if err := link.ReadFrame(streamB, &ch); err != nil {
			t.Fatalf("read chunk: %v", err)
		}
		got = append(got, ch.GetData()...)
		if ch.GetEof() {
			break
		}
	}
	if !bytes.Equal(got, content) {
		t.Errorf("content mismatch: got %d bytes, want %d", len(got), len(content))
	}
}

func TestFilesReadGo_Scan_CountsTreeRoots(t *testing.T) {
	reg := installSysFilesReadGo(t)
	dir := t.TempDir()
	// Create 3 files + a sub with 2 more files.
	for _, name := range []string{"a", "b", "c"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"x", "y"} {
		if err := os.WriteFile(filepath.Join(sub, name), []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	streamA, streamB := net.Pipe()
	defer streamA.Close()
	defer streamB.Close()

	meta, err := proto.Marshal(&v2pb.FileScanRequest{Paths: []string{dir}})
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_, _ = reg.DispatchStream(context.Background(),
			v2pb.StreamType_STREAM_TYPE_FILE_SCAN, streamA, meta)
	}()

	var resp v2pb.FileScanResponse
	if err := streamB.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := link.ReadFrame(streamB, &resp); err != nil {
		t.Fatalf("read scan response: %v", err)
	}
	if resp.GetError() != "" {
		t.Fatalf("scan error: %s", resp.GetError())
	}
	if resp.GetFileCount() != 5 {
		t.Errorf("file_count = %d, want 5", resp.GetFileCount())
	}
	// The walker counts the root directory + the sub directory.
	if resp.GetDirCount() < 1 {
		t.Errorf("dir_count = %d, want ≥ 1", resp.GetDirCount())
	}
}

func TestFilesReadGo_Archive_TarRoundTrip(t *testing.T) {
	reg := installSysFilesReadGo(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("greetings"), 0o644); err != nil {
		t.Fatal(err)
	}

	streamA, streamB := net.Pipe()
	defer streamA.Close()
	defer streamB.Close()

	meta, err := proto.Marshal(&v2pb.FileArchiveRequest{
		Paths:  []string{dir},
		Format: v2pb.ArchiveFormat_ARCHIVE_FORMAT_TAR,
	})
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_, _ = reg.DispatchStream(context.Background(),
			v2pb.StreamType_STREAM_TYPE_FILE_ARCHIVE, streamA, meta)
	}()

	if err := streamB.SetReadDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.Fatal(err)
	}
	// First frame: FileReadResponse header.
	var hdr v2pb.FileReadResponse
	if err := link.ReadFrame(streamB, &hdr); err != nil {
		t.Fatalf("read header: %v", err)
	}
	if hdr.GetError() != "" {
		t.Fatalf("header error: %s", hdr.GetError())
	}

	// Subsequent frames: FileChunk frames with archive bytes; final
	// frame has eof=true.
	var archive []byte
	for {
		var ch v2pb.FileChunk
		if err := link.ReadFrame(streamB, &ch); err != nil {
			t.Fatalf("read chunk: %v", err)
		}
		archive = append(archive, ch.GetData()...)
		if ch.GetEof() {
			if ch.GetError() != "" {
				t.Errorf("terminal error: %s", ch.GetError())
			}
			break
		}
	}

	// Decode the tar; verify hello.txt is present with the right content.
	tr := tar.NewReader(bytes.NewReader(archive))
	found := false
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar next: %v", err)
		}
		if filepath.Base(h.Name) == "hello.txt" {
			body, _ := io.ReadAll(tr)
			if string(body) != "greetings" {
				t.Errorf("hello.txt body = %q", body)
			}
			found = true
		}
	}
	if !found {
		t.Errorf("hello.txt missing from archive")
	}
}
