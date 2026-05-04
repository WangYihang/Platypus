package plugin_test

import (
	"bytes"
	"context"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/WangYihang/Platypus/internal/agent/plugin"
	"github.com/WangYihang/Platypus/internal/agent/plugin/bridge"
	"github.com/WangYihang/Platypus/internal/link"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// installSysFilesWrite wires the freshly-built sys-files-write wasm
// into a fresh registry with the fs.write capability granted. Same
// pattern as installSysFilesRead.
func installSysFilesWrite(t *testing.T) *plugin.Registry {
	t.Helper()
	wasm := stagedWasmBytes(t, "com.platypus.sys-files-write", "1.0.0", "sys_files_write.wasm")
	manifestBytes := stagedManifestBytes(t, "com.platypus.sys-files-write", "1.0.0")

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
	manifestStr := rewriteManifestKeyID(string(manifestBytes), plugin.HumanKeyID(pk))
	sig, err := plugin.Sign(sk, wasm, plugin.DefaultTrustedComment("sys_files_write.wasm"))
	if err != nil {
		t.Fatal(err)
	}
	reg, err := plugin.New(plugin.Options{Paths: paths})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { reg.Close(context.Background()) })

	if err := reg.InstallFromBytes(context.Background(), plugin.InstallParams{
		PluginID:            "com.platypus.sys-files-write",
		Version:             "1.0.0",
		PublisherPubkey:     []byte(plugin.EncodePublicKey(pk, "")),
		Manifest:            []byte(manifestStr),
		Wasm:                wasm,
		Signature:           []byte(plugin.EncodeSignature(sig)),
		Actor:               "test",
		GrantedCapabilities: []string{"fs.write"},
	}, nil); err != nil {
		t.Fatal(err)
	}
	return reg
}

// TestFilesWrite_Mkdir creates a fresh directory and asserts the
// host actually made it on disk.
func TestFilesWrite_Mkdir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX mode behaviour is unix-only")
	}
	reg := installSysFilesWrite(t)
	root := t.TempDir()
	target := filepath.Join(root, "newdir")

	resp := bridge.Mkdir(reg)(context.Background(), &v2pb.MkdirRequest{
		Path: target,
		Mode: 0o755,
	})
	if resp.GetError() != "" {
		t.Fatalf("mkdir error: %s", resp.GetError())
	}
	st, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat after mkdir: %v", err)
	}
	if !st.IsDir() {
		t.Errorf("expected directory at %s", target)
	}
}

// TestFilesWrite_Mkdir_RecursiveCreatesParents covers the mkdirs=true
// branch — a multi-segment path should land even when only the
// closest existing ancestor is the project root.
func TestFilesWrite_Mkdir_RecursiveCreatesParents(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX mode is unix-only")
	}
	reg := installSysFilesWrite(t)
	root := t.TempDir()
	target := filepath.Join(root, "a", "b", "c")

	resp := bridge.Mkdir(reg)(context.Background(), &v2pb.MkdirRequest{
		Path:   target,
		Mode:   0o755,
		Mkdirs: true,
	})
	if resp.GetError() != "" {
		t.Fatalf("mkdir error: %s", resp.GetError())
	}
	if _, err := os.Stat(target); err != nil {
		t.Errorf("recursive mkdir did not create %s: %v", target, err)
	}
}

// TestFilesWrite_Chmod toggles a file's mode and verifies the OS
// stat reflects the change. Skip on Windows where Go's chmod
// semantics are limited (read-only bit only).
func TestFilesWrite_Chmod(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod semantics are unix-only")
	}
	reg := installSysFilesWrite(t)
	dir := t.TempDir()
	target := filepath.Join(dir, "f")
	mustWrite(t, target, []byte("data"))
	// Start at 0644, flip to 0600.
	if err := os.Chmod(target, 0o644); err != nil {
		t.Fatal(err)
	}

	resp := bridge.Chmod(reg)(context.Background(), &v2pb.ChmodRequest{
		Path: target,
		Mode: 0o600,
	})
	if resp.GetError() != "" {
		t.Fatalf("chmod error: %s", resp.GetError())
	}
	st, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if got := st.Mode().Perm(); got != 0o600 {
		t.Errorf("mode after chmod = %o; want 0600", got)
	}
}

// TestFilesWrite_Delete_File: a regular file disappears.
func TestFilesWrite_Delete_File(t *testing.T) {
	reg := installSysFilesWrite(t)
	dir := t.TempDir()
	target := filepath.Join(dir, "f")
	mustWrite(t, target, []byte("bye"))

	resp := bridge.Delete(reg)(context.Background(), &v2pb.DeleteRequest{Path: target})
	if resp.GetError() != "" {
		t.Fatalf("delete error: %s", resp.GetError())
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Errorf("file still present after delete: %v", err)
	}
}

// TestFilesWrite_Delete_RecursiveDirectory: recursive=true wipes a
// non-empty directory; recursive=false on the same input would
// fail. We exercise the success path here; the failure path is
// covered by the OS-level rmdir semantics (already tested by Go's
// standard library).
func TestFilesWrite_Delete_RecursiveDirectory(t *testing.T) {
	reg := installSysFilesWrite(t)
	dir := t.TempDir()
	target := filepath.Join(dir, "tree")
	mustMkdir(t, filepath.Join(target, "sub"))
	mustWrite(t, filepath.Join(target, "sub", "x"), []byte("x"))

	resp := bridge.Delete(reg)(context.Background(), &v2pb.DeleteRequest{
		Path:      target,
		Recursive: true,
	})
	if resp.GetError() != "" {
		t.Fatalf("delete error: %s", resp.GetError())
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Errorf("tree still present: %v", err)
	}
}

// TestFilesWrite_Rename moves a file to a new name and verifies the
// rename took effect.
func TestFilesWrite_Rename(t *testing.T) {
	reg := installSysFilesWrite(t)
	dir := t.TempDir()
	from := filepath.Join(dir, "alpha")
	to := filepath.Join(dir, "beta")
	mustWrite(t, from, []byte("payload"))

	resp := bridge.Rename(reg)(context.Background(), &v2pb.RenameRequest{
		From: from, To: to,
	})
	if resp.GetError() != "" {
		t.Fatalf("rename error: %s", resp.GetError())
	}
	if _, err := os.Stat(from); !os.IsNotExist(err) {
		t.Errorf("source still present: %v", err)
	}
	got, err := os.ReadFile(to)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if string(got) != "payload" {
		t.Errorf("dest content = %q; want %q", got, "payload")
	}
}

// TestFilesWrite_Write_StreamRoundTrip drives the full upload
// streaming protocol:
//
//   1. metadata = FileWriteRequest{path, mkdirs}
//   2. agent acks with FileWriteResponse{error=""}
//   3. server pushes FileChunk{data, eof=true}
//   4. agent emits FileWriteResult{bytes_written, error=""}
//   5. on-disk content == what the server sent
//
// 64 KB payload is enough to verify the chunk push + that the agent
// wrote it correctly without venturing into the multi-chunk OOM
// territory (covered by sys-files-read's read test).
func TestFilesWrite_Write_StreamRoundTrip(t *testing.T) {
	reg := installSysFilesWrite(t)
	dir := t.TempDir()
	target := filepath.Join(dir, "out.bin")
	payload := bytes.Repeat([]byte{0x37}, 64*1024)

	streamA, streamB := net.Pipe()
	defer streamA.Close()
	defer streamB.Close()

	meta, err := proto.Marshal(&v2pb.FileWriteRequest{
		Path: target,
		Mode: 0o644,
	})
	if err != nil {
		t.Fatal(err)
	}

	dispatchDone := make(chan error, 1)
	go func() {
		_, err := reg.DispatchStream(context.Background(),
			v2pb.StreamType_STREAM_TYPE_FILE_WRITE, streamA, meta)
		dispatchDone <- err
	}()

	// Step 1: read the agent's ack.
	var ack v2pb.FileWriteResponse
	if err := link.ReadFrame(streamB, &ack); err != nil {
		t.Fatalf("read ack: %v", err)
	}
	if ack.GetError() != "" {
		t.Fatalf("write ack error: %s", ack.GetError())
	}

	// Step 2: push the payload as a single FileChunk with eof.
	if err := link.WriteFrame(streamB, &v2pb.FileChunk{
		Data: payload,
		Eof:  true,
	}); err != nil {
		t.Fatalf("push chunk: %v", err)
	}

	// Step 3: read the result frame.
	var result v2pb.FileWriteResult
	if err := link.ReadFrame(streamB, &result); err != nil {
		t.Fatalf("read result: %v", err)
	}
	if result.GetError() != "" {
		t.Errorf("result error: %s", result.GetError())
	}
	if result.GetBytesWritten() != int64(len(payload)) {
		t.Errorf("bytes_written = %d; want %d", result.GetBytesWritten(), len(payload))
	}

	// Step 4: verify on-disk bytes.
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Errorf("written bytes mismatch: %d on disk, %d sent", len(got), len(payload))
	}

	select {
	case <-dispatchDone:
	case <-time.After(5 * time.Second):
		t.Fatal("DispatchStream did not return")
	}
}

