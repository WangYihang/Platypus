package plugin_test

import (
	"testing"

	"github.com/WangYihang/Platypus/internal/agent/plugin"
)

// Per-OS sys-firewall siblings — manifest sanity only on linux CI.
// Typed-bridge round trips need pfctl / PowerShell and run on the
// per-OS CI matrix.

func TestSysFirewallDarwin_Manifest(t *testing.T) {
	manifestBytes := stagedManifestBytes(t, "com.platypus.sys-firewall-darwin", "1.0.0")
	m, err := plugin.ParseManifest(manifestBytes)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := m.Runtime.OSTargets; len(got) != 1 || got[0] != "darwin" {
		t.Errorf("os_targets = %v, want [darwin]", got)
	}
	if len(m.RPC) != 1 || m.RPC[0].Name != "list_firewall_rules" {
		t.Errorf("rpc = %+v", m.RPC)
	}
	if m.Capabilities.Exec == nil ||
		len(m.Capabilities.Exec.Commands) != 1 ||
		m.Capabilities.Exec.Commands[0] != "/sbin/pfctl" {
		t.Errorf("exec capability mis-declared: %+v", m.Capabilities.Exec)
	}
}

func TestSysFirewallWindows_Manifest(t *testing.T) {
	manifestBytes := stagedManifestBytes(t, "com.platypus.sys-firewall-windows", "1.0.0")
	m, err := plugin.ParseManifest(manifestBytes)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := m.Runtime.OSTargets; len(got) != 1 || got[0] != "windows" {
		t.Errorf("os_targets = %v, want [windows]", got)
	}
	if len(m.RPC) != 1 || m.RPC[0].Name != "list_firewall_rules" {
		t.Errorf("rpc = %+v", m.RPC)
	}
	if m.Capabilities.Exec == nil ||
		len(m.Capabilities.Exec.Commands) != 1 ||
		m.Capabilities.Exec.Commands[0] != `C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe` {
		t.Errorf("exec capability mis-declared: %+v", m.Capabilities.Exec)
	}
}
