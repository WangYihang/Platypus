package plugin_test

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/WangYihang/Platypus/internal/agent/plugin"
	"github.com/WangYihang/Platypus/internal/link"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// installSysProcessGo is the shared install fixture used by both
// the exec-RPC and the open-stream tests below.  Wires the staged
// TinyGo plugin into a fresh registry with both `exec` and
// `process` capabilities granted (matches manifest declarations).
func installSysProcessGo(t *testing.T) *plugin.Registry {
	t.Helper()
	wasm := stagedWasmBytes(t, "com.platypus.sys-process-go", "1.0.0", "sys_process.wasm")
	manifestBytes := stagedManifestBytes(t, "com.platypus.sys-process-go", "1.0.0")

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
	sig, err := plugin.Sign(sk, wasm, plugin.DefaultTrustedComment("sys_process.wasm"))
	if err != nil {
		t.Fatal(err)
	}
	reg, err := plugin.New(plugin.Options{Paths: paths})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { reg.Close(context.Background()) })

	if err := reg.InstallFromBytes(context.Background(), plugin.InstallParams{
		PluginID:            "com.platypus.sys-process-go",
		Version:             "1.0.0",
		PublisherPubkey:     []byte(plugin.EncodePublicKey(pk, "")),
		Manifest:            []byte(manifestStr),
		Wasm:                wasm,
		Signature:           []byte(plugin.EncodeSignature(sig)),
		Actor:               "test",
		GrantedCapabilities: []string{"exec", "process"},
	}, nil); err != nil {
		t.Fatal(err)
	}
	return reg
}

// TestProcessGo_ExecRPC drives the synchronous exec entry point
// (capability `exec`).  The Go plugin marshals the request straight
// through to host_exec; we assert stdout carries the expected
// command output.
func TestProcessGo_ExecRPC(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("/bin/echo not available on windows")
	}
	if _, err := os.Stat("/bin/echo"); err != nil {
		t.Skipf("/bin/echo not present: %v", err)
	}
	reg := installSysProcessGo(t)

	body, _ := json.Marshal(struct {
		Command string   `json:"command"`
		Args    []string `json:"args"`
	}{Command: "/bin/echo", Args: []string{"hello-go"}})

	resp := reg.Invoke(context.Background(), &v2pb.PluginCallRequest{
		PluginId: "com.platypus.sys-process-go",
		Method:   "exec",
		Payload:  body,
	})
	if resp.GetError() != "" {
		t.Fatalf("exec error: %s", resp.GetError())
	}
	var execResp struct {
		ExitCode int32  `json:"exit_code"`
		Stdout   string `json:"stdout"`
		Stderr   string `json:"stderr"`
		Error    string `json:"error"`
	}
	if err := json.Unmarshal(resp.GetPayload(), &execResp); err != nil {
		t.Fatalf("unmarshal: %v\npayload: %s", err, resp.GetPayload())
	}
	if execResp.Error != "" {
		t.Fatalf("plugin error: %s", execResp.Error)
	}
	if execResp.ExitCode != 0 {
		t.Errorf("exit_code = %d, want 0", execResp.ExitCode)
	}
	if !strings.Contains(execResp.Stdout, "hello-go") {
		t.Errorf("stdout = %q, want to contain 'hello-go'", execResp.Stdout)
	}
}

// TestProcessGo_OpenStream drives the streaming open entry point
// (capability `process`) against `/bin/echo hello`. Mirrors the
// Rust crate's TestProcessOpen_RustPluginNonPty.
func TestProcessGo_OpenStream(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("/bin/echo not available on windows")
	}
	if _, err := os.Stat("/bin/echo"); err != nil {
		t.Skipf("/bin/echo not present: %v", err)
	}
	reg := installSysProcessGo(t)

	streamA, streamB := net.Pipe()
	defer streamA.Close()
	defer streamB.Close()

	meta, err := proto.Marshal(&v2pb.ProcessOpenRequest{
		Command: "/bin/echo",
		Args:    []string{"hello"},
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
	if openResp.GetError() != "" {
		t.Fatalf("open error: %s", openResp.GetError())
	}
	if openResp.GetPid() == 0 {
		t.Errorf("pid = 0, want non-zero")
	}

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
