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

func installSysDiskDarwin(t *testing.T) *plugin.Registry {
	t.Helper()
	const id = "com.platypus.sys-disk-darwin"
	const ver = "1.0.0"
	wasm := stagedWasmBytes(t, id, ver, "sys_disk_darwin.wasm")
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
	sig, err := plugin.Sign(sk, wasm, plugin.DefaultTrustedComment("sys_disk_darwin.wasm"))
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

func TestSysDiskDarwin_Manifest(t *testing.T) {
	manifestBytes := stagedManifestBytes(t, "com.platypus.sys-disk-darwin", "1.0.0")
	m, err := plugin.ParseManifest(manifestBytes)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := m.Runtime.OSTargets; len(got) != 1 || got[0] != "darwin" {
		t.Errorf("os_targets = %v, want [darwin]", got)
	}
	if len(m.RPC) != 1 || m.RPC[0].Name != "list_filesystems" {
		t.Errorf("rpc = %+v", m.RPC)
	}
	if m.Capabilities.Exec == nil ||
		len(m.Capabilities.Exec.Commands) != 1 ||
		m.Capabilities.Exec.Commands[0] != "/bin/df" {
		t.Errorf("exec capability mis-declared: %+v", m.Capabilities.Exec)
	}
}

func TestSysDiskDarwin_ListFilesystems_RoundTrip(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("sys-disk-darwin shells to BSD /bin/df; runs on darwin only")
	}
	reg := installSysDiskDarwin(t)

	reqJSON, _ := json.Marshal(map[string]any{"skip_pseudo": true})
	resp := reg.Invoke(context.Background(), &v2pb.PluginCallRequest{
		PluginId: "com.platypus.sys-disk-darwin",
		Method:   "list_filesystems",
		Payload:  reqJSON,
	})
	if resp.GetError() != "" {
		t.Fatalf("plugin call: %s", resp.GetError())
	}
	var decoded struct {
		Filesystems []struct {
			Source     string `json:"source"`
			Fstype     string `json:"fstype"`
			Mountpoint string `json:"mountpoint"`
			SizeBytes  uint64 `json:"sizeBytes"`
		} `json:"filesystems"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(resp.GetPayload(), &decoded); err != nil {
		t.Fatalf("decode response %q: %v", string(resp.GetPayload()), err)
	}
	if decoded.Error != "" {
		t.Fatalf("plugin error: %s", decoded.Error)
	}
	if len(decoded.Filesystems) == 0 {
		t.Errorf("no filesystems returned (expected at least the / volume)")
	}
	hasRoot := false
	for _, fs := range decoded.Filesystems {
		if fs.Mountpoint == "/" {
			hasRoot = true
			if fs.SizeBytes == 0 {
				t.Errorf("/ has zero size_bytes: %+v", fs)
			}
		}
	}
	if !hasRoot {
		t.Errorf("expected the root '/' mount in skip_pseudo=true output")
	}
}
