package plugin

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/WangYihang/Platypus/internal/link"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// TestProcessOpen_RustPluginNonPty drives sys-process through
// the legacy-wasm bridge against a fixed `/bin/echo hello` invocation.
// Asserts the ProcessOpenResponse carries a non-zero pid, that we
// receive `hello\n` on stdout, and that the final ProcessFrame.exit
// arrives with code 0. PTY mode is exercised by the legacy
// process_stream tests already + needs creack/pty + a real terminal,
// which jsdom-style test environments don't always have.
func TestProcessOpen_RustPluginNonPty(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("/bin/echo not available on windows")
	}
	if _, err := os.Stat("/bin/echo"); err != nil {
		t.Skipf("/bin/echo not present: %v", err)
	}

	wasmPath := processOpenWasmPath()
	wasm, err := os.ReadFile(wasmPath)
	if err != nil {
		t.Skipf("sys_process_open.wasm not built (%v) — run `cargo build --release --target wasm32-unknown-unknown` in example/plugins/sys-process/", err)
	}
	manifestBytes, err := os.ReadFile(filepath.Join("..", "..", "..",
		"example", "plugins", "sys-process", "plugin.yaml"))
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
	sig, err := Sign(sk, wasm, DefaultTrustedComment("sys_process_open.wasm"))
	if err != nil {
		t.Fatal(err)
	}

	reg, err := New(Options{Paths: paths})
	if err != nil {
		t.Fatal(err)
	}
	defer reg.Close(context.Background())

	if err := reg.InstallFromBytes(context.Background(), InstallParams{
		PluginID:            "com.platypus.sys-process",
		Version:             "1.0.0",
		PublisherPubkey:     []byte(EncodePublicKey(pk, "")),
		Manifest:            []byte(manifestStr),
		Wasm:                wasm,
		Signature:           []byte(EncodeSignature(sig)),
		Actor:               "test",
		GrantedCapabilities: []string{"process"},
	}, nil); err != nil {
		t.Fatal(err)
	}

	streamA, streamB := net.Pipe()
	defer streamA.Close()
	defer streamB.Close()

	meta, err := proto.Marshal(&v2pb.ProcessOpenRequest{
		Command: "/bin/echo",
		Args:    []string{"hello"},
		Pty:     false,
	})
	if err != nil {
		t.Fatal(err)
	}

	dispatchDone := make(chan error, 1)
	go func() {
		_, err := reg.DispatchStream(context.Background(),
			v2pb.StreamType_STREAM_TYPE_PROCESS_OPEN, streamA, meta)
		dispatchDone <- err
	}()

	// First frame: ProcessOpenResponse with pid + no error.
	var openResp v2pb.ProcessOpenResponse
	if err := link.ReadFrame(streamB, &openResp); err != nil {
		t.Fatalf("read open response: %v", err)
	}
	if openResp.GetError() != "" {
		t.Fatalf("open error: %s", openResp.GetError())
	}
	if openResp.GetPid() == 0 {
		t.Errorf("pid = 0, want non-zero")
	}

	// Subsequent frames: ProcessFrame.stdout (one or more) then a
	// terminal ProcessFrame.exit.
	var stdout []byte
	deadline := time.Now().Add(10 * time.Second)
	sawExit := false
	for time.Now().Before(deadline) && !sawExit {
		var f v2pb.ProcessFrame
		if err := link.ReadFrame(streamB, &f); err != nil {
			t.Fatalf("read frame: %v", err)
		}
		switch p := f.Payload.(type) {
		case *v2pb.ProcessFrame_Stdout:
			stdout = append(stdout, p.Stdout...)
		case *v2pb.ProcessFrame_Stderr:
			t.Logf("stderr: %s", p.Stderr)
		case *v2pb.ProcessFrame_Exit:
			if p.Exit.GetCode() != 0 {
				t.Errorf("exit code = %d, want 0", p.Exit.GetCode())
			}
			sawExit = true
		}
	}
	if !sawExit {
		t.Fatal("never saw exit frame")
	}
	if !strings.Contains(string(stdout), "hello") {
		t.Errorf("stdout = %q, want to contain 'hello'", stdout)
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

// TestProcessOpen_DeniesUnlistedCommand exercises the policy
// boundary: a manifest that claims `process` with a narrower
// allowlist must reject any spawn outside the list. We override the
// shipped manifest's `commands: ["*"]` to a literal /bin/true and
// verify that requesting /bin/false comes back as an error.
func TestProcessOpen_DeniesUnlistedCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("/bin/false not available on windows")
	}
	wasmPath := processOpenWasmPath()
	wasm, err := os.ReadFile(wasmPath)
	if err != nil {
		t.Skipf("sys_process_open.wasm not built (%v)", err)
	}
	manifestBytes, err := os.ReadFile(filepath.Join("..", "..", "..",
		"example", "plugins", "sys-process", "plugin.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	pluginRoot := t.TempDir()
	paths := NewPaths(pluginRoot)
	sk, pk, err := GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
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
	// Narrow the wildcard to /bin/true. Any other binary should be
	// rejected with command_not_in_allowlist.
	manifestStr = strings.Replace(manifestStr, `commands: ["*"]`,
		`commands: ["/bin/true"]`, 1)
	sig, err := Sign(sk, wasm, DefaultTrustedComment("sys_process_open.wasm"))
	if err != nil {
		t.Fatal(err)
	}

	reg, err := New(Options{Paths: paths})
	if err != nil {
		t.Fatal(err)
	}
	defer reg.Close(context.Background())

	if err := reg.InstallFromBytes(context.Background(), InstallParams{
		PluginID:            "com.platypus.sys-process",
		Version:             "1.0.0",
		PublisherPubkey:     []byte(EncodePublicKey(pk, "")),
		Manifest:            []byte(manifestStr),
		Wasm:                wasm,
		Signature:           []byte(EncodeSignature(sig)),
		Actor:               "test",
		GrantedCapabilities: []string{"process"},
	}, nil); err != nil {
		t.Fatal(err)
	}

	streamA, streamB := net.Pipe()
	defer streamA.Close()
	defer streamB.Close()

	meta, err := proto.Marshal(&v2pb.ProcessOpenRequest{
		Command: "/bin/false", // NOT in the narrowed allowlist
		Pty:     false,
	})
	if err != nil {
		t.Fatal(err)
	}

	dispatchDone := make(chan error, 1)
	go func() {
		_, err := reg.DispatchStream(context.Background(),
			v2pb.StreamType_STREAM_TYPE_PROCESS_OPEN, streamA, meta)
		dispatchDone <- err
	}()

	var openResp v2pb.ProcessOpenResponse
	if err := link.ReadFrame(streamB, &openResp); err != nil {
		t.Fatalf("read open response: %v", err)
	}
	if !strings.Contains(strings.ToLower(openResp.GetError()), "allowlist") {
		t.Errorf("error = %q, want to mention allowlist", openResp.GetError())
	}

	select {
	case <-dispatchDone:
	case <-time.After(5 * time.Second):
		t.Fatal("DispatchStream did not return")
	}
}

func processOpenWasmPath() string {
	return filepath.Join("..", "..", "..", "example", "plugins", "sys-process",
		"target", "wasm32-unknown-unknown", "release", "sys_process_open.wasm")
}
