//go:build linux

package security

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func init() {
	Register(&sysctlCheck{})
}

// sysctlCheck reads a curated set of /proc/sys keys and flags
// settings that diverge from the recommended hardened value. The
// table is intentionally short — these are the knobs that are
// (a) cheap to read, (b) routinely audited by CIS / lynis, and
// (c) actionable with a single sysctl line.
type sysctlCheck struct{}

func (sysctlCheck) ID() string                        { return "sysctl.posture" }
func (sysctlCheck) Category() string                  { return "sysctl" }
func (sysctlCheck) Applicable(_ context.Context) bool { return dirExists("/proc/sys") }
func (sysctlCheck) Metadata() CheckMetadata {
	return CheckMetadata{
		Title: "Sysctl hardening posture",
		Description: "Reads a curated set of /proc/sys keys (kptr_restrict, " +
			"unprivileged_bpf_disabled, rp_filter, accept_redirects, fs.protected_*, " +
			"fs.suid_dumpable, tcp_syncookies, kernel.dmesg_restrict) and reports any " +
			"that diverge from the recommended hardened value. One finding per misaligned " +
			"key — the row's evidence shows the live value vs the expected one.",
		References: []string{"CIS 3.2", "CIS 1.5"},
	}
}

type sysctlExpectation struct {
	key string // sysctl key in dotted notation
	// want is the set of acceptable values; the finding fires only
	// when the live value matches none of them. Multiple entries
	// cover sysctls where stricter settings are also acceptable
	// (e.g. unprivileged_bpf_disabled=2 is "1, but locked", which
	// satisfies the same security posture as 1).
	want     []string
	severity string
	title    string
	desc     string
	fix      string
}

var sysctlExpectations = []sysctlExpectation{
	{
		key: "kernel.kptr_restrict", want: []string{"2"}, severity: SeverityMedium,
		title: "Kernel pointers exposed via /proc",
		desc:  "kernel.kptr_restrict<2 lets unprivileged processes read raw kernel addresses, which weakens KASLR.",
		fix:   "Set kernel.kptr_restrict=2 in /etc/sysctl.d/99-hardening.conf and run sysctl --system.",
	},
	{
		key: "kernel.dmesg_restrict", want: []string{"1"}, severity: SeverityLow,
		title: "dmesg readable by unprivileged users",
		desc:  "kernel.dmesg_restrict=0 lets any local user read kernel ring buffer contents (often leaks pointers, KASLR offsets, and driver state).",
		fix:   "Set kernel.dmesg_restrict=1 in /etc/sysctl.d/99-hardening.conf.",
	},
	{
		// 2 is "1, but locked until reboot" — strictly stronger than 1.
		key: "kernel.unprivileged_bpf_disabled", want: []string{"1", "2"}, severity: SeverityHigh,
		title: "Unprivileged eBPF enabled",
		desc:  "kernel.unprivileged_bpf_disabled=0 leaves a recurring CVE-class privilege escalation surface (CVE-2021-3490, CVE-2022-23222, …).",
		fix:   "Set kernel.unprivileged_bpf_disabled=1 unless you specifically need it for an eBPF userspace tool that runs unprivileged.",
	},
	{
		// 1 = strict, 2 = loose; both enforce reverse-path filtering.
		key: "net.ipv4.conf.all.rp_filter", want: []string{"1", "2"}, severity: SeverityMedium,
		title: "Reverse path filtering disabled",
		desc:  "net.ipv4.conf.all.rp_filter=0 allows spoofed source addresses to reach local services on multi-homed hosts.",
		fix:   "Set net.ipv4.conf.all.rp_filter=1 (strict mode), or 2 if asymmetric routes are expected.",
	},
	{
		key: "net.ipv4.conf.all.accept_redirects", want: []string{"0"}, severity: SeverityMedium,
		title: "ICMP redirects accepted",
		desc:  "Accepting ICMP redirects lets a local attacker rewrite the host's routing table.",
		fix:   "Set net.ipv4.conf.all.accept_redirects=0 and net.ipv6.conf.all.accept_redirects=0.",
	},
	{
		key: "net.ipv4.conf.all.send_redirects", want: []string{"0"}, severity: SeverityLow,
		title: "Sending ICMP redirects",
		desc:  "Hosts that aren't routers shouldn't emit ICMP redirects; doing so leaks topology information.",
		fix:   "Set net.ipv4.conf.all.send_redirects=0.",
	},
	{
		key: "net.ipv4.tcp_syncookies", want: []string{"1", "2"}, severity: SeverityLow,
		title: "TCP SYN cookies disabled",
		desc:  "SYN cookies are the primary defense against SYN-flood denial of service.",
		fix:   "Set net.ipv4.tcp_syncookies=1.",
	},
	{
		key: "fs.protected_hardlinks", want: []string{"1"}, severity: SeverityMedium,
		title: "Hardlink protection disabled",
		desc:  "fs.protected_hardlinks=0 enables a class of /tmp race-condition privilege escalations.",
		fix:   "Set fs.protected_hardlinks=1.",
	},
	{
		key: "fs.protected_symlinks", want: []string{"1"}, severity: SeverityMedium,
		title: "Symlink protection disabled",
		desc:  "fs.protected_symlinks=0 enables symlink-attack TOCTOU bugs in setuid programs and /tmp consumers.",
		fix:   "Set fs.protected_symlinks=1.",
	},
	{
		key: "fs.suid_dumpable", want: []string{"0"}, severity: SeverityMedium,
		title: "Setuid binaries write core dumps",
		desc:  "fs.suid_dumpable!=0 lets crashed setuid programs leave core files containing privileged memory state.",
		fix:   "Set fs.suid_dumpable=0.",
	},
}

func (sysctlCheck) Run(_ context.Context) ([]Finding, error) {
	var findings []Finding
	for _, e := range sysctlExpectations {
		got, err := readSysctl(e.key)
		if err != nil {
			// Missing keys are common on minimal containers / older
			// kernels — treat as "not applicable to this host"
			// rather than as a finding or a check error.
			continue
		}
		if matchesAny(got, e.want) {
			continue
		}
		findings = append(findings, Finding{
			ID:          "sysctl." + e.key,
			Category:    "sysctl",
			Severity:    e.severity,
			Title:       e.title,
			Description: e.desc,
			Evidence:    fmt.Sprintf("%s = %s (expected %s)", e.key, got, strings.Join(e.want, " or ")),
			Remediation: e.fix,
		})
	}
	return findings, nil
}

func readSysctl(key string) (string, error) {
	path := filepath.Join("/proc/sys", strings.ReplaceAll(key, ".", "/"))
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	// Multi-value sysctls (e.g. kernel.printk) return whitespace-
	// separated tokens; collapse to a single space-joined string so
	// equality comparisons are meaningful.
	return strings.Join(strings.Fields(string(b)), " "), nil
}

func matchesAny(got string, want []string) bool {
	for _, w := range want {
		if got == w {
			return true
		}
	}
	return false
}

func dirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}
