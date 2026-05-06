package plugin_test

import (
	"context"
	"encoding/json"
	"os"
	"runtime"
	"testing"

	"github.com/WangYihang/Platypus/internal/agent/plugin"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// installSysJournald wires the staged sys-journald-linux wasm into a
// fresh registry. CapExec is the only privilege; the plugin shells
// out to journalctl(1).
func installSysJournald(t *testing.T) *plugin.Registry {
	t.Helper()
	const id = "com.platypus.sys-journald-linux"
	const ver = "1.0.0"
	wasm := stagedWasmBytes(t, id, ver, "sys_journald_linux.wasm")
	manifestBytes := stagedManifestBytes(t, id, ver)

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
	sig, err := plugin.Sign(sk, wasm, plugin.DefaultTrustedComment("sys_journald_linux.wasm"))
	if err != nil {
		t.Fatal(err)
	}
	reg, err := plugin.New(plugin.Options{Paths: paths})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { reg.Close(context.Background()) })

	if err := reg.InstallFromBytes(context.Background(), plugin.InstallParams{
		PluginID:            id,
		Version:             ver,
		PublisherPubkey:     []byte(plugin.EncodePublicKey(pk, "")),
		Manifest:            []byte(manifestStr),
		Wasm:                wasm,
		Signature:           []byte(plugin.EncodeSignature(sig)),
		Actor:               "test",
		GrantedCapabilities: []string{"exec"},
	}, nil); err != nil {
		t.Fatal(err)
	}
	return reg
}

// TestSysJournald_Manifest_OSTargetsLinux confirms the manifest is
// well-formed under the H1 plumbing: os_targets=[linux], one RPC
// declared, exec capability present.
func TestSysJournald_Manifest_OSTargetsLinux(t *testing.T) {
	manifestBytes := stagedManifestBytes(t, "com.platypus.sys-journald-linux", "1.0.0")
	m, err := plugin.ParseManifest(manifestBytes)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := m.Runtime.OSTargets; len(got) != 1 || got[0] != "linux" {
		t.Errorf("os_targets = %v, want [linux]", got)
	}
	if len(m.RPC) != 1 || m.RPC[0].Name != "query" {
		t.Errorf("rpc = %+v, want one entry named 'query'", m.RPC)
	}
	if m.Capabilities.Exec == nil ||
		len(m.Capabilities.Exec.Commands) != 2 {
		t.Errorf("exec capability mis-declared: %+v", m.Capabilities.Exec)
	}
}

// TestSysJournald_Query_RoundTrip exercises the full plugin path.
// The container we test in has no running journald, so journalctl
// returns an error — we assert the JSON envelope decodes cleanly
// either way.
func TestSysJournald_Query_RoundTrip(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("sys-journald-linux is linux-only by os_targets")
	}
	reg := installSysJournald(t)

	reqJSON, err := json.Marshal(map[string]any{
		"lines":    50,
		"priority": "info",
	})
	if err != nil {
		t.Fatal(err)
	}
	resp := reg.Invoke(context.Background(), &v2pb.PluginCallRequest{
		PluginId: "com.platypus.sys-journald-linux",
		Method:   "query",
		Payload:  reqJSON,
	})
	if resp.GetError() != "" {
		t.Fatalf("plugin call: %s", resp.GetError())
	}
	var decoded struct {
		Entries []struct {
			TimestampUs uint64 `json:"timestampUs"`
			Unit        string `json:"unit"`
			Priority    uint8  `json:"priority"`
			Message     string `json:"message"`
		} `json:"entries"`
		Truncated bool   `json:"truncated"`
		Error     string `json:"error"`
	}
	if err := json.Unmarshal(resp.GetPayload(), &decoded); err != nil {
		t.Fatalf("decode response %q: %v", string(resp.GetPayload()), err)
	}
	t.Logf("query returned %d entries (truncated=%v error=%q)",
		len(decoded.Entries), decoded.Truncated, decoded.Error)
}

// TestSysJournald_Query_RejectsBadUnit confirms the early-validation
// path stops `-evil` before host_exec is touched.
func TestSysJournald_Query_RejectsBadUnit(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("sys-journald-linux is linux-only by os_targets")
	}
	reg := installSysJournald(t)

	reqJSON, err := json.Marshal(map[string]any{
		"unit": "-evil",
	})
	if err != nil {
		t.Fatal(err)
	}
	resp := reg.Invoke(context.Background(), &v2pb.PluginCallRequest{
		PluginId: "com.platypus.sys-journald-linux",
		Method:   "query",
		Payload:  reqJSON,
	})
	if resp.GetError() != "" {
		t.Fatalf("plugin call: %s", resp.GetError())
	}
	var decoded struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(resp.GetPayload(), &decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.Error == "" {
		t.Errorf("expected validation error for unit=-evil; got empty")
	}
}

// TestSysJournald_Query_RejectsBadBoot confirms the boot validator
// (32-char hex, integer, or "all") rejects garbage.
func TestSysJournald_Query_RejectsBadBoot(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("sys-journald-linux is linux-only by os_targets")
	}
	reg := installSysJournald(t)

	reqJSON, _ := json.Marshal(map[string]any{"boot": "yesterday"})
	resp := reg.Invoke(context.Background(), &v2pb.PluginCallRequest{
		PluginId: "com.platypus.sys-journald-linux",
		Method:   "query",
		Payload:  reqJSON,
	})
	if resp.GetError() != "" {
		t.Fatalf("plugin call: %s", resp.GetError())
	}
	var decoded struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(resp.GetPayload(), &decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.Error == "" {
		t.Errorf("expected boot validation error; got empty")
	}
}
