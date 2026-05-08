package plugin_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/WangYihang/Platypus/internal/agent/plugin"
	"github.com/WangYihang/Platypus/internal/agent/plugin/bridge"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// installSysProcess wires the freshly-built sys-process wasm into a
// fresh registry. Caller picks which capabilities to grant (the
// merged plugin declares both `exec` and `process` independently).
func installSysProcess(t *testing.T, granted []plugin.CapabilityID) *plugin.Registry {
	t.Helper()
	wasm, err := os.ReadFile(sysProcessWasmPath())
	if err != nil {
		t.Skipf("sys_process.wasm not built (%v) — run `cargo build --release --target wasm32-unknown-unknown` in examples/plugins/system/sys-process/", err)
	}
	manifestBytes, err := os.ReadFile(filepath.Join("..", "..", "..",
		"example", "plugins", "system", "sys-process", "plugin.yaml"))
	if err != nil {
		t.Fatal(err)
	}

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
	manifestStr := strings.Replace(string(manifestBytes),
		"REPLACE_WITH_YOUR_KEY_ID", plugin.HumanKeyID(pk), 1)
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
		PluginID:            "com.platypus.sys-process",
		Version:             "1.0.0",
		PublisherPubkey:     []byte(plugin.EncodePublicKey(pk, "")),
		Manifest:            []byte(manifestStr),
		Wasm:                wasm,
		Signature:           []byte(plugin.EncodeSignature(sig)),
		Actor:               "test",
		GrantedCapabilities: granted,
	}, nil); err != nil {
		t.Fatal(err)
	}
	return reg
}

// TestProcessExec_Stdout: a simple `/bin/echo hello` returns exit
// code 0, stdout containing "hello", and an empty error.
func TestProcessExec_Stdout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("/bin/echo not available on windows")
	}
	if _, err := os.Stat("/bin/echo"); err != nil {
		t.Skipf("/bin/echo not present: %v", err)
	}
	reg := installSysProcess(t, []plugin.CapabilityID{"exec"})

	resp := bridge.Exec(reg)(context.Background(), &v2pb.ExecRequest{
		Command:   "/bin/echo",
		Args:      []string{"hello"},
		TimeoutMs: 5000,
	})
	if resp.GetError() != "" {
		t.Fatalf("exec error: %s", resp.GetError())
	}
	if resp.GetExitCode() != 0 {
		t.Errorf("exit_code = %d; want 0", resp.GetExitCode())
	}
	if !strings.Contains(string(resp.GetStdout()), "hello") {
		t.Errorf("stdout = %q; want to contain hello", resp.GetStdout())
	}
}

// TestProcessExec_NonZeroExit: exit codes round-trip; an explicit
// `sh -c "exit 7"` lands as exit_code=7 with no transport error.
func TestProcessExec_NonZeroExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("/bin/sh not available on windows")
	}
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skipf("/bin/sh not present: %v", err)
	}
	reg := installSysProcess(t, []plugin.CapabilityID{"exec"})

	resp := bridge.Exec(reg)(context.Background(), &v2pb.ExecRequest{
		Command:   "/bin/sh",
		Args:      []string{"-c", "exit 7"},
		TimeoutMs: 5000,
	})
	if resp.GetExitCode() != 7 {
		t.Errorf("exit_code = %d; want 7", resp.GetExitCode())
	}
}

// TestProcessExec_DeniedWithoutCapability: granting only `process`
// (not `exec`) means the host rejects exec calls with capability
// denied. Different families = independent grants, mirroring the
// merge's intent.
func TestProcessExec_DeniedWithoutCapability(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("/bin/echo not available on windows")
	}
	reg := installSysProcess(t, []plugin.CapabilityID{"process"}) // exec NOT granted

	resp := bridge.Exec(reg)(context.Background(), &v2pb.ExecRequest{
		Command:   "/bin/echo",
		Args:      []string{"x"},
		TimeoutMs: 5000,
	})
	// Plugin sees capability denial as Envelope.error; bridge surfaces
	// it through ExecResponse.error rather than as a transport error.
	if resp.GetError() == "" {
		t.Errorf("expected a denied error, got exit_code=%d stdout=%q",
			resp.GetExitCode(), resp.GetStdout())
	}
}

// TestProcessExec_StderrCapture: we route stderr through the same
// envelope as stdout. Confirm a process writing to stderr surfaces
// non-empty Stderr in the response.
func TestProcessExec_StderrCapture(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("/bin/sh not available on windows")
	}
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skipf("/bin/sh not present: %v", err)
	}
	reg := installSysProcess(t, []plugin.CapabilityID{"exec"})

	resp := bridge.Exec(reg)(context.Background(), &v2pb.ExecRequest{
		Command:   "/bin/sh",
		Args:      []string{"-c", "echo OUT; echo ERR 1>&2"},
		TimeoutMs: 5000,
	})
	if resp.GetError() != "" {
		t.Fatalf("exec error: %s", resp.GetError())
	}
	if !strings.Contains(string(resp.GetStdout()), "OUT") {
		t.Errorf("stdout = %q; want to contain OUT", resp.GetStdout())
	}
	if !strings.Contains(string(resp.GetStderr()), "ERR") {
		t.Errorf("stderr = %q; want to contain ERR", resp.GetStderr())
	}
}

func sysProcessWasmPath() string {
	return filepath.Join("..", "..", "..", "example", "plugins", "system", "sys-process",
		"target", "wasm32-unknown-unknown", "release", "sys_process.wasm")
}
