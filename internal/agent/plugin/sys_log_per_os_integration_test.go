package plugin_test

import (
	"testing"

	"github.com/WangYihang/Platypus/internal/agent/plugin"
)

// Per-OS sys-log siblings — manifest sanity only on linux CI;
// typed-bridge round trips need the actual OS to run `log show`
// (darwin) or Get-WinEvent (windows) and live in the per-OS CI
// matrix.

func TestSysLogDarwin_Manifest(t *testing.T) {
	manifestBytes := stagedManifestBytes(t, "com.platypus.sys-log-darwin", "1.0.0")
	m, err := plugin.ParseManifest(manifestBytes)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := m.Runtime.OSTargets; len(got) != 1 || got[0] != "darwin" {
		t.Errorf("os_targets = %v, want [darwin]", got)
	}
	if len(m.RPC) != 1 || m.RPC[0].Name != "query" {
		t.Errorf("rpc = %+v", m.RPC)
	}
	if m.Capabilities.Exec == nil ||
		!containsString(m.Capabilities.Exec.Commands, "/usr/bin/log") {
		t.Errorf("exec must include /usr/bin/log: %+v", m.Capabilities.Exec)
	}
}

func TestSysLogWindows_Manifest(t *testing.T) {
	manifestBytes := stagedManifestBytes(t, "com.platypus.sys-log-windows", "1.0.0")
	m, err := plugin.ParseManifest(manifestBytes)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := m.Runtime.OSTargets; len(got) != 1 || got[0] != "windows" {
		t.Errorf("os_targets = %v, want [windows]", got)
	}
	if len(m.RPC) != 1 || m.RPC[0].Name != "query" {
		t.Errorf("rpc = %+v", m.RPC)
	}
	if m.Capabilities.Exec == nil ||
		!containsString(m.Capabilities.Exec.Commands, `C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`) {
		t.Errorf("exec must include powershell.exe: %+v", m.Capabilities.Exec)
	}
}
