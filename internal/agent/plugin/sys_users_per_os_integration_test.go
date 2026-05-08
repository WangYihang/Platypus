package plugin_test

import (
	"testing"

	"github.com/WangYihang/Platypus/internal/agent/plugin"
)

// Per-OS sys-users siblings — manifest sanity only on Linux CI.

func TestSysUsersDarwin_Manifest(t *testing.T) {
	manifestBytes := stagedManifestBytes(t, "com.platypus.sys-users-darwin", "1.0.0")
	m, err := plugin.ParseManifest(manifestBytes)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := m.Runtime.OSTargets; len(got) != 1 || got[0] != "darwin" {
		t.Errorf("os_targets = %v, want [darwin]", got)
	}
	if len(m.RPC) != 1 || m.RPC[0].Name != "list_users" {
		t.Errorf("rpc = %+v", m.RPC)
	}
	if m.Capabilities.Exec == nil ||
		!containsString(m.Capabilities.Exec.Commands, "/usr/bin/dscl") {
		t.Errorf("exec must include /usr/bin/dscl: %+v", m.Capabilities.Exec)
	}
}

func TestSysUsersWindows_Manifest(t *testing.T) {
	manifestBytes := stagedManifestBytes(t, "com.platypus.sys-users-windows", "1.0.0")
	m, err := plugin.ParseManifest(manifestBytes)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := m.Runtime.OSTargets; len(got) != 1 || got[0] != "windows" {
		t.Errorf("os_targets = %v, want [windows]", got)
	}
	if len(m.RPC) != 1 || m.RPC[0].Name != "list_users" {
		t.Errorf("rpc = %+v", m.RPC)
	}
	if m.Capabilities.Exec == nil ||
		!containsString(m.Capabilities.Exec.Commands, `C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`) {
		t.Errorf("exec must include powershell.exe: %+v", m.Capabilities.Exec)
	}
}

func containsString(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}
