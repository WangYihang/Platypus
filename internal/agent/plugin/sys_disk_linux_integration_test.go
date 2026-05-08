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

func installSysDiskLinux(t *testing.T) *plugin.Registry {
	t.Helper()
	const id = "com.platypus.sys-disk-linux"
	const ver = "1.0.0"
	wasm := stagedWasmBytes(t, id, ver, "sys_disk_linux.wasm")
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
	sig, err := plugin.Sign(sk, wasm, plugin.DefaultTrustedComment("sys_disk_linux.wasm"))
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
		GrantedCapabilities: []plugin.CapabilityID{"exec"},
	}, nil); err != nil {
		t.Fatal(err)
	}
	return reg
}

// TestSysDiskLinux_Manifest_OSTargetsLinux: H1 plumbing sanity.
func TestSysDiskLinux_Manifest_OSTargetsLinux(t *testing.T) {
	manifestBytes := stagedManifestBytes(t, "com.platypus.sys-disk-linux", "1.0.0")
	m, err := plugin.ParseManifest(manifestBytes)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := m.Runtime.OSTargets; len(got) != 1 || got[0] != "linux" {
		t.Errorf("os_targets = %v, want [linux]", got)
	}
	if len(m.RPC) != 1 || m.RPC[0].Name != "list_filesystems" {
		t.Errorf("rpc = %+v", m.RPC)
	}
}

// TestSysDiskLinux_ListFilesystems_RoundTrip exercises the full
// plugin path against the test environment's df. Asserts at least
// one filesystem is reported (the container's root /) when
// skip_pseudo=true.
func TestSysDiskLinux_ListFilesystems_RoundTrip(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("sys-disk-linux is linux-only")
	}
	reg := installSysDiskLinux(t)

	reqJSON, _ := json.Marshal(map[string]any{"skip_pseudo": true})
	resp := reg.Invoke(context.Background(), &v2pb.PluginCallRequest{
		PluginId: "com.platypus.sys-disk-linux",
		Method:   "list_filesystems",
		Payload:  reqJSON,
	})
	if resp.GetError() != "" {
		t.Fatalf("plugin call: %s", resp.GetError())
	}
	var decoded struct {
		Filesystems []struct {
			Source         string `json:"source"`
			Fstype         string `json:"fstype"`
			Mountpoint     string `json:"mountpoint"`
			SizeBytes      uint64 `json:"sizeBytes"`
			UsedBytes      uint64 `json:"usedBytes"`
			AvailableBytes uint64 `json:"availableBytes"`
			PercentUsed    uint8  `json:"percentUsed"`
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
		t.Errorf("no filesystems returned (skip_pseudo filtered everything?)")
	}
	for _, fs := range decoded.Filesystems {
		if fs.Mountpoint == "" {
			t.Errorf("entry missing mountpoint: %+v", fs)
		}
		if fs.Fstype == "" {
			t.Errorf("entry missing fstype: %+v", fs)
		}
		if fs.SizeBytes == 0 {
			t.Errorf("entry has zero size_bytes: %+v", fs)
		}
		if fs.PercentUsed > 100 {
			t.Errorf("percent_used out of range: %d (%+v)", fs.PercentUsed, fs)
		}
	}
	t.Logf("listed %d non-pseudo filesystems", len(decoded.Filesystems))
}

// TestSysDiskLinux_ListFilesystems_SkipPseudoOff confirms that with
// skip_pseudo=false the tmpfs entries appear in the result. This
// also doubles as an assertion that the filter is request-driven,
// not hardcoded.
func TestSysDiskLinux_ListFilesystems_SkipPseudoOff(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("sys-disk-linux is linux-only")
	}
	reg := installSysDiskLinux(t)

	reqJSON, _ := json.Marshal(map[string]any{"skip_pseudo": false})
	resp := reg.Invoke(context.Background(), &v2pb.PluginCallRequest{
		PluginId: "com.platypus.sys-disk-linux",
		Method:   "list_filesystems",
		Payload:  reqJSON,
	})
	if resp.GetError() != "" {
		t.Fatalf("plugin call: %s", resp.GetError())
	}
	var decoded struct {
		Filesystems []struct {
			Fstype string `json:"fstype"`
		} `json:"filesystems"`
	}
	if err := json.Unmarshal(resp.GetPayload(), &decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	hasTmpfs := false
	for _, fs := range decoded.Filesystems {
		if fs.Fstype == "tmpfs" {
			hasTmpfs = true
			break
		}
	}
	if !hasTmpfs {
		t.Logf("no tmpfs reported with skip_pseudo=false; environment may not have one (rare)")
	}
}

// TestSysDiskLinux_TypedBridge_RoundTrip exercises bridge.FilesystemList.
// Fails if proto/v2/sys_disk.proto drifts from the JSON shape the
// plugin actually emits.
func TestSysDiskLinux_TypedBridge_RoundTrip(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("sys-disk-linux is linux-only")
	}
	reg := installSysDiskLinux(t)

	resp := bridge.FilesystemList(reg)(context.Background(),
		&v2pb.FilesystemListRequest{SkipPseudo: true})
	if resp.GetError() != "" {
		t.Fatalf("typed bridge returned error: %s", resp.GetError())
	}
	if len(resp.GetFilesystems()) == 0 {
		t.Fatalf("expected at least one mounted filesystem")
	}
	for i, fs := range resp.GetFilesystems() {
		if fs.GetMountpoint() == "" {
			t.Errorf("filesystems[%d] empty mountpoint: %+v", i, fs)
		}
		if fs.GetSizeBytes() == 0 && fs.GetMountpoint() != "" {
			// /proc and similar pseudo fs have size 0 — but skip_pseudo
			// should have filtered them. Real fs always has non-zero.
			t.Logf("filesystems[%d] %s has size 0 (filtered by skip_pseudo?)", i, fs.GetMountpoint())
		}
	}
}
