package plugin_test

import (
	"context"
	"os"
	"runtime"
	"testing"

	"github.com/WangYihang/Platypus/internal/agent/plugin"
	"github.com/WangYihang/Platypus/internal/agent/plugin/bridge"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

func installSysUsersLinux(t *testing.T) *plugin.Registry {
	t.Helper()
	const id = "com.platypus.sys-users-linux"
	const ver = "1.0.0"
	wasm := stagedWasmBytes(t, id, ver, "sys_users_linux.wasm")
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
	sig, err := plugin.Sign(sk, wasm, plugin.DefaultTrustedComment("sys_users_linux.wasm"))
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

func TestSysUsersLinux_Manifest(t *testing.T) {
	manifestBytes := stagedManifestBytes(t, "com.platypus.sys-users-linux", "1.0.0")
	m, err := plugin.ParseManifest(manifestBytes)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := m.Runtime.OSTargets; len(got) != 1 || got[0] != "linux" {
		t.Errorf("os_targets = %v, want [linux]", got)
	}
	if len(m.RPC) != 1 || m.RPC[0].Name != "list_users" {
		t.Errorf("rpc = %+v", m.RPC)
	}
}

// TestSysUsersLinux_TypedBridge_RoundTrip exercises bridge.UserList.
// Asserts proto matches plugin output and that the test container
// has at least the root account.
func TestSysUsersLinux_TypedBridge_RoundTrip(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("sys-users-linux is linux-only")
	}
	reg := installSysUsersLinux(t)

	// Ask for system accounts so the assertion finds root reliably
	// even on a slim container with no human users.
	resp := bridge.UserList(reg)(context.Background(),
		&v2pb.UserListRequest{IncludeSystem: true})
	if resp.GetError() != "" {
		t.Fatalf("typed bridge: %s", resp.GetError())
	}
	if len(resp.GetUsers()) == 0 {
		t.Fatal("expected at least one user")
	}
	hasRoot := false
	for _, u := range resp.GetUsers() {
		if u.GetUsername() == "root" && u.GetUid() == 0 {
			hasRoot = true
		}
		if u.GetUsername() == "" {
			t.Errorf("user with empty username: %+v", u)
		}
	}
	if !hasRoot {
		t.Errorf("expected root user in response")
	}
	t.Logf("listed %d users, %d groups, %d sudoers",
		len(resp.GetUsers()), len(resp.GetGroups()), len(resp.GetSudoers()))
}
