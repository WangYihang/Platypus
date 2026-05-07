package main

import "testing"

// ---- parseKernelMajorMinor ----------------------------------------

func TestParseKernelMajorMinor(t *testing.T) {
	cases := []struct {
		in           string
		major, minor uint32
	}{
		{"5.15.0-1024-generic", 5, 15},
		{"6.1.0-13-amd64", 6, 1},
		{"4.19.0-25-cloud-amd64", 4, 19},
		{"5.10.0", 5, 10},
		{"5.15", 5, 15},
		{"not-a-version", 0, 0},
		{"", 0, 0},
	}
	for _, c := range cases {
		major, minor := parseKernelMajorMinor(c.in)
		if major != c.major || minor != c.minor {
			t.Errorf("parseKernelMajorMinor(%q) = (%d, %d); want (%d, %d)",
				c.in, major, minor, c.major, c.minor)
		}
	}
}

// ---- kernelFindings -----------------------------------------------

func TestKernelFindings_Modern(t *testing.T) {
	for _, in := range []string{"5.10.0", "5.15.0-1024-generic", "6.1.0"} {
		if got := kernelFindings(in); len(got) != 0 {
			t.Errorf("kernelFindings(%q) = %d findings; want 0", in, len(got))
		}
	}
}

func TestKernelFindings_OutdatedEmitsOne(t *testing.T) {
	got := kernelFindings("4.19.0-25-cloud-amd64")
	if len(got) != 1 {
		t.Fatalf("got %d findings, want 1", len(got))
	}
	if got[0].ID != "kernel.version.outdated" {
		t.Errorf("ID = %q", got[0].ID)
	}
	if got[0].Severity != "medium" {
		t.Errorf("Severity = %q", got[0].Severity)
	}
}

func TestKernelFindings_GarbageFlagged(t *testing.T) {
	if len(kernelFindings("garbage")) != 1 {
		t.Error("garbage should be treated as outdated")
	}
}

// ---- mitigationFindings -------------------------------------------

func TestMitigationFindings_NotAffected(t *testing.T) {
	if got := mitigationFindings("meltdown", "Not affected\n"); len(got) != 0 {
		t.Errorf("got %d findings, want 0", len(got))
	}
}

func TestMitigationFindings_Mitigation(t *testing.T) {
	if got := mitigationFindings("spectre_v2", "Mitigation: Retpolines\n"); len(got) != 0 {
		t.Errorf("got %d findings, want 0", len(got))
	}
}

func TestMitigationFindings_Vulnerable(t *testing.T) {
	got := mitigationFindings("spectre_v1", "Vulnerable: __user pointer sanitization")
	if len(got) != 1 {
		t.Fatalf("got %d findings, want 1", len(got))
	}
	if got[0].ID != "kernel.mitigations.spectre_v1" {
		t.Errorf("ID = %q", got[0].ID)
	}
	if got[0].Severity != "high" {
		t.Errorf("Severity = %q", got[0].Severity)
	}
}

func TestMitigationFindings_Empty(t *testing.T) {
	if got := mitigationFindings("meltdown", ""); len(got) != 0 {
		t.Errorf("got %d findings, want 0", len(got))
	}
	if got := mitigationFindings("meltdown", "   \n"); len(got) != 0 {
		t.Errorf("got %d findings, want 0", len(got))
	}
}

// ---- sshdConfigFindings -------------------------------------------

func TestSSHDFindings_CleanConfig(t *testing.T) {
	raw := `# default config
PermitRootLogin no
PasswordAuthentication no
PubkeyAuthentication yes
`
	if got := sshdConfigFindings(raw); len(got) != 0 {
		t.Errorf("got %d findings, want 0", len(got))
	}
}

func TestSSHDFindings_PermitRootYes(t *testing.T) {
	got := sshdConfigFindings("PermitRootLogin yes\n")
	if len(got) != 1 {
		t.Fatalf("got %d findings, want 1", len(got))
	}
	if got[0].ID != "ssh.permit_root_login" || got[0].Severity != "high" {
		t.Errorf("unexpected: %+v", got[0])
	}
}

func TestSSHDFindings_PasswordAuth(t *testing.T) {
	got := sshdConfigFindings("PasswordAuthentication yes\n")
	if len(got) != 1 || got[0].Severity != "medium" {
		t.Errorf("unexpected: %+v", got)
	}
}

func TestSSHDFindings_EmptyPasswords(t *testing.T) {
	got := sshdConfigFindings("PermitEmptyPasswords yes\n")
	if len(got) != 1 || got[0].ID != "ssh.permit_empty_passwords" || got[0].Severity != "critical" {
		t.Errorf("unexpected: %+v", got)
	}
}

func TestSSHDFindings_X11Forwarding(t *testing.T) {
	got := sshdConfigFindings("X11Forwarding yes\n")
	if len(got) != 1 || got[0].ID != "ssh.x11_forwarding" || got[0].Severity != "low" {
		t.Errorf("unexpected: %+v", got)
	}
}

func TestSSHDFindings_AllFour(t *testing.T) {
	raw := "PermitRootLogin yes\nPasswordAuthentication yes\nPermitEmptyPasswords yes\nX11Forwarding yes\n"
	if got := sshdConfigFindings(raw); len(got) != 4 {
		t.Errorf("got %d findings, want 4", len(got))
	}
}

func TestSSHDFindings_SkipsCommentedLine(t *testing.T) {
	raw := "#PermitRootLogin yes\nPermitRootLogin no  # but this is fine\n"
	if got := sshdConfigFindings(raw); len(got) != 0 {
		t.Errorf("got %d findings, want 0", len(got))
	}
}

// ---- sysctlFinding -------------------------------------------------

func TestSysctlFinding_Match(t *testing.T) {
	exps := sysctlExpectations()
	if got := sysctlFinding(exps[0], "2"); len(got) != 0 {
		t.Errorf("got %d findings, want 0", len(got))
	}
}

func TestSysctlFinding_Mismatch(t *testing.T) {
	exps := sysctlExpectations()
	got := sysctlFinding(exps[0], "0")
	if len(got) != 1 {
		t.Fatalf("got %d findings, want 1", len(got))
	}
	if got[0].ID != "sysctl.kernel.kptr_restrict" {
		t.Errorf("ID = %q", got[0].ID)
	}
}

func TestSysctlFinding_AlternateAcceptable(t *testing.T) {
	var exp sysctlExpectation
	for _, e := range sysctlExpectations() {
		if e.key == "kernel.unprivileged_bpf_disabled" {
			exp = e
			break
		}
	}
	if len(sysctlFinding(exp, "1")) != 0 {
		t.Error("'1' should be accepted")
	}
	if len(sysctlFinding(exp, "2")) != 0 {
		t.Error("'2' should be accepted")
	}
	if len(sysctlFinding(exp, "0")) != 1 {
		t.Error("'0' should be flagged")
	}
}

func TestSysctlPath(t *testing.T) {
	if got := sysctlPath("kernel.kptr_restrict"); got != "/proc/sys/kernel/kptr_restrict" {
		t.Errorf("got %q", got)
	}
	if got := sysctlPath("net.ipv4.conf.all.rp_filter"); got != "/proc/sys/net/ipv4/conf/all/rp_filter" {
		t.Errorf("got %q", got)
	}
}

func TestNormalizeSysctl(t *testing.T) {
	if got := normalizeSysctl("4   4\t1   7"); got != "4 4 1 7" {
		t.Errorf("got %q", got)
	}
	if got := normalizeSysctl("\t2\n"); got != "2" {
		t.Errorf("got %q", got)
	}
}

// ---- parsePathEnv -------------------------------------------------

func TestParsePathEnv_Typical(t *testing.T) {
	environ := "USER=root\x00HOME=/root\x00PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin\x00LANG=C\x00"
	got := parsePathEnv(environ)
	want := []string{"/usr/local/sbin", "/usr/local/bin", "/usr/sbin", "/usr/bin"}
	if !equalStrings(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestParsePathEnv_NoPath(t *testing.T) {
	if got := parsePathEnv("USER=root\x00HOME=/root\x00"); len(got) != 0 {
		t.Errorf("got %v, want []", got)
	}
}

func TestParsePathEnv_Dedupes(t *testing.T) {
	got := parsePathEnv("PATH=/bin:/usr/bin:/bin\x00")
	want := []string{"/bin", "/usr/bin"}
	if !equalStrings(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestParsePathEnv_EmptySegments(t *testing.T) {
	got := parsePathEnv("PATH=/bin::/usr/bin\x00")
	want := []string{"/bin", "/usr/bin"}
	if !equalStrings(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// ---- splitParent --------------------------------------------------

func TestSplitParent_Typical(t *testing.T) {
	parent, name, ok := splitParent("/usr/bin/sudo")
	if !ok || parent != "/usr/bin" || name != "sudo" {
		t.Errorf("got (%q, %q, %v)", parent, name, ok)
	}
	parent, name, ok = splitParent("/etc")
	if !ok || parent != "/" || name != "etc" {
		t.Errorf("got (%q, %q, %v)", parent, name, ok)
	}
}

func TestSplitParent_RootRejected(t *testing.T) {
	if _, _, ok := splitParent("/"); ok {
		t.Error("'/' should return ok=false")
	}
	if _, _, ok := splitParent(""); ok {
		t.Error("'' should return ok=false")
	}
}

func TestSplitParent_StripsTrailingSlash(t *testing.T) {
	parent, name, ok := splitParent("/usr/bin/")
	if !ok || parent != "/usr" || name != "bin" {
		t.Errorf("got (%q, %q, %v)", parent, name, ok)
	}
}

// ---- pathWritableFinding ------------------------------------------

func TestPathWritableFinding(t *testing.T) {
	f := pathWritableFinding("/opt/custom-bin", 0o757)
	if f.Severity != "critical" {
		t.Errorf("Severity = %q", f.Severity)
	}
	if f.Evidence != "/opt/custom-bin mode=0757" {
		t.Errorf("Evidence = %q", f.Evidence)
	}
}

// ---- octal4 -------------------------------------------------------

func TestOctal4(t *testing.T) {
	cases := []struct {
		mode uint32
		want string
	}{
		{0o0000, "0000"},
		{0o0644, "0644"},
		{0o0755, "0755"},
		{0o4755, "4755"},
		{0o2755, "2755"},
		{0o1777, "1777"},
		{0o4750, "4750"},
	}
	for _, c := range cases {
		if got := octal4(c.mode); got != c.want {
			t.Errorf("octal4(%#o) = %q; want %q", c.mode, got, c.want)
		}
	}
}

// ---- suidOutlierFinding -------------------------------------------

func TestSUIDOutlier_NoSpecial(t *testing.T) {
	if _, ok := suidOutlierFinding("/usr/bin/ls", "ls", 0o755); ok {
		t.Error("ls without special bits shouldn't flag")
	}
}

func TestSUIDOutlier_Allowlisted(t *testing.T) {
	if _, ok := suidOutlierFinding("/usr/bin/sudo", "sudo", 0o4755); ok {
		t.Error("sudo is allowlisted")
	}
	if _, ok := suidOutlierFinding("/usr/bin/passwd", "passwd", 0o4755); ok {
		t.Error("passwd is allowlisted")
	}
}

func TestSUIDOutlier_SetuidOutsider(t *testing.T) {
	f, ok := suidOutlierFinding("/opt/vendor/helper", "helper", 0o4755)
	if !ok {
		t.Fatal("expected finding")
	}
	if f.ID != "fs.suid_outlier" || f.Severity != "medium" {
		t.Errorf("unexpected %+v", f)
	}
	if f.Title != "Unexpected setuid binary" {
		t.Errorf("Title = %q", f.Title)
	}
}

func TestSUIDOutlier_SetgidOutsider(t *testing.T) {
	f, ok := suidOutlierFinding("/opt/vendor/helper", "helper", 0o2755)
	if !ok {
		t.Fatal("expected finding")
	}
	if f.Title != "Unexpected setgid binary" {
		t.Errorf("Title = %q", f.Title)
	}
}

// ---- depthBelow ---------------------------------------------------

func TestDepthBelow(t *testing.T) {
	cases := []struct {
		root, path string
		want       int
	}{
		{"/opt", "/opt", 0},
		{"/opt", "/opt/foo", 1},
		{"/opt", "/opt/a/b/c", 3},
	}
	for _, c := range cases {
		if got := depthBelow(c.root, c.path); got != c.want {
			t.Errorf("depthBelow(%q, %q) = %d; want %d", c.root, c.path, got, c.want)
		}
	}
}

// ---- helpers ------------------------------------------------------

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
