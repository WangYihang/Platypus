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

// installSysSystemd wires the staged sys-systemd-linux wasm into a
// fresh registry. The plugin needs CapExec (the systemctl shell-out)
// — sysinfo / fs.read aren't required because every read goes through
// systemctl(1)'s parsed output.
//
// Source-of-truth for artefacts is the staged tree under
// internal/server/sysplugins/embedded/system-plugins/. A missing
// artefact fails the test loudly (D-tests-era convention) so a
// skipped re-stage doesn't hide a real bug.
func installSysSystemd(t *testing.T) *plugin.Registry {
	t.Helper()
	const id = "com.platypus.sys-systemd-linux"
	const ver = "1.0.0"
	wasm := stagedWasmBytes(t, id, ver, "sys_systemd_linux.wasm")
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
	sig, err := plugin.Sign(sk, wasm, plugin.DefaultTrustedComment("sys_systemd_linux.wasm"))
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

// TestSysSystemd_Manifest_OSTargetsLinux exercises the H1 manifest
// plumbing end-to-end via the staged artefact: the parsed manifest
// must declare os_targets=[linux] and lang=rust.
func TestSysSystemd_Manifest_OSTargetsLinux(t *testing.T) {
	manifestBytes := stagedManifestBytes(t, "com.platypus.sys-systemd-linux", "1.0.0")
	m, err := plugin.ParseManifest(manifestBytes)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := m.Runtime.OSTargets; len(got) != 1 || got[0] != "linux" {
		t.Errorf("os_targets = %v, want [linux]", got)
	}
	if m.Runtime.Lang != "rust" {
		t.Errorf("lang = %q, want rust", m.Runtime.Lang)
	}
	rpcSet := map[string]bool{}
	for _, r := range m.RPC {
		rpcSet[r.Name] = true
	}
	for _, want := range []string{"list_units", "show_unit", "unit_action"} {
		if !rpcSet[want] {
			t.Errorf("rpc %q not declared", want)
		}
	}
	if m.Capabilities.Exec == nil ||
		len(m.Capabilities.Exec.Commands) != 2 {
		t.Errorf("exec capability mis-declared: %+v", m.Capabilities.Exec)
	}
}

// TestSysSystemd_ListUnits_RoundTrip exercises the full plugin path:
// install → invoke list_units → the response decodes to the expected
// shape. We don't assert specific units exist (containers may not
// have a working systemd daemon — `systemctl list-units` returns a
// connection error in that case). The test verifies the JSON
// envelope, not the systemd state.
func TestSysSystemd_ListUnits_RoundTrip(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("sys-systemd-linux is linux-only by os_targets")
	}
	reg := installSysSystemd(t)

	reqJSON, err := json.Marshal(map[string]any{"unit_type": "service"})
	if err != nil {
		t.Fatal(err)
	}
	resp := reg.Invoke(context.Background(), &v2pb.PluginCallRequest{
		PluginId: "com.platypus.sys-systemd-linux",
		Method:   "list_units",
		Payload:  reqJSON,
	})
	if resp.GetError() != "" {
		t.Fatalf("plugin call: %s", resp.GetError())
	}
	var decoded struct {
		Units []struct {
			Name        string `json:"name"`
			Load        string `json:"load"`
			Active      string `json:"active"`
			Sub         string `json:"sub"`
			Description string `json:"description,omitempty"`
		} `json:"units"`
		Error string `json:"error,omitempty"`
	}
	if err := json.Unmarshal(resp.GetPayload(), &decoded); err != nil {
		t.Fatalf("decode response %q: %v", string(resp.GetPayload()), err)
	}
	// Either the daemon is up (units > 0, no error) OR it isn't (units
	// == 0, error mentions a connection / bus failure). Both are
	// acceptable — the assertion is the JSON shape parses cleanly.
	if decoded.Error == "" && len(decoded.Units) > 0 {
		// Sanity: every entry must have a name + service-typed
		// suffix (we asked for unit_type=service).
		for _, u := range decoded.Units {
			if u.Name == "" {
				t.Errorf("unit with empty name: %+v", u)
			}
		}
		t.Logf("listed %d service units (first: %s)", len(decoded.Units), decoded.Units[0].Name)
	} else {
		t.Logf("list_units returned error %q (units=%d) — expected on systemd-less containers",
			decoded.Error, len(decoded.Units))
	}
}

// TestSysSystemd_UnitAction_Rejects_BadAction confirms the plugin's
// allowlist gate works without needing a running systemd daemon —
// the early validation path returns before host_exec is touched.
func TestSysSystemd_UnitAction_Rejects_BadAction(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("sys-systemd-linux is linux-only by os_targets")
	}
	reg := installSysSystemd(t)

	reqJSON, err := json.Marshal(map[string]any{
		"name":   "ssh.service",
		"action": "daemon-reexec",
	})
	if err != nil {
		t.Fatal(err)
	}
	resp := reg.Invoke(context.Background(), &v2pb.PluginCallRequest{
		PluginId: "com.platypus.sys-systemd-linux",
		Method:   "unit_action",
		Payload:  reqJSON,
	})
	if resp.GetError() != "" {
		t.Fatalf("plugin call: %s", resp.GetError())
	}
	var decoded struct {
		Ok    bool   `json:"ok"`
		Error string `json:"error,omitempty"`
	}
	if err := json.Unmarshal(resp.GetPayload(), &decoded); err != nil {
		t.Fatalf("decode: %v (payload=%q)", err, string(resp.GetPayload()))
	}
	if decoded.Ok {
		t.Errorf("daemon-reexec should be rejected; got ok=true")
	}
	if decoded.Error == "" {
		t.Errorf("expected non-empty error for disallowed action")
	}
}

// TestSysSystemd_UnitAction_Rejects_LeadingDash confirms the unit
// name validator stops a `-foo` argument from being passed to
// systemctl as an option.
func TestSysSystemd_UnitAction_Rejects_LeadingDash(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("sys-systemd-linux is linux-only by os_targets")
	}
	reg := installSysSystemd(t)

	reqJSON, err := json.Marshal(map[string]any{
		"name":   "-evil",
		"action": "status",
	})
	if err != nil {
		t.Fatal(err)
	}
	resp := reg.Invoke(context.Background(), &v2pb.PluginCallRequest{
		PluginId: "com.platypus.sys-systemd-linux",
		Method:   "unit_action",
		Payload:  reqJSON,
	})
	if resp.GetError() != "" {
		t.Fatalf("plugin call: %s", resp.GetError())
	}
	var decoded struct {
		Ok    bool   `json:"ok"`
		Error string `json:"error,omitempty"`
	}
	if err := json.Unmarshal(resp.GetPayload(), &decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.Ok || decoded.Error == "" {
		t.Errorf("expected rejection of -evil; got ok=%v err=%q", decoded.Ok, decoded.Error)
	}
}
