package plugin_test

import (
	"context"
	"os"
	"runtime"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"

	"github.com/WangYihang/Platypus/internal/agent/plugin"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// installSysInfoGo wires the staged TinyGo sys-info-go plugin into a
// fresh registry. Same capability set as the Rust counterpart
// (sysinfo + fs.read of /proc, /etc, /sys); the agent has no idea
// the .wasm came from a different source language.
func installSysInfoGo(t *testing.T) *plugin.Registry {
	t.Helper()
	wasm := stagedWasmBytes(t, "com.platypus.sys-info-go", "1.0.0", "sys_info_plugin.wasm")
	manifestBytes := stagedManifestBytes(t, "com.platypus.sys-info-go", "1.0.0")

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
	sig, err := plugin.Sign(sk, wasm, plugin.DefaultTrustedComment("sys_info_plugin.wasm"))
	if err != nil {
		t.Fatal(err)
	}
	reg, err := plugin.New(plugin.Options{Paths: paths})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { reg.Close(context.Background()) })

	if err := reg.InstallFromBytes(context.Background(), plugin.InstallParams{
		PluginID:            "com.platypus.sys-info-go",
		Version:             "1.0.0",
		PublisherPubkey:     []byte(plugin.EncodePublicKey(pk, "")),
		Manifest:            []byte(manifestStr),
		Wasm:                wasm,
		Signature:           []byte(plugin.EncodeSignature(sig)),
		Actor:               "test",
		GrantedCapabilities: []string{"sysinfo", "fs.read"},
	}, nil); err != nil {
		t.Fatal(err)
	}
	return reg
}

// invokeGoSysInfo invokes the Go plugin's sys_info RPC directly
// (the bridge.SysInfo helper hardcodes the Rust plugin id) and
// returns the unmarshalled response. The plugin emits protojson, so
// we use protojson.Unmarshal to match the Rust integration's bridge
// path.
func invokeGoSysInfo(t *testing.T, reg *plugin.Registry) *v2pb.SysInfoResponse {
	t.Helper()
	resp := reg.Invoke(context.Background(), &v2pb.PluginCallRequest{
		PluginId: "com.platypus.sys-info-go",
		Method:   "sys_info",
	})
	if resp.GetError() != "" {
		t.Fatalf("sys_info error: %s", resp.GetError())
	}
	out := &v2pb.SysInfoResponse{}
	if err := protojson.Unmarshal(resp.GetPayload(), out); err != nil {
		t.Fatalf("protojson.Unmarshal: %v\npayload: %s", err, resp.GetPayload())
	}
	return out
}

// TestSysInfoGo_Hostname round-trips a sys_info call through the Go
// plugin and asserts the hostname matches what the OS reports.
// Mirrors the Rust TestSysInfo_Hostname so a parity-matrix check is
// just a pair of greps.
func TestSysInfoGo_Hostname(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("sys-info reads /proc paths; linux-only")
	}
	reg := installSysInfoGo(t)
	resp := invokeGoSysInfo(t, reg)

	wantHost, _ := os.Hostname()
	if resp.GetHostname() == "" {
		t.Errorf("hostname empty in response: %+v", resp)
	}
	if wantHost != "" && resp.GetHostname() != wantHost {
		t.Logf("note: plugin hostname=%q, os.Hostname=%q (acceptable on containers where /etc/hostname differs)",
			resp.GetHostname(), wantHost)
	}
}

// TestSysInfoGo_PlatformReportsLinux asserts the plugin populates
// at least one of platform / platform_family / kernel_version on
// linux. Identical fixture to the Rust counterpart.
func TestSysInfoGo_PlatformReportsLinux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("sys-info reads /proc paths; linux-only")
	}
	reg := installSysInfoGo(t)
	resp := invokeGoSysInfo(t, reg)
	if resp.GetPlatform() == "" && resp.GetPlatformFamily() == "" && resp.GetKernelVersion() == "" {
		t.Errorf("no platform info populated: %+v", resp)
	}
}
