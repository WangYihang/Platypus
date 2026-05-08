// sys-file-caps-linux — Linux file-capabilities inventory.
//
// Shells `getcap -r <root> 2>/dev/null` (Debian / Ubuntu put it at
// /sbin/getcap; RHEL / Fedora at /usr/sbin/getcap) for each scan
// root, parses the `<path> caps` lines, classifies entries against
// a curated allowlist + risk table, returns a FileCapsListResponse.
//
// Companion to sys-security's fs.suid_outliers check — SUID and
// file-capabilities are orthogonal privilege-escalation surfaces.
// `ping` no longer carries SUID; it carries cap_net_raw=ep. An
// attacker placing a custom binary with cap_dac_read_search
// (read-any-file) would slip past sys-security but show up here.

use extism_pdk::*;
use serde::{Deserialize, Serialize};

#[cfg(target_arch = "wasm32")]
#[host_fn("platypus")]
extern "ExtismHost" {
    fn host_exec(req: String) -> Json<Envelope>;
}

#[derive(Deserialize, Default)]
struct Envelope {
    ok: bool,
    #[serde(default)]
    data: serde_json::Value,
    #[serde(default)]
    error: String,
}

#[derive(Serialize)]
struct ExecRequest {
    command: String,
    args: Vec<String>,
    #[serde(rename = "timeout_ms")]
    timeout_ms: u32,
}

#[derive(Deserialize, Default)]
struct ExecResponse {
    #[serde(default, rename = "exit_code")]
    exit_code: i32,
    #[serde(default)]
    stdout: String,
    #[serde(default)]
    stderr: String,
}

// ---- request / response wire shapes ----

#[derive(Deserialize, Default)]
struct ListRequest {
    #[serde(default)]
    roots: Vec<String>,
    #[serde(default)]
    max_results: u32,
    #[serde(default)]
    include_allowlisted: bool,
}

#[derive(Serialize, Default)]
struct ListResponse {
    entries: Vec<FileCap>,
    backend: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    error: String,
}

#[derive(Serialize, Default, Debug, PartialEq)]
struct FileCap {
    path: String,
    caps: String,
    #[serde(skip_serializing_if = "is_false")]
    allowlisted: bool,
    risk: String,
}

fn is_false(b: &bool) -> bool {
    !*b
}

const DEFAULT_ROOTS: &[&str] = &[
    "/usr/bin",
    "/usr/sbin",
    "/usr/local/bin",
    "/usr/local/sbin",
    "/bin",
    "/sbin",
    "/opt",
];

const GETCAP_PATHS: &[&str] = &["/sbin/getcap", "/usr/sbin/getcap"];

const DEFAULT_MAX: u32 = 5_000;

// ---- entry point ----

#[cfg(target_arch = "wasm32")]
#[plugin_fn]
pub fn list_file_caps(req: Json<ListRequest>) -> FnResult<String> {
    let r = req.0;
    let getcap = match detect_getcap() {
        Some(p) => p,
        None => {
            return Ok(serde_json::to_string(&ListResponse {
                entries: Vec::new(),
                backend: String::new(),
                error: "getcap_not_installed".to_string(),
            })?)
        }
    };
    let roots: Vec<String> = if r.roots.is_empty() {
        DEFAULT_ROOTS.iter().map(|s| (*s).to_string()).collect()
    } else {
        r.roots
    };
    let mut all_lines = String::new();
    let mut last_err = String::new();
    for root in &roots {
        match run_getcap(&getcap, root, 25_000) {
            Ok(resp) => {
                if !resp.stdout.is_empty() {
                    all_lines.push_str(&resp.stdout);
                    if !resp.stdout.ends_with('\n') {
                        all_lines.push('\n');
                    }
                }
                // getcap exits non-zero when a root doesn't exist;
                // we treat that as benign (skip).
                if resp.exit_code != 0 && !resp.stderr.is_empty() {
                    last_err = format!("{root}: {}", resp.stderr.trim());
                }
            }
            Err(e) => {
                last_err = format!("{root}: {e}");
            }
        }
    }
    let mut entries = parse_getcap_output(&all_lines);
    if !r.include_allowlisted {
        entries.retain(|e| !e.allowlisted);
    }
    let cap = if r.max_results == 0 { DEFAULT_MAX } else { r.max_results };
    if entries.len() as u32 > cap {
        entries.truncate(cap as usize);
    }
    // Suppress last_err when at least some entries came through —
    // partial success is more useful than a misleading error string.
    let err = if !entries.is_empty() { String::new() } else { last_err };
    Ok(serde_json::to_string(&ListResponse {
        entries,
        backend: "getcap".to_string(),
        error: err,
    })?)
}

// ---- backend detection ----

#[cfg(target_arch = "wasm32")]
fn detect_getcap() -> Option<String> {
    for p in GETCAP_PATHS {
        if probe_command_exists(p) {
            return Some((*p).to_string());
        }
    }
    None
}

#[cfg(target_arch = "wasm32")]
fn probe_command_exists(path: &str) -> bool {
    // Use `/bin/sh -c 'command -v <path>'` rather than execing the
    // binary directly — getcap with no args prints help to stderr
    // and exits non-zero on some distros, which we'd then misread.
    let req = ExecRequest {
        command: "/bin/sh".to_string(),
        args: vec!["-c".to_string(), format!("command -v {path} >/dev/null 2>&1")],
        timeout_ms: 2_000,
    };
    let body = match serde_json::to_string(&req) {
        Ok(s) => s,
        Err(_) => return false,
    };
    let env: Envelope = match unsafe { host_exec(body) } {
        Ok(j) => j.0,
        Err(_) => return false,
    };
    if !env.ok {
        return false;
    }
    let resp: ExecResponse = match serde_json::from_value(env.data) {
        Ok(r) => r,
        Err(_) => return false,
    };
    resp.exit_code == 0
}

#[cfg(target_arch = "wasm32")]
fn run_getcap(getcap: &str, root: &str, timeout_ms: u32) -> Result<ExecResponse, String> {
    // Wrap in /bin/sh so we can swallow getcap's stderr noise on
    // unreadable subdirs; the plugin doesn't get to read /root/ as
    // a non-root user, getcap chatters about that, we don't care.
    let req = ExecRequest {
        command: "/bin/sh".to_string(),
        args: vec![
            "-c".to_string(),
            format!("{getcap} -r {root} 2>/dev/null"),
        ],
        timeout_ms,
    };
    let body = serde_json::to_string(&req).map_err(|e| format!("encode_exec_req: {e}"))?;
    let env: Envelope = unsafe {
        host_exec(body)
            .map_err(|e| format!("host_exec: {e}"))?
            .0
    };
    if !env.ok {
        return Err(env.error);
    }
    serde_json::from_value(env.data).map_err(|e| format!("decode_exec_resp: {e}"))
}

// ---- pure parsers ----

// parse_getcap_output handles two getcap output formats:
//   v1 (libcap < 2.32):  "/usr/bin/ping = cap_net_raw+ep"
//   v2 (libcap >= 2.32): "/usr/bin/ping cap_net_raw=ep"
// Both parse into the same `path` + `caps` pair. Empty/invalid
// lines are skipped silently.
fn parse_getcap_output(stdout: &str) -> Vec<FileCap> {
    let mut out = Vec::new();
    for line in stdout.lines() {
        let line = line.trim();
        if line.is_empty() {
            continue;
        }
        let (path, caps) = match split_getcap_line(line) {
            Some(v) => v,
            None => continue,
        };
        let allowlisted = is_allowlisted(&path);
        let risk = classify_caps(&caps);
        out.push(FileCap { path, caps, allowlisted, risk });
    }
    out
}

fn split_getcap_line(line: &str) -> Option<(String, String)> {
    // v1 format: "<path> = <caps>" — split on " = " (with spaces).
    if let Some(idx) = line.find(" = ") {
        let path = line[..idx].trim().to_string();
        let caps = line[idx + 3..].trim().to_string();
        if !path.is_empty() && !caps.is_empty() {
            return Some((path, caps));
        }
    }
    // v2 format: "<path> <caps>" — last whitespace-separated token
    // is the cap string. The path may itself contain spaces (rare on
    // distros but not impossible), so we split on the LAST space.
    if let Some(idx) = line.rfind(' ') {
        let path = line[..idx].trim().to_string();
        let caps = line[idx + 1..].trim().to_string();
        if !path.is_empty() && !caps.is_empty() && caps.contains("cap_") {
            return Some((path, caps));
        }
    }
    None
}

// ALLOWLIST is the set of basenames known to legitimately ship with
// file capabilities on common distros. Mirrors the `suidAllowlist`
// pattern in sys-security/fs.suid_outliers.
const ALLOWLIST: &[&str] = &[
    "ping",
    "ping4",
    "ping6",
    "mtr",
    "mtr-packet",
    "traceroute",
    "traceroute6",
    "tracepath",
    "tracepath6",
    "arping",
    "clockdiff",
    "fping",
    "fping6",
    // Wireshark / tcpdump helpers.
    "dumpcap",
    "wireshark",
    // Container / namespace helpers.
    "newuidmap",
    "newgidmap",
];

fn is_allowlisted(path: &str) -> bool {
    let base = match path.rsplit('/').next() {
        Some(b) => b,
        None => return false,
    };
    ALLOWLIST.iter().any(|a| *a == base)
}

// classify_caps maps the raw cap string to a "low" / "medium" /
// "high" risk tag. Intent: an operator scrolling the response sees
// "high" entries first as the most worth investigating. The
// classification is deliberately coarse — getcap only tells us the
// declared cap set, not how the binary uses it.
fn classify_caps(caps: &str) -> String {
    let lower = caps.to_ascii_lowercase();
    // High: near-root capabilities. cap_dac_* defeats file perms;
    // cap_sys_* is a grab bag of dangerous knobs.
    const HIGH: &[&str] = &[
        "cap_dac_read_search",
        "cap_dac_override",
        "cap_sys_admin",
        "cap_sys_module",
        "cap_sys_ptrace",
        "cap_setuid",
        "cap_setgid",
        "cap_chown",
    ];
    if HIGH.iter().any(|c| lower.contains(c)) {
        return "high".to_string();
    }
    // Medium: powerful network / admin capabilities that aren't
    // root-equivalent but are far more than ping needs.
    const MEDIUM: &[&str] = &[
        "cap_net_admin",
        "cap_sys_time",
        "cap_sys_chroot",
        "cap_audit_write",
        "cap_audit_control",
        "cap_kill",
        "cap_linux_immutable",
    ];
    if MEDIUM.iter().any(|c| lower.contains(c)) {
        return "medium".to_string();
    }
    // Low: cap_net_raw (ping et al), cap_net_bind_service (low ports
    // for unprivileged daemons), cap_ipc_lock (mlock for crypto).
    "low".to_string()
}

// ============================================================
// Pure-function unit tests (host build, not wasm)
// ============================================================
#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn split_v1_format() {
        let (p, c) = split_getcap_line("/usr/bin/ping = cap_net_raw+ep").unwrap();
        assert_eq!(p, "/usr/bin/ping");
        assert_eq!(c, "cap_net_raw+ep");
    }

    #[test]
    fn split_v2_format() {
        let (p, c) = split_getcap_line("/usr/bin/ping cap_net_raw=ep").unwrap();
        assert_eq!(p, "/usr/bin/ping");
        assert_eq!(c, "cap_net_raw=ep");
    }

    #[test]
    fn split_v2_multi_caps() {
        let (p, c) = split_getcap_line(
            "/usr/bin/dumpcap cap_dac_override,cap_net_admin,cap_net_raw=eip",
        )
        .unwrap();
        assert_eq!(p, "/usr/bin/dumpcap");
        assert!(c.contains("cap_net_admin"));
    }

    #[test]
    fn split_empty_returns_none() {
        assert!(split_getcap_line("").is_none());
        assert!(split_getcap_line("   ").is_none());
    }

    #[test]
    fn split_no_caps_returns_none() {
        // A path with no cap_* token shouldn't fall through to the
        // last-space rule and produce garbage.
        assert!(split_getcap_line("/usr/bin/ls").is_none());
    }

    #[test]
    fn is_allowlisted_basename() {
        assert!(is_allowlisted("/usr/bin/ping"));
        assert!(is_allowlisted("/bin/ping"));
        assert!(is_allowlisted("/usr/sbin/mtr-packet"));
        assert!(!is_allowlisted("/opt/vendor/custom-helper"));
    }

    #[test]
    fn classify_high_caps() {
        assert_eq!(classify_caps("cap_sys_admin=eip"), "high");
        assert_eq!(classify_caps("cap_dac_read_search=ep"), "high");
        assert_eq!(classify_caps("cap_sys_module=ep"), "high");
    }

    #[test]
    fn classify_medium_caps() {
        assert_eq!(classify_caps("cap_net_admin=ep"), "medium");
        assert_eq!(classify_caps("cap_sys_time=ep"), "medium");
    }

    #[test]
    fn classify_low_caps() {
        assert_eq!(classify_caps("cap_net_raw=ep"), "low");
        assert_eq!(classify_caps("cap_net_bind_service=ep"), "low");
    }

    #[test]
    fn classify_high_wins_when_mixed() {
        // A cap set with both high and low takes the high label.
        assert_eq!(classify_caps("cap_net_raw,cap_sys_admin=ep"), "high");
    }

    #[test]
    fn parse_full_output_three_entries() {
        let stdout = "\
/usr/bin/ping cap_net_raw=ep
/usr/bin/mtr-packet cap_net_raw=ep
/opt/vendor/helper cap_dac_read_search=eip
";
        let entries = parse_getcap_output(stdout);
        assert_eq!(entries.len(), 3);
        // Allowlist tagging.
        let ping = entries.iter().find(|e| e.path.ends_with("/ping")).unwrap();
        assert!(ping.allowlisted);
        assert_eq!(ping.risk, "low");
        let mtr = entries.iter().find(|e| e.path.contains("mtr-packet")).unwrap();
        assert!(mtr.allowlisted);
        let outlier = entries.iter().find(|e| e.path.contains("vendor")).unwrap();
        assert!(!outlier.allowlisted);
        assert_eq!(outlier.risk, "high");
    }

    #[test]
    fn parse_skips_blank_and_garbage_lines() {
        let stdout = "\

/usr/bin/ping cap_net_raw=ep

random line with no caps
";
        let entries = parse_getcap_output(stdout);
        assert_eq!(entries.len(), 1);
    }

    #[test]
    fn parse_v1_format_lines() {
        let stdout = "/usr/bin/ping = cap_net_raw+ep\n";
        let entries = parse_getcap_output(stdout);
        assert_eq!(entries.len(), 1);
        assert_eq!(entries[0].path, "/usr/bin/ping");
        assert!(entries[0].caps.contains("cap_net_raw"));
    }
}
