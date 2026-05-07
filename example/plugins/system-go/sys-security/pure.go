// pure.go — decision-layer logic for sys-security-go. No SDK
// imports, no //go:build constraint, so `go test ./...` against the
// host triple compiles and runs these functions directly.

package main

import (
	"strconv"
	"strings"
)

// ---- response shapes (mirrors v2pb encodings) -------------------

type SecurityFinding struct {
	ID          string   `json:"id"`
	CheckID     string   `json:"checkId"`
	Category    string   `json:"category"`
	Severity    string   `json:"severity"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Evidence    string   `json:"evidence"`
	Remediation string   `json:"remediation"`
	References  []string `json:"references,omitempty"`
}

type CheckResult struct {
	ID           string `json:"id"`
	Category     string `json:"category"`
	Status       string `json:"status"` // "ok" | "skipped" | "error"
	Error        string `json:"error,omitempty"`
	ElapsedMs    uint64 `json:"elapsedMs,omitempty"`
	FindingCount uint32 `json:"findingCount,omitempty"`
}

type ScanResponse struct {
	Findings      []SecurityFinding `json:"findings"`
	Checks        []CheckResult     `json:"checks"`
	StartedAtUnix int64             `json:"startedAtUnix,omitempty"`
	ElapsedMs     uint64            `json:"elapsedMs,omitempty"`
	Error         string            `json:"error,omitempty"`
}

type AvailableCheck struct {
	ID          string   `json:"id"`
	Category    string   `json:"category"`
	Applicable  bool     `json:"applicable"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	References  []string `json:"references,omitempty"`
}

type ListResponse struct {
	Checks []AvailableCheck `json:"checks"`
	Error  string           `json:"error,omitempty"`
}

// ScanRequest accepts both `check_ids` (snake) and `checkIds`
// (camel) keys — operators may hand-craft requests via the REST API
// in either form, mirroring the Rust crate's serde alias setup.
type ScanRequest struct {
	CheckIDs      []string `json:"checkIds"`
	CheckIDsSnake []string `json:"check_ids"`
	Categories    []string `json:"categories"`
}

// ============================================================
// kernel.version
// ============================================================

// kernelFindings is the pure decision layer behind checkKernelVersion.
func kernelFindings(osrelease string) []SecurityFinding {
	major, minor := parseKernelMajorMinor(osrelease)
	if major > 5 || (major == 5 && minor >= 10) {
		return nil
	}
	return []SecurityFinding{{
		ID:          "kernel.version.outdated",
		CheckID:     "kernel.version",
		Category:    "kernel",
		Severity:    "medium",
		Title:       "Kernel " + osrelease + " is older than 5.10",
		Description: "Long-term-support kernel lines start at 5.10 (Mar 2021). Hosts on older kernels miss several years of CVE fixes.",
		Evidence:    "/proc/sys/kernel/osrelease = " + osrelease,
		Remediation: "Upgrade to a distribution release that ships a 5.10+ kernel; reboot.",
	}}
}

func parseKernelMajorMinor(s string) (uint32, uint32) {
	parts := strings.SplitN(s, ".", 3)
	var major, minor uint32
	if len(parts) >= 1 {
		if v, err := strconv.ParseUint(parts[0], 10, 32); err == nil {
			major = uint32(v)
		}
	}
	if len(parts) >= 2 {
		minorRaw := parts[1]
		end := 0
		for end < len(minorRaw) && minorRaw[end] >= '0' && minorRaw[end] <= '9' {
			end++
		}
		if v, err := strconv.ParseUint(minorRaw[:end], 10, 32); err == nil {
			minor = uint32(v)
		}
	}
	return major, minor
}

// ============================================================
// kernel.mitigations
// ============================================================

// mitigationFindings is the pure decision layer.
func mitigationFindings(name, body string) []SecurityFinding {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return nil
	}
	if strings.HasPrefix(trimmed, "Not affected") || strings.HasPrefix(trimmed, "Mitigation:") {
		return nil
	}
	return []SecurityFinding{{
		ID:          "kernel.mitigations." + name,
		CheckID:     "kernel.mitigations",
		Category:    "kernel",
		Severity:    "high",
		Title:       "CPU vulnerability '" + name + "' not mitigated",
		Description: "The kernel reports this CPU vulnerability as exploitable on this host. Common cause: the operator passed `mitigations=off` on the kernel command line, or the microcode update needed for the mitigation isn't installed.",
		Evidence:    "/sys/devices/system/cpu/vulnerabilities/" + name + " = " + trimmed,
		Remediation: "Remove `mitigations=off` (and any per-vuln overrides like `nospectre_v2`) from the kernel cmdline; install the latest microcode package; reboot.",
		References:  []string{"https://www.kernel.org/doc/html/latest/admin-guide/hw-vuln/index.html"},
	}}
}

// ============================================================
// ssh.config
// ============================================================

func sshdConfigFindings(raw string) []SecurityFinding {
	var findings []SecurityFinding
	for _, line := range strings.Split(raw, "\n") {
		if i := strings.Index(line, "#"); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		switch {
		case fields[0] == "PermitRootLogin" && fields[1] == "yes":
			findings = append(findings, SecurityFinding{
				ID:          "ssh.permit_root_login",
				CheckID:     "ssh.config",
				Category:    "ssh",
				Severity:    "high",
				Title:       "SSH server allows root login",
				Description: "PermitRootLogin yes lets anyone with the root password (or root SSH key) authenticate as root directly. Best practice: log in as a non-root user + use sudo.",
				Evidence:    "/etc/ssh/sshd_config: PermitRootLogin yes",
				Remediation: `Set PermitRootLogin to "no" (or "prohibit-password") in /etc/ssh/sshd_config and restart sshd.`,
			})
		case fields[0] == "PasswordAuthentication" && fields[1] == "yes":
			findings = append(findings, SecurityFinding{
				ID:          "ssh.password_authentication",
				CheckID:     "ssh.config",
				Category:    "ssh",
				Severity:    "medium",
				Title:       "SSH server allows password authentication",
				Description: "Password auth is brute-forceable. Public-key authentication has no such failure mode and is the recommended posture for production hosts.",
				Evidence:    "/etc/ssh/sshd_config: PasswordAuthentication yes",
				Remediation: `Distribute SSH keys to users, set PasswordAuthentication to "no" in /etc/ssh/sshd_config, restart sshd.`,
			})
		case fields[0] == "PermitEmptyPasswords" && fields[1] == "yes":
			findings = append(findings, SecurityFinding{
				ID:          "ssh.permit_empty_passwords",
				CheckID:     "ssh.config",
				Category:    "ssh",
				Severity:    "critical",
				Title:       "SSH server allows empty passwords",
				Description: "PermitEmptyPasswords yes lets accounts with no password set log in via SSH with no auth at all.",
				Evidence:    "/etc/ssh/sshd_config: PermitEmptyPasswords yes",
				Remediation: `Set PermitEmptyPasswords to "no" in /etc/ssh/sshd_config and restart sshd.`,
			})
		case fields[0] == "X11Forwarding" && fields[1] == "yes":
			findings = append(findings, SecurityFinding{
				ID:          "ssh.x11_forwarding",
				CheckID:     "ssh.config",
				Category:    "ssh",
				Severity:    "low",
				Title:       "SSH X11 forwarding enabled",
				Description: "X11Forwarding yes exposes the host's X server through the SSH session. A malicious or compromised client can read keystrokes and screen contents from any X11-aware program the user runs.",
				Evidence:    "/etc/ssh/sshd_config: X11Forwarding yes",
				Remediation: `Set X11Forwarding to "no" in /etc/ssh/sshd_config unless your operators rely on remote X11 apps.`,
			})
		}
	}
	return findings
}

// ============================================================
// sysctl.posture
// ============================================================

type sysctlExpectation struct {
	key      string
	want     []string
	severity string
	title    string
	desc     string
	fix      string
}

func sysctlExpectations() []sysctlExpectation {
	return []sysctlExpectation{
		{
			key: "kernel.kptr_restrict", want: []string{"2"}, severity: "medium",
			title: "Kernel pointers exposed via /proc",
			desc:  "kernel.kptr_restrict<2 lets unprivileged processes read raw kernel addresses, which weakens KASLR.",
			fix:   "Set kernel.kptr_restrict=2 in /etc/sysctl.d/99-hardening.conf and run sysctl --system.",
		},
		{
			key: "kernel.dmesg_restrict", want: []string{"1"}, severity: "low",
			title: "dmesg readable by unprivileged users",
			desc:  "kernel.dmesg_restrict=0 lets any local user read kernel ring buffer contents (often leaks pointers, KASLR offsets, and driver state).",
			fix:   "Set kernel.dmesg_restrict=1 in /etc/sysctl.d/99-hardening.conf.",
		},
		{
			// 2 = "1, but locked until reboot" — strictly stronger than 1.
			key: "kernel.unprivileged_bpf_disabled", want: []string{"1", "2"}, severity: "high",
			title: "Unprivileged eBPF enabled",
			desc:  "kernel.unprivileged_bpf_disabled=0 leaves a recurring CVE-class privilege escalation surface (CVE-2021-3490, CVE-2022-23222, …).",
			fix:   "Set kernel.unprivileged_bpf_disabled=1 unless you specifically need it for an eBPF userspace tool that runs unprivileged.",
		},
		{
			// 1 = strict, 2 = loose; both enforce reverse-path filtering.
			key: "net.ipv4.conf.all.rp_filter", want: []string{"1", "2"}, severity: "medium",
			title: "Reverse path filtering disabled",
			desc:  "net.ipv4.conf.all.rp_filter=0 allows spoofed source addresses to reach local services on multi-homed hosts.",
			fix:   "Set net.ipv4.conf.all.rp_filter=1 (strict mode), or 2 if asymmetric routes are expected.",
		},
		{
			key: "net.ipv4.conf.all.accept_redirects", want: []string{"0"}, severity: "medium",
			title: "ICMP redirects accepted",
			desc:  "Accepting ICMP redirects lets a local attacker rewrite the host's routing table.",
			fix:   "Set net.ipv4.conf.all.accept_redirects=0 and net.ipv6.conf.all.accept_redirects=0.",
		},
		{
			key: "net.ipv4.conf.all.send_redirects", want: []string{"0"}, severity: "low",
			title: "Sending ICMP redirects",
			desc:  "Hosts that aren't routers shouldn't emit ICMP redirects; doing so leaks topology information.",
			fix:   "Set net.ipv4.conf.all.send_redirects=0.",
		},
		{
			key: "net.ipv4.tcp_syncookies", want: []string{"1", "2"}, severity: "low",
			title: "TCP SYN cookies disabled",
			desc:  "SYN cookies are the primary defense against SYN-flood denial of service.",
			fix:   "Set net.ipv4.tcp_syncookies=1.",
		},
		{
			key: "fs.protected_hardlinks", want: []string{"1"}, severity: "medium",
			title: "Hardlink protection disabled",
			desc:  "fs.protected_hardlinks=0 enables a class of /tmp race-condition privilege escalations.",
			fix:   "Set fs.protected_hardlinks=1.",
		},
		{
			key: "fs.protected_symlinks", want: []string{"1"}, severity: "medium",
			title: "Symlink protection disabled",
			desc:  "fs.protected_symlinks=0 enables symlink-attack TOCTOU bugs in setuid programs and /tmp consumers.",
			fix:   "Set fs.protected_symlinks=1.",
		},
		{
			key: "fs.suid_dumpable", want: []string{"0"}, severity: "medium",
			title: "Setuid binaries write core dumps",
			desc:  "fs.suid_dumpable!=0 lets crashed setuid programs leave core files containing privileged memory state.",
			fix:   "Set fs.suid_dumpable=0.",
		},
	}
}

func sysctlPath(key string) string {
	return "/proc/sys/" + strings.ReplaceAll(key, ".", "/")
}

// normalizeSysctl collapses whitespace-separated tokens to a single
// space-joined string (multi-value sysctls like kernel.printk).
func normalizeSysctl(raw string) string {
	return strings.Join(strings.Fields(raw), " ")
}

func sysctlFinding(e sysctlExpectation, got string) []SecurityFinding {
	for _, w := range e.want {
		if got == w {
			return nil
		}
	}
	return []SecurityFinding{{
		ID:          "sysctl." + e.key,
		CheckID:     "sysctl.posture",
		Category:    "sysctl",
		Severity:    e.severity,
		Title:       e.title,
		Description: e.desc,
		Evidence:    e.key + " = " + got + " (expected " + strings.Join(e.want, " or ") + ")",
		Remediation: e.fix,
	}}
}

// ============================================================
// fs.path_writable
// ============================================================

var pathFallback = []string{
	"/usr/local/sbin",
	"/usr/local/bin",
	"/usr/sbin",
	"/usr/bin",
	"/sbin",
	"/bin",
	"/snap/bin",
}

// parsePathEnv extracts PATH= from a NUL-separated /proc/<pid>/environ blob.
func parsePathEnv(environ string) []string {
	for _, entry := range strings.Split(environ, "\x00") {
		if !strings.HasPrefix(entry, "PATH=") {
			continue
		}
		var seen []string
		for _, p := range strings.Split(entry[len("PATH="):], ":") {
			if p == "" {
				continue
			}
			if !contains(seen, p) {
				seen = append(seen, p)
			}
		}
		return seen
	}
	return nil
}

func splitParent(path string) (string, string, bool) {
	trimmed := strings.TrimRight(path, "/")
	if trimmed == "" || trimmed == "/" {
		return "", "", false
	}
	idx := strings.LastIndex(trimmed, "/")
	if idx < 0 {
		return "", "", false
	}
	parent := "/"
	if idx > 0 {
		parent = trimmed[:idx]
	}
	name := trimmed[idx+1:]
	if name == "" {
		return "", "", false
	}
	return parent, name, true
}

func pathWritableFinding(dir string, mode uint32) SecurityFinding {
	return SecurityFinding{
		ID:          "fs.path_writable",
		CheckID:     "fs.path_writable",
		Category:    "filesystem",
		Severity:    "critical",
		Title:       "World-writable directory on PATH",
		Description: "Any local user can replace binaries in this directory. The next time root (or any other account) invokes a command that resolves here, it executes attacker-controlled code.",
		Evidence:    dir + " mode=" + octal4(mode),
		Remediation: "chmod o-w " + dir + "; investigate how the directory got created with this mode (often a packaging bug or a misconfigured deploy script).",
	}
}

// octal4 renders a Unix mode as a 4-digit octal string (no fmt
// dependency — safe for TinyGo binary size).
func octal4(mode uint32) string {
	return string([]byte{
		'0' + byte((mode>>9)&0o7),
		'0' + byte((mode>>6)&0o7),
		'0' + byte((mode>>3)&0o7),
		'0' + byte(mode&0o7),
	})
}

// ============================================================
// fs.suid_outliers
// ============================================================

var suidScanRoots = []string{
	"/usr/bin",
	"/usr/sbin",
	"/usr/local/bin",
	"/usr/local/sbin",
	"/bin",
	"/sbin",
	"/opt",
}

const suidScanCap = 20_000

// suidAllowlist is the set of binary base names that legitimately
// ship as setuid/setgid on common Linux distros.
var suidAllowlist = map[string]struct{}{
	"chage": {}, "chfn": {}, "chsh": {}, "crontab": {}, "expiry": {},
	"fusermount": {}, "fusermount3": {}, "gpasswd": {}, "mount": {},
	"newgidmap": {}, "newgrp": {}, "newuidmap": {}, "passwd": {},
	"pkexec": {}, "ping": {}, "ping6": {}, "pmount": {}, "pumount": {},
	"sg": {}, "ssh-agent": {}, "ssh-keysign": {}, "su": {}, "sudo": {},
	"sudoedit": {}, "sudo_logsrvd": {}, "umount": {},
	"polkit-agent-helper-1": {}, "dbus-daemon-launch-helper": {},
	"unix_chkpwd": {}, "Xorg.wrap": {},
	"pam_timestamp_check": {}, "utempter": {}, "write": {}, "wall": {},
	"doas": {},
}

func depthBelow(root, path string) int {
	rel := path
	if strings.HasPrefix(rel, root) {
		rel = rel[len(root):]
	}
	return strings.Count(rel, "/")
}

func suidOutlierFinding(path, name string, mode uint32) (SecurityFinding, bool) {
	setuid := mode&0o4000 != 0
	setgid := mode&0o2000 != 0
	if !setuid && !setgid {
		return SecurityFinding{}, false
	}
	if _, ok := suidAllowlist[name]; ok {
		return SecurityFinding{}, false
	}
	bit := "setuid"
	if setgid && !setuid {
		bit = "setgid"
	}
	return SecurityFinding{
		ID:          "fs.suid_outlier",
		CheckID:     "fs.suid_outliers",
		Category:    "filesystem",
		Severity:    "medium",
		Title:       "Unexpected " + bit + " binary",
		Description: "This binary carries a privileged-execution bit but is not on the agent's allowlist of well-known distro setuid programs. Attacker-installed or vendor-bundled setuid binaries are a common privilege-escalation vector and worth a human review.",
		Evidence:    path + " mode=" + octal4(mode),
		Remediation: "Confirm the binary is expected (often it's a SUID helper from a third-party package). If not needed, remove the bit with `chmod u-s` / `chmod g-s` or uninstall the package.",
	}, true
}

// ---- helpers ----------------------------------------------------

func contains(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}
