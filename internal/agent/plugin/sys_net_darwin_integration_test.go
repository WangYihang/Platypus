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

func installSysNetDarwin(t *testing.T) *plugin.Registry {
	t.Helper()
	const id = "com.platypus.sys-net-darwin"
	const ver = "1.0.0"
	wasm := stagedWasmBytes(t, id, ver, "sys_net_darwin.wasm")
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
	sig, err := plugin.Sign(sk, wasm, plugin.DefaultTrustedComment("sys_net_darwin.wasm"))
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

func TestSysNetDarwin_Manifest(t *testing.T) {
	manifestBytes := stagedManifestBytes(t, "com.platypus.sys-net-darwin", "1.0.0")
	m, err := plugin.ParseManifest(manifestBytes)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := m.Runtime.OSTargets; len(got) != 1 || got[0] != "darwin" {
		t.Errorf("os_targets = %v, want [darwin]", got)
	}
	if len(m.RPC) != 2 {
		t.Errorf("rpc len = %d, want 2", len(m.RPC))
	}
	if m.Capabilities.Exec == nil || len(m.Capabilities.Exec.Commands) != 1 ||
		m.Capabilities.Exec.Commands[0] != "/usr/sbin/netstat" {
		t.Errorf("exec mis-declared: %+v", m.Capabilities.Exec)
	}
}

func TestSysNetDarwin_ListListeners_RoundTrip(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("sys-net-darwin shells to BSD netstat; runs on darwin only")
	}
	reg := installSysNetDarwin(t)

	resp := reg.Invoke(context.Background(), &v2pb.PluginCallRequest{
		PluginId: "com.platypus.sys-net-darwin",
		Method:   "list_listeners",
		Payload:  []byte(`{}`),
	})
	if resp.GetError() != "" {
		t.Fatalf("plugin call: %s", resp.GetError())
	}
	var decoded struct {
		Listeners []struct {
			Proto     string `json:"proto"`
			LocalAddr string `json:"localAddr"`
			LocalPort uint16 `json:"localPort"`
		} `json:"listeners"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(resp.GetPayload(), &decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.Error != "" {
		t.Fatalf("plugin error: %s", decoded.Error)
	}
	t.Logf("darwin listed %d listeners", len(decoded.Listeners))
}
