package plugin

import (
	"strings"
	"testing"
)

const validManifest = `
api_version: 1
id: com.example.nginx-status
name: Nginx Status
version: 1.4.2
author: { name: Jane, email: jane@example.com }
license: Apache-2.0
runtime:
  type: wasm
  entry: nginx_status.wasm
  abi: extism/1
rpc:
  - name: nginx_status
    request:  { proto: NginxStatusRequest }
    response: { proto: NginxStatusResponse }
capabilities:
  exec:
    commands: [/usr/sbin/nginx]
  fs.read:
    paths: [/etc/nginx]
  kv: true
resources:
  max_memory_mb: 32
  max_invocation_ms: 5000
signature:
  algo: minisign-ed25519
  key_id: RWQTESTKEY00000000
  sig_file: nginx_status.wasm.minisig
`

func TestParseManifest_Happy(t *testing.T) {
	m, err := ParseManifest([]byte(validManifest))
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if m.ID != "com.example.nginx-status" {
		t.Errorf("id = %q", m.ID)
	}
	if m.Version != "1.4.2" {
		t.Errorf("version = %q", m.Version)
	}
	if len(m.RPC) != 1 || m.RPC[0].Name != "nginx_status" {
		t.Errorf("rpc = %+v", m.RPC)
	}
	want := []CapabilityID{CapLog, CapKV, CapExec, CapFSRead}
	got := m.DeclaredCapabilities()
	if len(got) != len(want) {
		t.Fatalf("declared = %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("declared[%d] = %s, want %s", i, got[i], w)
		}
	}
}

func TestParseManifest_RejectsInvalid(t *testing.T) {
	cases := []struct {
		name string
		mut  func(string) string
		want string
	}{
		{"bad api_version", strReplace("api_version: 1", "api_version: 2"), "api_version=2"},
		{"bad id", strReplace("com.example.nginx-status", "Bad ID"), "is not a valid reverse-DNS"},
		{"bad version", strReplace("version: 1.4.2", "version: 1.4"), "not strict semver"},
		{"missing runtime entry", strReplace("entry: nginx_status.wasm", "entry: ''"), "runtime.entry is required"},
		{"path traversal in entry", strReplace("entry: nginx_status.wasm", "entry: ../foo.wasm"), "must be a plain filename"},
		{"bad abi", strReplace("abi: extism/1", "abi: extism/2"), "extism/1"},
		{"no rpc + no streams", strReplace("rpc:\n  - name: nginx_status\n    request:  { proto: NginxStatusRequest }\n    response: { proto: NginxStatusResponse }", "rpc: []"), "at least one rpc or streams entry is required"},
		{"exec without commands", strReplace("commands: [/usr/sbin/nginx]", "commands: []"), "exec set without any commands"},
		{"relative exec command", strReplace("commands: [/usr/sbin/nginx]", "commands: [nginx]"), "must be an absolute path"},
		{"oversize memory", strReplace("max_memory_mb: 32", "max_memory_mb: 4096"), "exceeds the 1024 MB ceiling"},
		{"missing timeout", strReplace("max_invocation_ms: 5000", "max_invocation_ms: 0"), "max_invocation_ms is required"},
		{"missing sig algo", strReplace("algo: minisign-ed25519", "algo: pgp"), "minisign-ed25519"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := tc.mut(validManifest)
			_, err := ParseManifest([]byte(input))
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestValidateGranted(t *testing.T) {
	m, err := ParseManifest([]byte(validManifest))
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	cases := []struct {
		name    string
		granted []string
		wantErr string
	}{
		{"all-declared", []string{"kv", "exec", "fs.read"}, ""},
		{"unknown cap", []string{"telekinesis"}, "unknown"},
		{"overgrant", []string{"net.http"}, "not requested by manifest"},
		{"empty", nil, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := m.ValidateGranted(tc.granted)
			if tc.wantErr == "" {
				if err != nil {
					t.Errorf("unexpected: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %v, want substring %q", err, tc.wantErr)
			}
		})
	}
}

func strReplace(old, new string) func(string) string {
	return func(in string) string { return strings.Replace(in, old, new, 1) }
}

// TestIsAbsCrossPlatform covers the manifest validator's
// platform-agnostic absolute-path check. Linux servers must accept
// `C:\\Windows\\...` paths in cross-platform plugin manifests
// (e.g. sys-procs-windows's allowlisted powershell.exe), so the
// validator can't lean on stdlib's filepath.IsAbs which is
// host-native.
func TestIsAbsCrossPlatform(t *testing.T) {
	cases := []struct {
		p    string
		want bool
	}{
		{"/usr/bin/df", true},
		{"/", true},
		{"/proc", true},
		// Windows drive letter forms.
		{`C:\Windows\System32\powershell.exe`, true},
		{`c:\path\to\thing`, true},
		{"D:/forward/slash/works/too", true},
		// UNC.
		{`\\server\share\file.exe`, true},
		// Non-absolute / relative forms must reject.
		{"powershell.exe", false},
		{"../etc/passwd", false},
		{"./foo", false},
		{"", false},
		{"C:", false},        // missing root separator
		{"C:foo", false},     // drive-relative path, not absolute
		{`\single-back`, false},
	}
	for _, tc := range cases {
		t.Run(tc.p, func(t *testing.T) {
			if got := isAbsCrossPlatform(tc.p); got != tc.want {
				t.Errorf("isAbsCrossPlatform(%q) = %v, want %v", tc.p, got, tc.want)
			}
		})
	}
}
