package plugin_test

import (
	"context"
	"encoding/json"
	"os"
	"runtime"
	"testing"

	"github.com/WangYihang/Platypus/internal/agent/plugin"
	"github.com/WangYihang/Platypus/internal/agent/plugin/bridge"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

func installSysNetLinux(t *testing.T) *plugin.Registry {
	t.Helper()
	const id = "com.platypus.sys-net-linux"
	const ver = "1.0.0"
	wasm := stagedWasmBytes(t, id, ver, "sys_net_linux.wasm")
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
	sig, err := plugin.Sign(sk, wasm, plugin.DefaultTrustedComment("sys_net_linux.wasm"))
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
		GrantedCapabilities: []plugin.CapabilityID{"fs.read"},
	}, nil); err != nil {
		t.Fatal(err)
	}
	return reg
}

func TestSysNetLinux_Manifest(t *testing.T) {
	manifestBytes := stagedManifestBytes(t, "com.platypus.sys-net-linux", "1.0.0")
	m, err := plugin.ParseManifest(manifestBytes)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := m.Runtime.OSTargets; len(got) != 1 || got[0] != "linux" {
		t.Errorf("os_targets = %v, want [linux]", got)
	}
	if len(m.RPC) != 2 {
		t.Errorf("rpc len = %d, want 2 (list_listeners + list_connections)", len(m.RPC))
	}
	if m.Capabilities.FSRead == nil ||
		len(m.Capabilities.FSRead.Paths) != 1 ||
		m.Capabilities.FSRead.Paths[0] != "/proc/net" {
		t.Errorf("fs.read mis-declared: %+v", m.Capabilities.FSRead)
	}
}

// TestSysNetLinux_ListListeners_RoundTrip exercises the plugin in
// this Linux container — /proc/net/tcp is always available and
// usually has at least one LISTEN row (sshd, the test runner's own
// transient socket, etc.). Asserts on JSON shape; tolerates an
// empty result if the kernel has no listeners (rare).
func TestSysNetLinux_ListListeners_RoundTrip(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("sys-net-linux reads /proc/net; runs on linux only")
	}
	reg := installSysNetLinux(t)

	resp := reg.Invoke(context.Background(), &v2pb.PluginCallRequest{
		PluginId: "com.platypus.sys-net-linux",
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
		t.Fatalf("decode: %v\npayload: %s", err, resp.GetPayload())
	}
	if decoded.Error != "" {
		t.Fatalf("plugin error: %s", decoded.Error)
	}
	t.Logf("listed %d listeners", len(decoded.Listeners))
	for i, l := range decoded.Listeners {
		if l.Proto != "tcp" && l.Proto != "tcp6" {
			t.Errorf("listener[%d].proto = %q, want tcp|tcp6", i, l.Proto)
		}
		if l.LocalAddr == "" {
			t.Errorf("listener[%d] has empty local_addr", i)
		}
	}
}

// TestSysNetLinux_ListConnections_StateFilter verifies the state
// filter works: passing state=LISTEN should return only LISTEN rows.
func TestSysNetLinux_ListConnections_StateFilter(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("sys-net-linux reads /proc/net; runs on linux only")
	}
	reg := installSysNetLinux(t)

	body, _ := json.Marshal(map[string]any{"state": "LISTEN"})
	resp := reg.Invoke(context.Background(), &v2pb.PluginCallRequest{
		PluginId: "com.platypus.sys-net-linux",
		Method:   "list_connections",
		Payload:  body,
	})
	if resp.GetError() != "" {
		t.Fatalf("plugin call: %s", resp.GetError())
	}
	var decoded struct {
		Connections []struct {
			State string `json:"state"`
		} `json:"connections"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(resp.GetPayload(), &decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for i, c := range decoded.Connections {
		if c.State != "LISTEN" {
			t.Errorf("connection[%d].state = %q, want LISTEN (filter ignored?)", i, c.State)
		}
	}
}

// TestSysNetLinux_TypedBridge_RoundTrip exercises bridge.ListListeners
// + bridge.ListConnections. Fails if proto/v2/sys_net.proto drifts
// from the JSON the plugin emits.
func TestSysNetLinux_TypedBridge_RoundTrip(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("sys-net-linux is linux-only")
	}
	reg := installSysNetLinux(t)

	listeners := bridge.ListListeners(reg)(context.Background(), &v2pb.ListListenersRequest{})
	if listeners.GetError() != "" {
		t.Fatalf("ListListeners typed bridge: %s", listeners.GetError())
	}
	for i, l := range listeners.GetListeners() {
		if l.GetProto() == "" {
			t.Errorf("listener[%d] empty proto", i)
		}
	}

	conns := bridge.ListConnections(reg)(context.Background(),
		&v2pb.ListConnectionsRequest{})
	if conns.GetError() != "" {
		t.Fatalf("ListConnections typed bridge: %s", conns.GetError())
	}
	for i, c := range conns.GetConnections() {
		if c.GetProto() == "" || c.GetState() == "" {
			t.Errorf("connection[%d] missing proto or state: %+v", i, c)
		}
	}
}
