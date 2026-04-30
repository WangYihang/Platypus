//go:build linux

package security

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSplitSSHDLine(t *testing.T) {
	cases := []struct {
		in       string
		key, val string
		ok       bool
	}{
		{"PermitRootLogin no", "PermitRootLogin", "no", true},
		{"PermitRootLogin\tno", "PermitRootLogin", "no", true},
		{"Port=22", "Port", "22", true},
		{"Port = 22", "Port", "22", true},
		{"PasswordAuthentication no  # inline", "PasswordAuthentication", "no", true},
		{"# comment", "", "", false},
		{"   ", "", "", false},
		{"Lonely", "", "", false},
	}
	for _, c := range cases {
		k, v, ok := splitSSHDLine(c.in)
		if ok != c.ok || k != c.key || v != c.val {
			t.Errorf("splitSSHDLine(%q) = (%q,%q,%v), want (%q,%q,%v)", c.in, k, v, ok, c.key, c.val, c.ok)
		}
	}
}

func TestSSHConfigCheck_FlagsRiskyDirectives(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sshd_config")
	body := `# test fixture
PermitRootLogin yes
PasswordAuthentication yes
PermitEmptyPasswords yes
X11Forwarding yes
Protocol 2,1
LoginGraceTime 0
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	c := &sshConfigCheck{path: path}
	findings, err := c.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := map[string]string{}
	for _, f := range findings {
		got[f.ID] = f.Severity
	}
	want := map[string]string{
		"ssh.permitrootlogin":        SeverityHigh,
		"ssh.passwordauthentication": SeverityHigh,
		"ssh.permitemptypasswords":   SeverityCritical,
		"ssh.x11forwarding":          SeverityLow,
		"ssh.protocol":               SeverityCritical,
		"ssh.logingracetime":         SeverityMedium,
	}
	if len(got) != len(want) {
		t.Fatalf("finding set mismatch: got %+v, want %+v", got, want)
	}
	for id, sev := range want {
		if got[id] != sev {
			t.Errorf("finding %s: severity %q, want %q", id, got[id], sev)
		}
	}
}

func TestSSHConfigCheck_HardenedConfigProducesNoFindings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sshd_config")
	body := `PermitRootLogin no
PasswordAuthentication no
PermitEmptyPasswords no
X11Forwarding no
LoginGraceTime 30
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	c := &sshConfigCheck{path: path}
	findings, err := c.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Fatalf("hardened config should produce no findings, got %+v", findings)
	}
}

func TestSSHConfigCheck_MissingDirectivesFlaggedAsDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sshd_config")
	if err := os.WriteFile(path, []byte("# nothing\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := &sshConfigCheck{path: path}
	findings, err := c.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, f := range findings {
		got[f.ID] = true
	}
	// PermitRootLogin and PasswordAuthentication default-bad on
	// historical sshd builds, so we report when absent.
	if !got["ssh.permitrootlogin"] || !got["ssh.passwordauthentication"] {
		t.Fatalf("expected absent-directive findings for root login + password auth, got %+v", got)
	}
	// PermitEmptyPasswords / X11Forwarding default-good; absence
	// should NOT produce a finding.
	if got["ssh.permitemptypasswords"] || got["ssh.x11forwarding"] {
		t.Fatalf("default-good directives should not be reported when absent, got %+v", got)
	}
}
