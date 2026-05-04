// sys-security v2 — real implementation. Walks /proc + /etc
// directly via host_fs_read; the agent-side host_security_scan +
// host_list_security_checks are no longer in the path.
//
// v2 ships with a SUBSET of the legacy hardening checks:
//   - kernel.version: parse /proc/sys/kernel/osrelease, compare
//     against a "stale kernel" threshold (5.10 — most distros'
//     long-term-support lines).
//   - ssh.config:     parse /etc/ssh/sshd_config for risky settings
//     (PermitRootLogin yes / PasswordAuthentication yes).
//
// Future v3 will port the gopsutil-heavy checks (sysctl posture,
// world-writable scan, SUID outliers, kernel mitigations). The
// per-check architecture supports incremental additions: add a
// check struct + register it in CHECKS — both response endpoints
// pick it up automatically.
//
// Capabilities: fs.read of /proc + /etc.

use extism_pdk::*;
use serde::{Deserialize, Serialize};

#[cfg(target_arch = "wasm32")]
#[host_fn("platypus")]
extern "ExtismHost" {
    fn host_fs_read(path: String) -> Json<Envelope>;
}

#[derive(Deserialize, Default)]
struct Envelope {
    ok: bool,
    #[serde(default)]
    data: serde_json::Value,
    #[serde(default)]
    error: String,
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

// Each check is a (id, category, title, description, runner). The
// runner returns (findings_for_this_check, applicable). Applicable
// = false skips the check (UI shows it dimmed); the response's
// CheckResult status reflects this. Gated to wasm32 because the
// runners (check_kernel_version, check_ssh_config) read /proc + /etc
// via host_fs_read; the host build only compiles the pure
// {kernel,sshd_config}_findings layer.
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
        id: "ssh.config",
        category: "ssh",
        title: "SSH server config posture",
        description: "Reads /etc/ssh/sshd_config and flags risky settings: root login over SSH (PermitRootLogin yes) + password authentication (PasswordAuthentication yes — keys-only is the recommended posture).",
        run: check_ssh_config,
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

// ---- check implementations --------------------------------------

#[cfg(target_arch = "wasm32")]
fn check_kernel_version() -> (Vec<SecurityFinding>, bool) {
    let raw = match read_string("/proc/sys/kernel/osrelease") {
        Some(s) => s.trim().to_string(),
        None => return (Vec::new(), false), // not on linux
    };
    (kernel_findings(&raw), true)
}

// kernel_findings is the pure decision layer behind
// check_kernel_version. Parses an osrelease string, decides whether
// it predates the 5.10 LTS line, and emits a SecurityFinding when it
// does. Extracted from check_kernel_version so it's testable on the
// host without host_fs_read.
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

#[cfg(target_arch = "wasm32")]
fn check_ssh_config() -> (Vec<SecurityFinding>, bool) {
    let raw = match read_string("/etc/ssh/sshd_config") {
        Some(s) => s,
        None => return (Vec::new(), false), // sshd not installed / unreadable
    };
    (sshd_config_findings(&raw), true)
}

// sshd_config_findings is the pure decision layer behind
// check_ssh_config. Walks the lines of an sshd_config-format string,
// strips comments + whitespace, and emits a SecurityFinding for each
// risky directive (today: PermitRootLogin yes, PasswordAuthentication
// yes). Extracted so it's testable without host_fs_read.
fn sshd_config_findings(raw: &str) -> Vec<SecurityFinding> {
    let mut findings = Vec::new();
    for line in raw.lines() {
        // Strip comments + leading whitespace.
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
            _ => {}
        }
    }
    findings
}

#[cfg(target_arch = "wasm32")]
fn read_string(path: &str) -> Option<String> {
    let env: Envelope = unsafe { host_fs_read(path.to_string()).ok()?.0 };
    if !env.ok {
        return None;
    }
    env.data.as_str().map(|s| s.to_string())
}

// ============================================================
// Pure-function unit tests (host build, not wasm)
// ============================================================
//
// `cargo test --lib` runs these on the host triple. The wasm-only
// glue (host_fn declarations, plugin_fn entries, host_fs_read
// readers) is excluded by cfg(target_arch="wasm32") gates above;
// full end-to-end coverage of the wasm side lives in
// internal/agent/plugin/sys_security_integration_test.go.
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
    fn parse_kernel_only_major_minor() {
        assert_eq!(parse_kernel_major_minor("5.15"), (5, 15));
    }

    #[test]
    fn parse_kernel_garbage_returns_zero() {
        assert_eq!(parse_kernel_major_minor("not-a-version"), (0, 0));
        assert_eq!(parse_kernel_major_minor(""), (0, 0));
    }

    // ---- kernel_findings -------------------------------------

    #[test]
    fn kernel_findings_modern_no_finding() {
        // 5.10 is the boundary; 5.10+ is fine, 5.9 / 4.x are flagged.
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
    fn kernel_findings_below_5_10() {
        // 5.9 is below 5.10, also flagged.
        let findings = kernel_findings("5.9.0-foo");
        assert_eq!(findings.len(), 1);
    }

    #[test]
    fn kernel_findings_unparsable_treated_as_outdated() {
        // Garbage parses to (0, 0), which is < (5, 10). The check
        // chooses the safer "looks outdated" stance — false positive
        // beats silently passing an unparsable kernel string.
        assert_eq!(kernel_findings("garbage").len(), 1);
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
        let raw = "PermitRootLogin yes\n";
        let findings = sshd_config_findings(raw);
        assert_eq!(findings.len(), 1);
        assert_eq!(findings[0].id, "ssh.permit_root_login");
        assert_eq!(findings[0].severity, "high");
    }

    #[test]
    fn sshd_findings_password_auth_yes_medium_severity() {
        let raw = "PasswordAuthentication yes\n";
        let findings = sshd_config_findings(raw);
        assert_eq!(findings.len(), 1);
        assert_eq!(findings[0].id, "ssh.password_authentication");
        assert_eq!(findings[0].severity, "medium");
    }

    #[test]
    fn sshd_findings_both_risky_directives() {
        let raw = "\
PermitRootLogin yes
PasswordAuthentication yes
";
        let findings = sshd_config_findings(raw);
        assert_eq!(findings.len(), 2);
    }

    #[test]
    fn sshd_findings_skips_comments_and_blanks() {
        let raw = "\
# this would be risky if uncommented:
#PermitRootLogin yes

  # indented comment with no risk

PermitRootLogin no  # but this is fine
";
        // The trailing-comment line: split('#').next() → 'PermitRootLogin no  ',
        // .trim() → 'PermitRootLogin no'. Key=PermitRootLogin val=no — no finding.
        assert!(sshd_config_findings(raw).is_empty());
    }

    #[test]
    fn sshd_findings_case_sensitive_match() {
        // sshd treats keys case-insensitively, but our parser is
        // case-sensitive on purpose: it only knows about the exact
        // "PermitRootLogin" / "PasswordAuthentication" spellings the
        // distros ship in the default config. A lowercase variant
        // skips the finding — same posture as the legacy Go check.
        let raw = "permitrootlogin yes\n";
        assert!(sshd_config_findings(raw).is_empty());
    }
}
