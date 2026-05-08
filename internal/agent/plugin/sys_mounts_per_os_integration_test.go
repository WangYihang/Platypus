package plugin_test

import (
	"testing"

	"github.com/WangYihang/Platypus/internal/agent/plugin"
)

// Lightweight manifest sanity tests for the per-OS sys-mounts
// siblings. Their typed-bridge round trip needs the actual OS to
// run /sbin/mount or PowerShell; that part is exercised by the
// per-OS CI matrix (which skips when runtime.GOOS doesn't match).

func TestSysMountsDarwin_Manifest(t *testing.T) {
	manifestBytes := stagedManifestBytes(t, "com.platypus.sys-mounts-darwin", "1.0.0")
	m, err := plugin.ParseManifest(manifestBytes)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := m.Runtime.OSTargets; len(got) != 1 || got[0] != "darwin" {
		t.Errorf("os_targets = %v, want [darwin]", got)
	}
	if len(m.RPC) != 1 || m.RPC[0].Name != "list_mounts" {
		t.Errorf("rpc = %+v", m.RPC)
	}
	if m.Capabilities.Exec == nil ||
		len(m.Capabilities.Exec.Commands) != 1 ||
		m.Capabilities.Exec.Commands[0] != "/sbin/mount" {
		t.Errorf("exec capability mis-declared: %+v", m.Capabilities.Exec)
	}
}

func TestSysMountsWindows_Manifest(t *testing.T) {
	manifestBytes := stagedManifestBytes(t, "com.platypus.sys-mounts-windows", "1.0.0")
	m, err := plugin.ParseManifest(manifestBytes)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := m.Runtime.OSTargets; len(got) != 1 || got[0] != "windows" {
		t.Errorf("os_targets = %v, want [windows]", got)
	}
	if len(m.RPC) != 1 || m.RPC[0].Name != "list_mounts" {
		t.Errorf("rpc = %+v", m.RPC)
	}
	if m.Capabilities.Exec == nil ||
		len(m.Capabilities.Exec.Commands) != 1 ||
		m.Capabilities.Exec.Commands[0] != `C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe` {
		t.Errorf("exec capability mis-declared: %+v", m.Capabilities.Exec)
	}
}
