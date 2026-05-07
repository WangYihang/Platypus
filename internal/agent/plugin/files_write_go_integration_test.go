package plugin_test

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/WangYihang/Platypus/internal/agent/plugin"
	"github.com/WangYihang/Platypus/internal/link"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

func installSysFilesWriteGo(t *testing.T) *plugin.Registry {
	t.Helper()
	wasm := stagedWasmBytes(t, "com.platypus.sys-files-write-go", "1.0.0", "sys_files_write.wasm")
	manifestBytes := stagedManifestBytes(t, "com.platypus.sys-files-write-go", "1.0.0")

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
		PluginID:            "com.platypus.sys-files-write-go",
		Version:             "1.0.0",
		PublisherPubkey:     []byte(plugin.EncodePublicKey(pk, "")),
		Manifest:            []byte(manifestStr),
		Wasm:                wasm,
		Signature:           []byte(plugin.EncodeSignature(sig)),
		Actor:               "test",
		GrantedCapabilities: []plugin.CapabilityID{"fs.write"},
	}, nil); err != nil {
		t.Fatal(err)
	}
	return reg
}

// invokeGoFilesWriteRPC invokes one of the sys-files-write-go RPCs
// (mkdir / chmod / delete / rename) with a snake_case JSON body and
// returns the unmarshalled error envelope.
func invokeGoFilesWriteRPC(t *testing.T, reg *plugin.Registry, method string, body []byte) string {
	t.Helper()
	resp := reg.Invoke(context.Background(), &v2pb.PluginCallRequest{
		PluginId: "com.platypus.sys-files-write-go",
		Method:   method,
		Payload:  body,
	})
	if resp.GetError() != "" {
		return resp.GetError()
	}
	var out struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(resp.GetPayload(), &out); err != nil {
		t.Fatalf("unmarshal: %v\npayload: %s", err, resp.GetPayload())
	}
	return out.Error
}

func TestFilesWriteGo_Mkdir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX mode is unix-only")
	}
	reg := installSysFilesWriteGo(t)
	target := filepath.Join(t.TempDir(), "newdir")
	body, _ := json.Marshal(struct {
		Path string `json:"path"`
		Mode uint32 `json:"mode"`
	}{Path: target, Mode: 0o755})
	if errMsg := invokeGoFilesWriteRPC(t, reg, "mkdir", body); errMsg != "" {
		t.Fatalf("mkdir: %s", errMsg)
	}
	st, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if !st.IsDir() {
		t.Errorf("expected directory at %s", target)
	}
}

func TestFilesWriteGo_Mkdir_Recursive(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX mode is unix-only")
	}
	reg := installSysFilesWriteGo(t)
	target := filepath.Join(t.TempDir(), "a", "b", "c")
	body, _ := json.Marshal(struct {
		Path   string `json:"path"`
		Mode   uint32 `json:"mode"`
		Mkdirs bool   `json:"mkdirs"`
	}{Path: target, Mode: 0o755, Mkdirs: true})
	if errMsg := invokeGoFilesWriteRPC(t, reg, "mkdir", body); errMsg != "" {
		t.Fatalf("mkdir: %s", errMsg)
	}
	if _, err := os.Stat(target); err != nil {
		t.Errorf("recursive mkdir did not create %s: %v", target, err)
	}
}

func TestFilesWriteGo_Chmod(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod is unix-only")
	}
	reg := installSysFilesWriteGo(t)
	dir := t.TempDir()
	target := filepath.Join(dir, "f")
	if err := os.WriteFile(target, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(struct {
		Path string `json:"path"`
		Mode uint32 `json:"mode"`
	}{Path: target, Mode: 0o600})
	if errMsg := invokeGoFilesWriteRPC(t, reg, "chmod", body); errMsg != "" {
		t.Fatalf("chmod: %s", errMsg)
	}
	st, _ := os.Stat(target)
	if got := st.Mode().Perm(); got != 0o600 {
		t.Errorf("mode after chmod = %o; want 0600", got)
	}
}

func TestFilesWriteGo_Delete_File(t *testing.T) {
	reg := installSysFilesWriteGo(t)
	dir := t.TempDir()
	target := filepath.Join(dir, "f")
	if err := os.WriteFile(target, []byte("bye"), 0o644); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(struct {
		Path string `json:"path"`
	}{Path: target})
	if errMsg := invokeGoFilesWriteRPC(t, reg, "delete", body); errMsg != "" {
		t.Fatalf("delete: %s", errMsg)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Errorf("file still present after delete: %v", err)
	}
}

func TestFilesWriteGo_Rename(t *testing.T) {
	reg := installSysFilesWriteGo(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "from")
	dst := filepath.Join(dir, "to")
	if err := os.WriteFile(src, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(struct {
		From string `json:"from"`
		To   string `json:"to"`
	}{From: src, To: dst})
	if errMsg := invokeGoFilesWriteRPC(t, reg, "rename", body); errMsg != "" {
		t.Fatalf("rename: %s", errMsg)
	}
	if _, err := os.Stat(dst); err != nil {
		t.Errorf("dst missing after rename: %v", err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Errorf("src still present: %v", err)
	}
}

// TestFilesWriteGo_StreamWrite drives the streaming `write` entry
// against a freshly-created file.  Mirrors the Rust crate's
// TestFilesWrite_Write_StreamRoundTrip.
func TestFilesWriteGo_StreamWrite(t *testing.T) {
	reg := installSysFilesWriteGo(t)
	dir := t.TempDir()
	target := filepath.Join(dir, "out.bin")

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

	// First frame back: FileWriteResponse with no error.
	var ack v2pb.FileWriteResponse
	if err := link.ReadFrame(streamB, &ack); err != nil {
		t.Fatalf("read ack: %v", err)
	}
	if ack.GetError() != "" {
		t.Fatalf("ack error: %s", ack.GetError())
	}

	// Send a chunk + EOF.
	payload := []byte("hello from go plugin\n")
	if err := link.WriteFrame(streamB, &v2pb.FileChunk{Data: payload}); err != nil {
		t.Fatal(err)
	}
	if err := link.WriteFrame(streamB, &v2pb.FileChunk{Eof: true}); err != nil {
		t.Fatal(err)
	}

	// Final frame: FileWriteResult with bytes_written.
	var result v2pb.FileWriteResult
	if err := streamB.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := link.ReadFrame(streamB, &result); err != nil {
		t.Fatalf("read result: %v", err)
	}
	if result.GetError() != "" {
		t.Fatalf("result error: %s", result.GetError())
	}
	if result.GetBytesWritten() != int64(len(payload)) {
		t.Errorf("bytes_written = %d, want %d", result.GetBytesWritten(), len(payload))
	}

	streamB.Close()
	select {
	case err := <-dispatchDone:
		if err != nil {
			t.Logf("dispatch err: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("DispatchStream did not return")
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(got) != string(payload) {
		t.Errorf("file content = %q, want %q", got, payload)
	}
}
