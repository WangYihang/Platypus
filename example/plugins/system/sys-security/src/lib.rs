// sys-security v3 — real implementation. Walks /proc + /etc + /sys
// + the standard binary directories directly via host_fs_*; the
// agent-side host_security_scan + host_list_security_checks are no
// longer in the path.
//
// v3 ships with six checks ported from internal/agent/security/:
//
//   - kernel.version      parse /proc/sys/kernel/osrelease, compare
//                         against the 5.10 LTS floor.
//   - ssh.config          parse /etc/ssh/sshd_config for risky
//                         settings (PermitRootLogin yes,
//                         PasswordAuthentication yes,
//                         PermitEmptyPasswords yes, X11Forwarding yes).
//   - sysctl.posture      read a curated set of /proc/sys keys (10
//                         keys covering kptr_restrict, dmesg_restrict,
//                         unprivileged_bpf, rp_filter, accept/send
//                         redirects, tcp_syncookies, fs.protected_*,
//                         fs.suid_dumpable) and flag any that diverge
//                         from the recommended hardened value.
//   - fs.path_writable    stat each PATH directory + the standard
//                         fallback set; flag world-writable + non-
//                         sticky directories (the LPE classic).
//   - fs.suid_outliers    listdir each binary directory, flag SUID/
//                         SGID files outside a tight allowlist of
//                         well-known distro helpers. Capped at 20 000
//                         visited entries.
//   - kernel.mitigations  read every file under
//                         /sys/devices/system/cpu/vulnerabilities/,
//                         flag any whose body does not start with
//                         "Mitigation:" / "Not affected".
//
// Capabilities: fs.read of /proc, /etc, /sys, /usr, /usr/local, /bin,
// /sbin, /snap/bin, /opt.

use extism_pdk::*;
use serde::{Deserialize, Serialize};

#[cfg(target_arch = "wasm32")]
#[host_fn("platypus")]
extern "ExtismHost" {
    fn host_fs_read(path: String) -> Json<Envelope>;
    fn host_fs_listdir(path: String) -> Json<Envelope>;
}

#[derive(Deserialize, Default)]
struct Envelope {
    ok: bool,
    #[serde(default)]
    data: serde_json::Value,
    #[serde(default)]
    error: String,
}

#[derive(Deserialize)]
struct DirEntryJSON {
    name: String,
    #[serde(default)]
    is_dir: bool,
    // The host folds POSIX SUID/SGID/sticky bits into the low 12 of
    // mode (see internal/agent/plugin/host_fs.go:posixMode), so a
    // value of 0o4755 means "rwxr-xr-x + setuid".
    #[serde(default)]
    mode: u32,
}

// ---- response shapes (mirrors v2pb encodings) -------------------

#[derive(Serialize)]
struct SecurityFinding {
    id: String,
    #[serde(rename = "checkId")]
    check_id: String,
    category: String,
    severity: String,
    title: String,
    description: String,
    evidence: String,
    remediation: String,
    #[serde(skip_serializing_if = "Vec::is_empty")]
    references: Vec<String>,
}

#[derive(Serialize)]
struct CheckResult {
    id: String,
    category: String,
    status: String, // "ok" | "skipped" | "error"
    #[serde(skip_serializing_if = "String::is_empty")]
    error: String,
    #[serde(rename = "elapsedMs", skip_serializing_if = "is_zero_u64")]
    elapsed_ms: u64,
    #[serde(rename = "findingCount", skip_serializing_if = "is_zero_u32")]
    finding_count: u32,
}

#[derive(Serialize)]
struct ScanResponse {
    findings: Vec<SecurityFinding>,
    checks: Vec<CheckResult>,
    #[serde(rename = "startedAtUnix", skip_serializing_if = "is_zero_i64")]
    started_at_unix: i64,
    #[serde(rename = "elapsedMs", skip_serializing_if = "is_zero_u64")]
    elapsed_ms: u64,
    #[serde(skip_serializing_if = "String::is_empty")]
    error: String,
}

#[derive(Serialize)]
struct AvailableCheck {
    id: String,
    category: String,
    applicable: bool,
    title: String,
    description: String,
    #[serde(skip_serializing_if = "Vec::is_empty")]
    references: Vec<String>,
}

#[derive(Serialize)]
struct ListResponse {
    checks: Vec<AvailableCheck>,
    #[serde(skip_serializing_if = "String::is_empty")]
    error: String,
}

#[derive(Deserialize)]
struct ScanRequest {
    // protojson encodes proto field check_ids as camelCase
    // checkIds. Accept both — operators may hand-craft requests in
    // snake_case directly via the REST API.
    #[serde(default, alias = "check_ids")]
    #[serde(rename = "checkIds")]
    check_ids: Vec<String>,
    #[serde(default)]
    categories: Vec<String>,
}

fn is_zero_u32(x: &u32) -> bool { *x == 0 }
fn is_zero_u64(x: &u64) -> bool { *x == 0 }
fn is_zero_i64(x: &i64) -> bool { *x == 0 }

// ---- registered checks ------------------------------------------

#[cfg(target_arch = "wasm32")]
struct Check {
    id: &'static str,
    category: &'static str,
    title: &'static str,
    description: &'static str,
    run: fn() -> (Vec<SecurityFinding>, bool),
}

#[cfg(target_arch = "wasm32")]
const CHECKS: &[Check] = &[
    Check {
        id: "kernel.version",
        category: "kernel",
        title: "Kernel version is recent",
        description: "Hosts on a kernel older than 5.10 are missing several years of CVE fixes; long-term-support lines start at 5.10 (Mar 2021). This check parses /proc/sys/kernel/osrelease.",
        run: check_kernel_version,
    },
    Check {
        id: "kernel.mitigations",
        category: "kernel",
        title: "CPU vulnerability mitigations active",
        description: "Reads each file under /sys/devices/system/cpu/vulnerabilities/ and flags any whose first token is not 'Mitigation:' or 'Not affected'. Catches Spectre/Meltdown/MDS/L1TF/etc. running with mitigations disabled (often via mitigations=off boot flag).",
        run: check_kernel_mitigations,
    },
    Check {
        id: "ssh.config",
        category: "ssh",
        title: "SSH server config posture",
        description: "Reads /etc/ssh/sshd_config and flags risky settings: PermitRootLogin yes, PasswordAuthentication yes, PermitEmptyPasswords yes, X11Forwarding yes.",
        run: check_ssh_config,
    },
    Check {
        id: "sysctl.posture",
        category: "sysctl",
        title: "Sysctl hardening posture",
        description: "Reads a curated set of /proc/sys keys (kptr_restrict, dmesg_restrict, unprivileged_bpf_disabled, rp_filter, accept_redirects, send_redirects, tcp_syncookies, fs.protected_hardlinks/symlinks, fs.suid_dumpable). One finding per misaligned key.",
        run: check_sysctl_posture,
    },
    Check {
        id: "fs.path_writable",
        category: "filesystem",
        title: "World-writable directories on PATH",
        description: "Stats each PATH directory (plus the standard fallback set /usr/local/sbin, /usr/local/bin, /usr/sbin, /usr/bin, /sbin, /bin, /snap/bin) and flags any that are world-writable AND non-sticky — the textbook setup for an unprivileged user to swap out a binary that root will later invoke.",
        run: check_fs_path_writable,
    },
    Check {
        id: "fs.suid_outliers",
        category: "filesystem",
        title: "Unexpected setuid/setgid binaries",
        description: "Lists /usr/bin, /usr/sbin, /usr/local/bin, /usr/local/sbin, /bin, /sbin, /opt and flags setuid/setgid binaries not on the allowlist of well-known distro helpers. Capped at 20,000 visited entries.",
        run: check_fs_suid_outliers,
    },
];

// ---- entry points -----------------------------------------------

#[cfg(target_arch = "wasm32")]
#[plugin_fn]
pub fn list_security_checks(_: ()) -> FnResult<String> {
    let checks: Vec<AvailableCheck> = CHECKS
        .iter()
        .map(|c| AvailableCheck {
            id: c.id.to_string(),
            category: c.category.to_string(),
            applicable: true,
            title: c.title.to_string(),
            description: c.description.to_string(),
            references: Vec::new(),
        })
        .collect();
    Ok(serde_json::to_string(&ListResponse {
        checks,
        error: String::new(),
    })?)
}

#[cfg(target_arch = "wasm32")]
#[plugin_fn]
pub fn security_scan(req: Json<ScanRequest>) -> FnResult<String> {
    let want = &req.0.check_ids;
    let want_cats = &req.0.categories;
    let mut findings = Vec::new();
    let mut results = Vec::new();
    for c in CHECKS {
        if !want.is_empty() && !want.iter().any(|w| w == c.id) {
            continue;
        }
        if !want_cats.is_empty() && !want_cats.iter().any(|w| w == c.category) {
            continue;
        }
        let (mut fs, applicable) = (c.run)();
        let status = if applicable { "ok" } else { "skipped" };
        let count = fs.len() as u32;
        results.push(CheckResult {
            id: c.id.to_string(),
            category: c.category.to_string(),
            status: status.to_string(),
            error: String::new(),
            elapsed_ms: 0,
            finding_count: count,
        });
        findings.append(&mut fs);
    }
    Ok(serde_json::to_string(&ScanResponse {
        findings,
        checks: results,
        started_at_unix: 0,
        elapsed_ms: 0,
        error: String::new(),
    })?)
}

// ============================================================
// kernel.version
// ============================================================

#[cfg(target_arch = "wasm32")]
fn check_kernel_version() -> (Vec<SecurityFinding>, bool) {
    let raw = match read_string("/proc/sys/kernel/osrelease") {
        Some(s) => s.trim().to_string(),
        None => return (Vec::new(), false), // not on linux
    };
    (kernel_findings(&raw), true)
}

fn kernel_findings(osrelease: &str) -> Vec<SecurityFinding> {
    let (major, minor) = parse_kernel_major_minor(osrelease);
    let outdated = (major, minor) < (5, 10);
    if !outdated {
        return Vec::new();
    }
    vec![SecurityFinding {
        id: "kernel.version.outdated".to_string(),
        check_id: "kernel.version".to_string(),
        category: "kernel".to_string(),
        severity: "medium".to_string(),
        title: format!("Kernel {} is older than 5.10", osrelease),
        description: "Long-term-support kernel lines start at 5.10 (Mar 2021). Hosts on older kernels miss several years of CVE fixes.".to_string(),
        evidence: format!("/proc/sys/kernel/osrelease = {}", osrelease),
        remediation: "Upgrade to a distribution release that ships a 5.10+ kernel; reboot.".to_string(),
        references: Vec::new(),
    }]
}

fn parse_kernel_major_minor(s: &str) -> (u32, u32) {
    let mut parts = s.split('.');
    let major: u32 = parts.next().and_then(|v| v.parse().ok()).unwrap_or(0);
    let minor: u32 = parts.next().and_then(|v| v.parse().ok()).unwrap_or(0);
    (major, minor)
}

// ============================================================
// kernel.mitigations
// ============================================================

#[cfg(target_arch = "wasm32")]
fn check_kernel_mitigations() -> (Vec<SecurityFinding>, bool) {
    const ROOT: &str = "/sys/devices/system/cpu/vulnerabilities";
    let entries = match list_dir(ROOT) {
        Some(v) => v,
        None => return (Vec::new(), false), // /sys not exposed (containers)
    };
    let mut out = Vec::new();
    for e in entries.into_iter().filter(|e| !e.is_dir) {
        let path = format!("{}/{}", ROOT, e.name);
        let body = match read_string(&path) {
            Some(v) => v,
            None => continue,
        };
        out.extend(mitigation_findings(&e.name, &body));
    }
    (out, true)
}

// mitigation_findings is the pure decision layer. The kernel writes
// one of these shapes per vulnerability file:
//   "Not affected"
//   "Mitigation: <details>"
//   "Vulnerable[: <details>]"
//   "Vulnerable; <details>"
// The first two are fine; anything else is a finding.
fn mitigation_findings(name: &str, body: &str) -> Vec<SecurityFinding> {
    let trimmed = body.trim();
    if trimmed.is_empty() {
        return Vec::new();
    }
    if trimmed.starts_with("Not affected") || trimmed.starts_with("Mitigation:") {
        return Vec::new();
    }
    vec![SecurityFinding {
        id: format!("kernel.mitigations.{}", name),
        check_id: "kernel.mitigations".to_string(),
        category: "kernel".to_string(),
        severity: "high".to_string(),
        title: format!("CPU vulnerability '{}' not mitigated", name),
        description: "The kernel reports this CPU vulnerability as exploitable on this host. Common cause: the operator passed `mitigations=off` on the kernel command line, or the microcode update needed for the mitigation isn't installed.".to_string(),
        evidence: format!("/sys/devices/system/cpu/vulnerabilities/{} = {}", name, trimmed),
        remediation: "Remove `mitigations=off` (and any per-vuln overrides like `nospectre_v2`) from the kernel cmdline; install the latest microcode package; reboot.".to_string(),
        references: vec!["https://www.kernel.org/doc/html/latest/admin-guide/hw-vuln/index.html".to_string()],
    }]
}

// ============================================================
// ssh.config
// ============================================================

#[cfg(target_arch = "wasm32")]
fn check_ssh_config() -> (Vec<SecurityFinding>, bool) {
    let raw = match read_string("/etc/ssh/sshd_config") {
        Some(s) => s,
        None => return (Vec::new(), false), // sshd not installed / unreadable
    };
    (sshd_config_findings(&raw), true)
}

fn sshd_config_findings(raw: &str) -> Vec<SecurityFinding> {
    let mut findings = Vec::new();
    for line in raw.lines() {
        let line = line.split('#').next().unwrap_or("").trim();
        if line.is_empty() {
            continue;
        }
        let mut parts = line.split_whitespace();
        let key = parts.next().unwrap_or("");
        let val = parts.next().unwrap_or("");
        match (key, val) {
            ("PermitRootLogin", "yes") => findings.push(SecurityFinding {
                id: "ssh.permit_root_login".to_string(),
                check_id: "ssh.config".to_string(),
                category: "ssh".to_string(),
                severity: "high".to_string(),
                title: "SSH server allows root login".to_string(),
                description: "PermitRootLogin yes lets anyone with the root password (or root SSH key) authenticate as root directly. Best practice: log in as a non-root user + use sudo.".to_string(),
                evidence: "/etc/ssh/sshd_config: PermitRootLogin yes".to_string(),
                remediation: "Set PermitRootLogin to \"no\" (or \"prohibit-password\") in /etc/ssh/sshd_config and restart sshd.".to_string(),
                references: Vec::new(),
            }),
            ("PasswordAuthentication", "yes") => findings.push(SecurityFinding {
                id: "ssh.password_authentication".to_string(),
                check_id: "ssh.config".to_string(),
                category: "ssh".to_string(),
                severity: "medium".to_string(),
                title: "SSH server allows password authentication".to_string(),
                description: "Password auth is brute-forceable. Public-key authentication has no such failure mode and is the recommended posture for production hosts.".to_string(),
                evidence: "/etc/ssh/sshd_config: PasswordAuthentication yes".to_string(),
                remediation: "Distribute SSH keys to users, set PasswordAuthentication to \"no\" in /etc/ssh/sshd_config, restart sshd.".to_string(),
                references: Vec::new(),
            }),
            ("PermitEmptyPasswords", "yes") => findings.push(SecurityFinding {
                id: "ssh.permit_empty_passwords".to_string(),
                check_id: "ssh.config".to_string(),
                category: "ssh".to_string(),
                severity: "critical".to_string(),
                title: "SSH server allows empty passwords".to_string(),
                description: "PermitEmptyPasswords yes lets accounts with no password set log in via SSH with no auth at all.".to_string(),
                evidence: "/etc/ssh/sshd_config: PermitEmptyPasswords yes".to_string(),
                remediation: "Set PermitEmptyPasswords to \"no\" in /etc/ssh/sshd_config and restart sshd.".to_string(),
                references: Vec::new(),
            }),
            ("X11Forwarding", "yes") => findings.push(SecurityFinding {
                id: "ssh.x11_forwarding".to_string(),
                check_id: "ssh.config".to_string(),
                category: "ssh".to_string(),
                severity: "low".to_string(),
                title: "SSH X11 forwarding enabled".to_string(),
                description: "X11Forwarding yes exposes the host's X server through the SSH session. A malicious or compromised client can read keystrokes and screen contents from any X11-aware program the user runs.".to_string(),
                evidence: "/etc/ssh/sshd_config: X11Forwarding yes".to_string(),
                remediation: "Set X11Forwarding to \"no\" in /etc/ssh/sshd_config unless your operators rely on remote X11 apps.".to_string(),
                references: Vec::new(),
            }),
            _ => {}
        }
    }
    findings
}

// ============================================================
// sysctl.posture
// ============================================================

struct SysctlExpectation {
    key: &'static str,
    want: &'static [&'static str],
    severity: &'static str,
    title: &'static str,
    desc: &'static str,
    fix: &'static str,
}

const SYSCTL_EXPECTATIONS: &[SysctlExpectation] = &[
    SysctlExpectation {
        key: "kernel.kptr_restrict",
        want: &["2"],
        severity: "medium",
        title: "Kernel pointers exposed via /proc",
        desc: "kernel.kptr_restrict<2 lets unprivileged processes read raw kernel addresses, which weakens KASLR.",
        fix: "Set kernel.kptr_restrict=2 in /etc/sysctl.d/99-hardening.conf and run sysctl --system.",
    },
    SysctlExpectation {
        key: "kernel.dmesg_restrict",
        want: &["1"],
        severity: "low",
        title: "dmesg readable by unprivileged users",
        desc: "kernel.dmesg_restrict=0 lets any local user read kernel ring buffer contents (often leaks pointers, KASLR offsets, and driver state).",
        fix: "Set kernel.dmesg_restrict=1 in /etc/sysctl.d/99-hardening.conf.",
    },
    SysctlExpectation {
        key: "kernel.unprivileged_bpf_disabled",
        // 2 = "1, but locked until reboot" — strictly stronger than 1.
        want: &["1", "2"],
        severity: "high",
        title: "Unprivileged eBPF enabled",
        desc: "kernel.unprivileged_bpf_disabled=0 leaves a recurring CVE-class privilege escalation surface (CVE-2021-3490, CVE-2022-23222, …).",
        fix: "Set kernel.unprivileged_bpf_disabled=1 unless you specifically need it for an eBPF userspace tool that runs unprivileged.",
    },
    SysctlExpectation {
        key: "net.ipv4.conf.all.rp_filter",
        // 1 = strict, 2 = loose; both enforce reverse-path filtering.
        want: &["1", "2"],
        severity: "medium",
        title: "Reverse path filtering disabled",
        desc: "net.ipv4.conf.all.rp_filter=0 allows spoofed source addresses to reach local services on multi-homed hosts.",
        fix: "Set net.ipv4.conf.all.rp_filter=1 (strict mode), or 2 if asymmetric routes are expected.",
    },
    SysctlExpectation {
        key: "net.ipv4.conf.all.accept_redirects",
        want: &["0"],
        severity: "medium",
        title: "ICMP redirects accepted",
        desc: "Accepting ICMP redirects lets a local attacker rewrite the host's routing table.",
        fix: "Set net.ipv4.conf.all.accept_redirects=0 and net.ipv6.conf.all.accept_redirects=0.",
    },
    SysctlExpectation {
        key: "net.ipv4.conf.all.send_redirects",
        want: &["0"],
        severity: "low",
        title: "Sending ICMP redirects",
        desc: "Hosts that aren't routers shouldn't emit ICMP redirects; doing so leaks topology information.",
        fix: "Set net.ipv4.conf.all.send_redirects=0.",
    },
    SysctlExpectation {
        key: "net.ipv4.tcp_syncookies",
        want: &["1", "2"],
        severity: "low",
        title: "TCP SYN cookies disabled",
        desc: "SYN cookies are the primary defense against SYN-flood denial of service.",
        fix: "Set net.ipv4.tcp_syncookies=1.",
    },
    SysctlExpectation {
        key: "fs.protected_hardlinks",
        want: &["1"],
        severity: "medium",
        title: "Hardlink protection disabled",
        desc: "fs.protected_hardlinks=0 enables a class of /tmp race-condition privilege escalations.",
        fix: "Set fs.protected_hardlinks=1.",
    },
    SysctlExpectation {
        key: "fs.protected_symlinks",
        want: &["1"],
        severity: "medium",
        title: "Symlink protection disabled",
        desc: "fs.protected_symlinks=0 enables symlink-attack TOCTOU bugs in setuid programs and /tmp consumers.",
        fix: "Set fs.protected_symlinks=1.",
    },
    SysctlExpectation {
        key: "fs.suid_dumpable",
        want: &["0"],
        severity: "medium",
        title: "Setuid binaries write core dumps",
        desc: "fs.suid_dumpable!=0 lets crashed setuid programs leave core files containing privileged memory state.",
        fix: "Set fs.suid_dumpable=0.",
    },
];

#[cfg(target_arch = "wasm32")]
fn check_sysctl_posture() -> (Vec<SecurityFinding>, bool) {
    // Cheap applicability probe: if /proc/sys is not exposed (e.g.
    // restricted container), skip the whole check.
    if read_string("/proc/sys/kernel/osrelease").is_none() {
        return (Vec::new(), false);
    }
    let mut findings = Vec::new();
    for e in SYSCTL_EXPECTATIONS {
        let path = sysctl_path(e.key);
        let got = match read_string(&path) {
            Some(v) => normalize_sysctl(&v),
            None => continue, // missing keys are common on minimal containers
        };
        findings.extend(sysctl_finding(e, &got));
    }
    (findings, true)
}

fn sysctl_path(key: &str) -> String {
    format!("/proc/sys/{}", key.replace('.', "/"))
}

// normalize_sysctl collapses whitespace-separated tokens to a single
// space-joined string so equality comparisons are meaningful for
// multi-value sysctls (e.g. kernel.printk).
fn normalize_sysctl(raw: &str) -> String {
    raw.split_whitespace().collect::<Vec<_>>().join(" ")
}

fn sysctl_finding(e: &SysctlExpectation, got: &str) -> Vec<SecurityFinding> {
    if e.want.iter().any(|w| *w == got) {
        return Vec::new();
    }
    vec![SecurityFinding {
        id: format!("sysctl.{}", e.key),
        check_id: "sysctl.posture".to_string(),
        category: "sysctl".to_string(),
        severity: e.severity.to_string(),
        title: e.title.to_string(),
        description: e.desc.to_string(),
        evidence: format!("{} = {} (expected {})", e.key, got, e.want.join(" or ")),
        remediation: e.fix.to_string(),
        references: Vec::new(),
    }]
}

// ============================================================
// fs.path_writable
// ============================================================

#[cfg(target_arch = "wasm32")]
fn check_fs_path_writable() -> (Vec<SecurityFinding>, bool) {
    // Read /proc/1/environ to discover the live PATH (host's PID 1).
    // Falls back to the static fallback set if /proc/1 isn't readable.
    let mut paths: Vec<String> = parse_path_env(&read_string("/proc/1/environ").unwrap_or_default());
    for p in PATH_FALLBACK {
        if !paths.iter().any(|x| x == p) {
            paths.push((*p).to_string());
        }
    }

    let mut findings = Vec::new();
    for d in &paths {
        // host_fs_listdir resolves the directory via stat under the
        // hood; we use it to read the parent's mode bits in one round
        // trip. That requires listing the parent — to avoid that we
        // just stat each via a probe read (returns is_directory error
        // on a dir). Instead, use list_one(d) which returns the dir's
        // own entry from its parent listing.
        let entry = match stat_via_parent(d) {
            Some(e) => e,
            None => continue,
        };
        if !entry.is_dir {
            continue;
        }
        if entry.mode & 0o002 == 0 {
            continue;
        }
        // World-writable + sticky (1777) is the /tmp pattern and is
        // intentional; flag only the non-sticky case.
        if entry.mode & 0o1000 != 0 {
            continue;
        }
        findings.push(path_writable_finding(d, entry.mode));
    }
    (findings, true)
}

fn path_writable_finding(dir: &str, mode: u32) -> SecurityFinding {
    SecurityFinding {
        id: "fs.path_writable".to_string(),
        check_id: "fs.path_writable".to_string(),
        category: "filesystem".to_string(),
        severity: "critical".to_string(),
        title: "World-writable directory on PATH".to_string(),
        description: "Any local user can replace binaries in this directory. The next time root (or any other account) invokes a command that resolves here, it executes attacker-controlled code.".to_string(),
        evidence: format!("{} mode={:04o}", dir, mode),
        remediation: format!("chmod o-w {dir}; investigate how the directory got created with this mode (often a packaging bug or a misconfigured deploy script)."),
        references: Vec::new(),
    }
}

// PATH_FALLBACK is the static directory list a hardened distro layout
// is expected to use. Mirrors candidatePathDirs() in the legacy Go
// implementation.
const PATH_FALLBACK: &[&str] = &[
    "/usr/local/sbin",
    "/usr/local/bin",
    "/usr/sbin",
    "/usr/bin",
    "/sbin",
    "/bin",
    "/snap/bin",
];

// parse_path_env extracts PATH= from a NUL-separated /proc/<pid>/environ
// blob.
fn parse_path_env(environ: &str) -> Vec<String> {
    for entry in environ.split('\0') {
        if let Some(rest) = entry.strip_prefix("PATH=") {
            let mut seen: Vec<String> = Vec::new();
            for p in rest.split(':') {
                if p.is_empty() {
                    continue;
                }
                if !seen.iter().any(|x| x == p) {
                    seen.push(p.to_string());
                }
            }
            return seen;
        }
    }
    Vec::new()
}

// stat_via_parent fetches a directory's entry from its parent's
// listing — host_fs_listdir is the only way to get mode bits without
// shipping a host_fs_stat call. Returns None when the parent isn't
// listable (unallowed path / non-existent / permission denied).
#[cfg(target_arch = "wasm32")]
fn stat_via_parent(path: &str) -> Option<DirEntryJSON> {
    let (parent, name) = split_parent(path)?;
    let entries = list_dir(&parent)?;
    entries.into_iter().find(|e| e.name == name)
}

fn split_parent(path: &str) -> Option<(String, String)> {
    let trimmed = path.trim_end_matches('/');
    if trimmed.is_empty() || trimmed == "/" {
        return None;
    }
    let idx = trimmed.rfind('/')?;
    let parent = if idx == 0 { "/".to_string() } else { trimmed[..idx].to_string() };
    let name = trimmed[idx + 1..].to_string();
    if name.is_empty() {
        return None;
    }
    Some((parent, name))
}

// ============================================================
// fs.suid_outliers
// ============================================================

const SUID_SCAN_ROOTS: &[&str] = &[
    "/usr/bin",
    "/usr/sbin",
    "/usr/local/bin",
    "/usr/local/sbin",
    "/bin",
    "/sbin",
    "/opt",
];

const SUID_SCAN_CAP: usize = 20_000;

// SUID_ALLOWLIST is the set of binary base names that legitimately
// ship as setuid/setgid on common Linux distros. Anything else
// surfaces for operator review.
const SUID_ALLOWLIST: &[&str] = &[
    "chage", "chfn", "chsh", "crontab", "expiry",
    "fusermount", "fusermount3", "gpasswd", "mount",
    "newgidmap", "newgrp", "newuidmap", "passwd",
    "pkexec", "ping", "ping6", "pmount", "pumount",
    "sg", "ssh-agent", "ssh-keysign", "su", "sudo",
    "sudoedit", "sudo_logsrvd", "umount",
    "polkit-agent-helper-1", "dbus-daemon-launch-helper",
    "unix_chkpwd", "Xorg.wrap",
    "pam_timestamp_check", "utempter", "write", "wall",
    "doas",
];

#[cfg(target_arch = "wasm32")]
fn check_fs_suid_outliers() -> (Vec<SecurityFinding>, bool) {
    let mut findings = Vec::new();
    let mut visited: usize = 0;

    'outer: for root in SUID_SCAN_ROOTS {
        let mut stack: Vec<String> = vec![(*root).to_string()];
        while let Some(dir) = stack.pop() {
            if visited >= SUID_SCAN_CAP {
                break 'outer;
            }
            let entries = match list_dir(&dir) {
                Some(v) => v,
                None => continue,
            };
            for e in entries {
                visited += 1;
                if visited >= SUID_SCAN_CAP {
                    break 'outer;
                }
                let path = format!("{}/{}", dir, e.name);
                if e.is_dir {
                    // /opt commonly nests one level deep; cap at 4
                    // levels under each scan root to keep runtime
                    // bounded on pathological filesystems.
                    if depth_below(root, &path) < 4 {
                        stack.push(path);
                    }
                    continue;
                }
                if let Some(f) = suid_outlier_finding(&path, &e.name, e.mode) {
                    findings.push(f);
                }
            }
        }
    }
    (findings, true)
}

fn depth_below(root: &str, path: &str) -> usize {
    let rel = path.strip_prefix(root).unwrap_or(path);
    rel.matches('/').count()
}

fn suid_outlier_finding(path: &str, name: &str, mode: u32) -> Option<SecurityFinding> {
    // Mode comes from the host's posixMode helper: SUID=0o4000,
    // SGID=0o2000.
    let setuid = mode & 0o4000 != 0;
    let setgid = mode & 0o2000 != 0;
    if !setuid && !setgid {
        return None;
    }
    if SUID_ALLOWLIST.iter().any(|allowed| *allowed == name) {
        return None;
    }
    let bit = if setuid { "setuid" } else { "setgid" };
    Some(SecurityFinding {
        id: "fs.suid_outlier".to_string(),
        check_id: "fs.suid_outliers".to_string(),
        category: "filesystem".to_string(),
        severity: "medium".to_string(),
        title: format!("Unexpected {} binary", bit),
        description: "This binary carries a privileged-execution bit but is not on the agent's allowlist of well-known distro setuid programs. Attacker-installed or vendor-bundled setuid binaries are a common privilege-escalation vector and worth a human review.".to_string(),
        evidence: format!("{} mode={:04o}", path, mode),
        remediation: "Confirm the binary is expected (often it's a SUID helper from a third-party package). If not needed, remove the bit with `chmod u-s` / `chmod g-s` or uninstall the package.".to_string(),
        references: Vec::new(),
    })
}

// ============================================================
// host helpers (wasm-only — pure code can be tested on the host)
// ============================================================

#[cfg(target_arch = "wasm32")]
fn read_string(path: &str) -> Option<String> {
    let env: Envelope = unsafe { host_fs_read(path.to_string()).ok()?.0 };
    if !env.ok {
        return None;
    }
    env.data.as_str().map(|s| s.to_string())
}

#[cfg(target_arch = "wasm32")]
fn list_dir(path: &str) -> Option<Vec<DirEntryJSON>> {
    let env: Envelope = unsafe { host_fs_listdir(path.to_string()).ok()?.0 };
    if !env.ok {
        return None;
    }
    serde_json::from_value(env.data).ok()
}

// ============================================================
// Pure-function unit tests (host build, not wasm)
// ============================================================
#[cfg(test)]
mod tests {
    use super::*;

    // ---- parse_kernel_major_minor ----------------------------

    #[test]
    fn parse_kernel_typical_distro_release() {
        assert_eq!(parse_kernel_major_minor("5.15.0-1024-generic"), (5, 15));
        assert_eq!(parse_kernel_major_minor("6.1.0-13-amd64"), (6, 1));
        assert_eq!(parse_kernel_major_minor("4.19.0-25-cloud-amd64"), (4, 19));
    }

    #[test]
    fn parse_kernel_bare_semver() {
        assert_eq!(parse_kernel_major_minor("5.10.0"), (5, 10));
    }

    #[test]
    fn parse_kernel_garbage_returns_zero() {
        assert_eq!(parse_kernel_major_minor("not-a-version"), (0, 0));
        assert_eq!(parse_kernel_major_minor(""), (0, 0));
    }

    // ---- kernel_findings -------------------------------------

    #[test]
    fn kernel_findings_modern_no_finding() {
        assert!(kernel_findings("5.10.0").is_empty());
        assert!(kernel_findings("5.15.0-1024-generic").is_empty());
        assert!(kernel_findings("6.1.0").is_empty());
    }

    #[test]
    fn kernel_findings_outdated_emits_one() {
        let findings = kernel_findings("4.19.0-25-cloud-amd64");
        assert_eq!(findings.len(), 1);
        let f = &findings[0];
        assert_eq!(f.id, "kernel.version.outdated");
        assert_eq!(f.check_id, "kernel.version");
        assert_eq!(f.severity, "medium");
        assert!(f.evidence.contains("4.19.0"));
    }

    #[test]
    fn kernel_findings_unparsable_treated_as_outdated() {
        // (0, 0) < (5, 10) — false positive beats silently passing.
        assert_eq!(kernel_findings("garbage").len(), 1);
    }

    // ---- mitigation_findings ---------------------------------

    #[test]
    fn mitigation_findings_not_affected_clean() {
        assert!(mitigation_findings("meltdown", "Not affected\n").is_empty());
    }

    #[test]
    fn mitigation_findings_mitigation_clean() {
        assert!(mitigation_findings("spectre_v2", "Mitigation: Retpolines, IBPB\n").is_empty());
    }

    #[test]
    fn mitigation_findings_vulnerable_flagged() {
        let f = mitigation_findings("spectre_v1", "Vulnerable: __user pointer sanitization");
        assert_eq!(f.len(), 1);
        assert_eq!(f[0].id, "kernel.mitigations.spectre_v1");
        assert_eq!(f[0].severity, "high");
        assert!(f[0].evidence.contains("Vulnerable"));
    }

    #[test]
    fn mitigation_findings_empty_body_skipped() {
        assert!(mitigation_findings("meltdown", "").is_empty());
        assert!(mitigation_findings("meltdown", "   \n").is_empty());
    }

    // ---- sshd_config_findings --------------------------------

    #[test]
    fn sshd_findings_clean_config_no_findings() {
        let raw = "\
# default config
PermitRootLogin no
PasswordAuthentication no
PubkeyAuthentication yes
";
        assert!(sshd_config_findings(raw).is_empty());
    }

    #[test]
    fn sshd_findings_permit_root_yes_high_severity() {
        let f = sshd_config_findings("PermitRootLogin yes\n");
        assert_eq!(f.len(), 1);
        assert_eq!(f[0].id, "ssh.permit_root_login");
        assert_eq!(f[0].severity, "high");
    }

    #[test]
    fn sshd_findings_password_auth_yes_medium_severity() {
        let f = sshd_config_findings("PasswordAuthentication yes\n");
        assert_eq!(f.len(), 1);
        assert_eq!(f[0].severity, "medium");
    }

    #[test]
    fn sshd_findings_permit_empty_passwords_critical() {
        let f = sshd_config_findings("PermitEmptyPasswords yes\n");
        assert_eq!(f.len(), 1);
        assert_eq!(f[0].id, "ssh.permit_empty_passwords");
        assert_eq!(f[0].severity, "critical");
    }

    #[test]
    fn sshd_findings_x11_forwarding_low() {
        let f = sshd_config_findings("X11Forwarding yes\n");
        assert_eq!(f.len(), 1);
        assert_eq!(f[0].id, "ssh.x11_forwarding");
        assert_eq!(f[0].severity, "low");
    }

    #[test]
    fn sshd_findings_all_four_directives() {
        let raw = "\
PermitRootLogin yes
PasswordAuthentication yes
PermitEmptyPasswords yes
X11Forwarding yes
";
        assert_eq!(sshd_config_findings(raw).len(), 4);
    }

    #[test]
    fn sshd_findings_skips_comments_and_blanks() {
        let raw = "\
# this would be risky if uncommented:
#PermitRootLogin yes
PermitRootLogin no  # but this is fine
";
        assert!(sshd_config_findings(raw).is_empty());
    }

    // ---- sysctl_finding --------------------------------------

    #[test]
    fn sysctl_finding_match_clean() {
        let e = &SYSCTL_EXPECTATIONS[0]; // kernel.kptr_restrict want=2
        assert!(sysctl_finding(e, "2").is_empty());
    }

    #[test]
    fn sysctl_finding_mismatch_emits_one() {
        let e = &SYSCTL_EXPECTATIONS[0];
        let f = sysctl_finding(e, "0");
        assert_eq!(f.len(), 1);
        assert_eq!(f[0].id, "sysctl.kernel.kptr_restrict");
        assert_eq!(f[0].check_id, "sysctl.posture");
        assert!(f[0].evidence.contains("0"));
        assert!(f[0].evidence.contains("expected 2"));
    }

    #[test]
    fn sysctl_finding_alternate_acceptable_value() {
        // unprivileged_bpf_disabled: both 1 and 2 are accepted.
        let e = SYSCTL_EXPECTATIONS
            .iter()
            .find(|x| x.key == "kernel.unprivileged_bpf_disabled")
            .unwrap();
        assert!(sysctl_finding(e, "1").is_empty());
        assert!(sysctl_finding(e, "2").is_empty());
        assert_eq!(sysctl_finding(e, "0").len(), 1);
    }

    #[test]
    fn sysctl_path_format() {
        assert_eq!(sysctl_path("kernel.kptr_restrict"), "/proc/sys/kernel/kptr_restrict");
        assert_eq!(sysctl_path("net.ipv4.conf.all.rp_filter"), "/proc/sys/net/ipv4/conf/all/rp_filter");
    }

    #[test]
    fn normalize_sysctl_collapses_whitespace() {
        assert_eq!(normalize_sysctl("4   4\t1   7"), "4 4 1 7");
        assert_eq!(normalize_sysctl("\t2\n"), "2");
    }

    // ---- parse_path_env --------------------------------------

    #[test]
    fn parse_path_env_typical_environ() {
        let environ = "USER=root\0HOME=/root\0PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin\0LANG=C\0";
        assert_eq!(
            parse_path_env(environ),
            vec!["/usr/local/sbin", "/usr/local/bin", "/usr/sbin", "/usr/bin"]
        );
    }

    #[test]
    fn parse_path_env_no_path_returns_empty() {
        assert!(parse_path_env("USER=root\0HOME=/root\0").is_empty());
    }

    #[test]
    fn parse_path_env_dedupes() {
        let environ = "PATH=/bin:/usr/bin:/bin\0";
        assert_eq!(parse_path_env(environ), vec!["/bin", "/usr/bin"]);
    }

    #[test]
    fn parse_path_env_empty_segments_skipped() {
        let environ = "PATH=/bin::/usr/bin\0";
        assert_eq!(parse_path_env(environ), vec!["/bin", "/usr/bin"]);
    }

    // ---- path_writable_finding -------------------------------

    #[test]
    fn path_writable_finding_renders_octal_mode() {
        let f = path_writable_finding("/opt/custom-bin", 0o757);
        assert_eq!(f.severity, "critical");
        assert!(f.evidence.contains("/opt/custom-bin"));
        assert!(f.evidence.contains("0757"));
    }

    // ---- split_parent ----------------------------------------

    #[test]
    fn split_parent_typical_paths() {
        assert_eq!(
            split_parent("/usr/bin/sudo"),
            Some(("/usr/bin".to_string(), "sudo".to_string()))
        );
        assert_eq!(
            split_parent("/etc"),
            Some(("/".to_string(), "etc".to_string()))
        );
    }

    #[test]
    fn split_parent_root_returns_none() {
        assert_eq!(split_parent("/"), None);
        assert_eq!(split_parent(""), None);
    }

    #[test]
    fn split_parent_strips_trailing_slash() {
        assert_eq!(
            split_parent("/usr/bin/"),
            Some(("/usr".to_string(), "bin".to_string()))
        );
    }

    // ---- suid_outlier_finding --------------------------------

    #[test]
    fn suid_outlier_no_special_bits_skipped() {
        assert!(suid_outlier_finding("/usr/bin/ls", "ls", 0o755).is_none());
    }

    #[test]
    fn suid_outlier_allowlisted_skipped() {
        assert!(suid_outlier_finding("/usr/bin/sudo", "sudo", 0o4755).is_none());
        assert!(suid_outlier_finding("/usr/bin/passwd", "passwd", 0o4755).is_none());
    }

    #[test]
    fn suid_outlier_setuid_outsider_flagged() {
        let f = suid_outlier_finding("/opt/vendor/helper", "helper", 0o4755).unwrap();
        assert_eq!(f.id, "fs.suid_outlier");
        assert_eq!(f.severity, "medium");
        assert!(f.title.contains("setuid"));
        assert!(f.evidence.contains("/opt/vendor/helper"));
        assert!(f.evidence.contains("4755"));
    }

    #[test]
    fn suid_outlier_setgid_outsider_flagged() {
        let f = suid_outlier_finding("/opt/vendor/helper", "helper", 0o2755).unwrap();
        assert!(f.title.contains("setgid"));
    }

    #[test]
    fn depth_below_counts_segments() {
        assert_eq!(depth_below("/opt", "/opt"), 0);
        assert_eq!(depth_below("/opt", "/opt/foo"), 1);
        assert_eq!(depth_below("/opt", "/opt/a/b/c"), 3);
    }
}
