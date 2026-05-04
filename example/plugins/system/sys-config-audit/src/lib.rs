// sys-config-audit v2 — real implementation. Walks shell history
// files + cloud-credential dotfiles directly via host_fs_*; the
// agent-side host_config_audit + the gitleaks-backed registry are
// no longer in the path.
//
// v2 ships with a SUBSET of the legacy auditor set, focused on the
// patterns most often surfaced in production environments:
//
//   shell.history     scan ~/.bash_history + ~/.zsh_history for
//                     embedded credentials (AWS access keys, GitHub
//                     tokens, generic Bearer/auth headers).
//   cloud.aws         flag the presence of ~/.aws/credentials and
//                     parse for non-empty key fields.
//   ssh.private_keys  list ~/.ssh/id_* files; flag any without a
//                     "ENCRYPTED" header (unencrypted private keys).
//
// Future v3 ports the broader gitleaks ruleset (50+ patterns) +
// the per-process env scanner (requires /proc/<pid>/environ which
// is per-uid restricted; needs root agent).
//
// Capabilities: fs.read of /home, /root, /etc.

use extism_pdk::*;
use serde::{Deserialize, Serialize};

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
    is_dir: bool,
    #[serde(default)]
    size: i64,
    #[serde(default)]
    mtime_unix: i64,
}

// ---- response shapes (mirrors v2pb encodings) -------------------

#[derive(Serialize)]
struct ConfigLeak {
    id: String,
    #[serde(rename = "auditorId")]
    auditor_id: String,
    category: String,
    risk: String,
    title: String,
    location: String,
    #[serde(rename = "match")]
    match_redacted: String,
    pattern: String,
    description: String,
    remediation: String,
    #[serde(skip_serializing_if = "Vec::is_empty")]
    references: Vec<String>,
}

#[derive(Serialize)]
struct AuditorResult {
    id: String,
    category: String,
    status: String, // "ok" | "skipped" | "error"
    #[serde(skip_serializing_if = "String::is_empty")]
    error: String,
    #[serde(rename = "elapsedMs", skip_serializing_if = "is_zero_u64")]
    elapsed_ms: u64,
    #[serde(rename = "leakCount", skip_serializing_if = "is_zero_u32")]
    leak_count: u32,
}

#[derive(Serialize)]
struct AuditResponse {
    leaks: Vec<ConfigLeak>,
    auditors: Vec<AuditorResult>,
    #[serde(rename = "startedAtUnix", skip_serializing_if = "is_zero_i64")]
    started_at_unix: i64,
    #[serde(rename = "elapsedMs", skip_serializing_if = "is_zero_u64")]
    elapsed_ms: u64,
    #[serde(skip_serializing_if = "String::is_empty")]
    error: String,
}

#[derive(Serialize)]
struct AvailableAuditor {
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
    auditors: Vec<AvailableAuditor>,
    #[serde(skip_serializing_if = "String::is_empty")]
    error: String,
}

#[derive(Deserialize)]
struct AuditRequest {
    #[serde(default, rename = "auditorIds", alias = "auditor_ids")]
    auditor_ids: Vec<String>,
    #[serde(default)]
    categories: Vec<String>,
}

fn is_zero_u32(x: &u32) -> bool { *x == 0 }
fn is_zero_u64(x: &u64) -> bool { *x == 0 }
fn is_zero_i64(x: &i64) -> bool { *x == 0 }

// ---- registered auditors ----------------------------------------

struct Auditor {
    id: &'static str,
    category: &'static str,
    title: &'static str,
    description: &'static str,
    run: fn() -> Vec<ConfigLeak>,
}

const AUDITORS: &[Auditor] = &[
    Auditor {
        id: "shell.history",
        category: "shell",
        title: "Shell history credential scan",
        description: "Scans ~/.bash_history and ~/.zsh_history for embedded AWS access keys, GitHub tokens, and Authorization headers.",
        run: audit_shell_history,
    },
    Auditor {
        id: "cloud.aws",
        category: "cloud",
        title: "AWS credentials file",
        description: "Flags the presence of ~/.aws/credentials with non-empty key fields. Sensitive even when filesystem permissions are correct because the file should ideally not exist on a server.",
        run: audit_aws_credentials,
    },
    Auditor {
        id: "ssh.private_keys",
        category: "ssh",
        title: "Unencrypted SSH private keys",
        description: "Lists ~/.ssh/id_* files and flags any whose first line is NOT '-----BEGIN OPENSSH PRIVATE KEY-----' followed by 'ENCRYPTED' marker — i.e. keys not protected by a passphrase.",
        run: audit_ssh_private_keys,
    },
];

// ---- entry points -----------------------------------------------

#[plugin_fn]
pub fn list_config_auditors(_: ()) -> FnResult<String> {
    let auditors: Vec<AvailableAuditor> = AUDITORS
        .iter()
        .map(|a| AvailableAuditor {
            id: a.id.to_string(),
            category: a.category.to_string(),
            applicable: true,
            title: a.title.to_string(),
            description: a.description.to_string(),
            references: Vec::new(),
        })
        .collect();
    Ok(serde_json::to_string(&ListResponse {
        auditors,
        error: String::new(),
    })?)
}

#[plugin_fn]
pub fn config_audit(req: Json<AuditRequest>) -> FnResult<String> {
    let want = &req.0.auditor_ids;
    let want_cats = &req.0.categories;
    let mut leaks = Vec::new();
    let mut results = Vec::new();
    for a in AUDITORS {
        if !want.is_empty() && !want.iter().any(|w| w == a.id) {
            continue;
        }
        if !want_cats.is_empty() && !want_cats.iter().any(|w| w == a.category) {
            continue;
        }
        let mut found = (a.run)();
        let count = found.len() as u32;
        results.push(AuditorResult {
            id: a.id.to_string(),
            category: a.category.to_string(),
            status: "ok".to_string(),
            error: String::new(),
            elapsed_ms: 0,
            leak_count: count,
        });
        leaks.append(&mut found);
    }
    Ok(serde_json::to_string(&AuditResponse {
        leaks,
        auditors: results,
        started_at_unix: 0,
        elapsed_ms: 0,
        error: String::new(),
    })?)
}

// ---- auditor implementations ------------------------------------

fn audit_shell_history() -> Vec<ConfigLeak> {
    // v2 uses simple substring + length scans instead of regex to
    // keep the wasm artefact small and the runtime panic-free in
    // sandboxed targets. Patterns:
    //   - AKIA prefix      — AWS access key id
    //   - ghp_/gho_/ghs_   — GitHub personal-access-token
    //   - "Bearer " marker — Authorization header (any token)
    // v3 will port a Rust regex pass once the runtime quirks are
    // resolved; the wider gitleaks ruleset lands then too.
    let mut out = Vec::new();
    for path in candidate_history_files() {
        let raw = match read_string(&path) {
            Some(v) => v,
            None => continue,
        };
        for (line_no, line) in raw.lines().enumerate() {
            // AWS access key: "AKIA" followed by 16 alphanumeric.
            if let Some(m) = scan_aws_key(line) {
                out.push(make_leak("aws-access-key-id", "high",
                    "AWS access key id (AKIA...) embedded in shell history",
                    &path, line_no + 1, &m));
                continue; // one finding per line
            }
            // GitHub token: "ghp_" / "gho_" / "ghs_" / "ghu_" + 20+ chars.
            if let Some(m) = scan_github_token(line) {
                out.push(make_leak("github-token", "high",
                    "GitHub personal-access-token (ghp_/gho_/ghs_/ghu_) in shell history",
                    &path, line_no + 1, &m));
                continue;
            }
            // Bearer header: case-insensitive "Bearer " then a token.
            if let Some(m) = scan_bearer(line) {
                out.push(make_leak("bearer-token", "medium",
                    "Authorization: Bearer header in shell history",
                    &path, line_no + 1, &m));
            }
        }
    }
    out
}

fn make_leak(pat_id: &str, risk: &str, title: &str, path: &str, line_no: usize, m: &str) -> ConfigLeak {
    ConfigLeak {
        id: format!("shell.history.{}", pat_id),
        auditor_id: "shell.history".to_string(),
        category: "shell".to_string(),
        risk: risk.to_string(),
        title: title.to_string(),
        location: format!("{}:{}", path, line_no),
        match_redacted: redact(m),
        pattern: format!("substring:{}", pat_id),
        description: "Plaintext credentials in shell history can be exfiltrated via .bash_history backup, screen-sharing, or anyone with read access to the operator's home directory.".to_string(),
        remediation: format!("Rotate the credential. Clear the relevant lines from {} (or the whole file). For long-term defence: set HISTCONTROL=ignorespace + prefix sensitive commands with a leading space.", path),
        references: Vec::new(),
    }
}

// scan_aws_key looks for "AKIA" + 16 [A-Z0-9] chars. Returns the
// full 20-char match.
fn scan_aws_key(line: &str) -> Option<String> {
    let bytes = line.as_bytes();
    let pat = b"AKIA";
    let mut i = 0;
    while i + pat.len() + 16 <= bytes.len() {
        if &bytes[i..i + pat.len()] == pat {
            let key = &bytes[i..i + pat.len() + 16];
            if key[pat.len()..].iter().all(|c| c.is_ascii_uppercase() || c.is_ascii_digit()) {
                return Some(String::from_utf8_lossy(key).to_string());
            }
        }
        i += 1;
    }
    None
}

// scan_github_token: gh[opsu]_ + 20+ alphanumeric/_.
fn scan_github_token(line: &str) -> Option<String> {
    let bytes = line.as_bytes();
    let mut i = 0;
    while i + 4 <= bytes.len() {
        if bytes[i] == b'g' && bytes[i + 1] == b'h'
            && matches!(bytes[i + 2], b'p' | b'o' | b's' | b'u') && bytes[i + 3] == b'_' {
            let mut j = i + 4;
            while j < bytes.len() && (bytes[j].is_ascii_alphanumeric() || bytes[j] == b'_') {
                j += 1;
            }
            if j - (i + 4) >= 20 {
                return Some(String::from_utf8_lossy(&bytes[i..j]).to_string());
            }
        }
        i += 1;
    }
    None
}

// scan_bearer: case-insensitive "Bearer " + 20+ token chars.
fn scan_bearer(line: &str) -> Option<String> {
    let lower = line.to_ascii_lowercase();
    let idx = lower.find("bearer ")?;
    let token_start = idx + "bearer ".len();
    let bytes = line.as_bytes();
    let mut j = token_start;
    while j < bytes.len() && (bytes[j].is_ascii_alphanumeric()
        || bytes[j] == b'.' || bytes[j] == b'_' || bytes[j] == b'-') {
        j += 1;
    }
    if j - token_start >= 20 {
        Some(String::from_utf8_lossy(&bytes[idx..j]).to_string())
    } else {
        None
    }
}

// candidate_history_files enumerates /home/<user> + /root looking
// for known shell history filenames. Paths that don't exist or
// aren't readable are skipped silently.
fn candidate_history_files() -> Vec<String> {
    let mut out = Vec::new();
    // Root user.
    for f in &[".bash_history", ".zsh_history"] {
        out.push(format!("/root/{}", f));
    }
    // Per-user homes.
    if let Some(env) = list_dir("/home") {
        for e in env.into_iter().filter(|e| e.is_dir) {
            for f in &[".bash_history", ".zsh_history"] {
                out.push(format!("/home/{}/{}", e.name, f));
            }
        }
    }
    out
}

fn audit_aws_credentials() -> Vec<ConfigLeak> {
    let mut out = Vec::new();
    for path in candidate_aws_files() {
        let raw = match read_string(&path) {
            Some(v) => v,
            None => continue,
        };
        for (line_no, line) in raw.lines().enumerate() {
            // Case-insensitive prefix check on the line, ignoring
            // whitespace + comments.
            let trimmed = line.trim();
            if trimmed.is_empty() || trimmed.starts_with('#') {
                continue;
            }
            let lower = trimmed.to_ascii_lowercase();
            if !lower.starts_with("aws_access_key_id") {
                continue;
            }
            let eq = match trimmed.find('=') { Some(i) => i, None => continue };
            let key = trimmed[eq + 1..].trim();
            if key.is_empty() {
                continue;
            }
            out.push(ConfigLeak {
                id: "cloud.aws.credentials_present".to_string(),
                auditor_id: "cloud.aws".to_string(),
                category: "cloud".to_string(),
                risk: "high".to_string(),
                title: "AWS credentials file present on host".to_string(),
                location: format!("{}:{}", path, line_no + 1),
                match_redacted: redact(key),
                pattern: "behavior:aws-credentials-file".to_string(),
                description: "An AWS access key in ~/.aws/credentials is the most common cloud-account hijack vector. Servers should use IAM-role credentials (instance metadata) instead.".to_string(),
                remediation: "Remove the credentials file. Switch to IAM-role-based auth (EC2 instance profile, EKS service account, or similar). Rotate the leaked key in AWS console.".to_string(),
                references: Vec::new(),
            });
        }
    }
    out
}

fn candidate_aws_files() -> Vec<String> {
    let mut out = Vec::new();
    out.push("/root/.aws/credentials".to_string());
    if let Some(env) = list_dir("/home") {
        for e in env.into_iter().filter(|e| e.is_dir) {
            out.push(format!("/home/{}/.aws/credentials", e.name));
        }
    }
    out
}

fn audit_ssh_private_keys() -> Vec<ConfigLeak> {
    let mut out = Vec::new();
    let dirs = candidate_ssh_dirs();
    for dir in dirs {
        let entries = match list_dir(&dir) {
            Some(v) => v,
            None => continue,
        };
        for e in entries.into_iter().filter(|e| !e.is_dir && e.name.starts_with("id_") && !e.name.ends_with(".pub")) {
            let path = format!("{}/{}", dir, e.name);
            let raw = match read_string(&path) {
                Some(v) => v,
                None => continue,
            };
            // OpenSSH passphrase-protected keys carry an "ENCRYPTED"
            // marker in the body. PEM-format encrypted keys carry
            // "Proc-Type: 4,ENCRYPTED". Anything else = unencrypted
            // private key.
            if !raw.contains("ENCRYPTED") {
                out.push(ConfigLeak {
                    id: "ssh.private_key.unencrypted".to_string(),
                    auditor_id: "ssh.private_keys".to_string(),
                    category: "ssh".to_string(),
                    risk: "medium".to_string(),
                    title: format!("Unencrypted SSH private key at {}", path),
                    location: path.clone(),
                    match_redacted: format!("{} (no ENCRYPTED marker in body)", e.name),
                    pattern: "behavior:unencrypted-ssh-private-key".to_string(),
                    description: "An SSH private key without a passphrase grants full access to whoever can read the file. ssh-add can cache the passphrase to avoid re-typing.".to_string(),
                    remediation: format!("Add a passphrase: ssh-keygen -p -f {}. Use ssh-agent / ssh-add to avoid prompts every connection.", path),
                    references: Vec::new(),
                });
            }
        }
    }
    out
}

fn candidate_ssh_dirs() -> Vec<String> {
    let mut out = Vec::new();
    out.push("/root/.ssh".to_string());
    if let Some(env) = list_dir("/home") {
        for e in env.into_iter().filter(|e| e.is_dir) {
            out.push(format!("/home/{}/.ssh", e.name));
        }
    }
    out
}

// ---- helpers ----------------------------------------------------

// redact matches the agent-side pattern: first 4 + last 4 chars of
// the matched string, with "****" between them. Matches gopsutil-
// era output so the UI's MatchCell renderer doesn't need a
// per-source variant.
fn redact(s: &str) -> String {
    let chars: Vec<char> = s.chars().collect();
    if chars.len() <= 8 {
        return "*".repeat(chars.len());
    }
    let head: String = chars.iter().take(4).collect();
    let tail_rev: Vec<char> = chars.iter().rev().take(4).copied().collect();
    let tail: String = tail_rev.into_iter().rev().collect();
    format!("{}****{}", head, tail)
}

fn read_string(path: &str) -> Option<String> {
    let env: Envelope = unsafe { host_fs_read(path.to_string()).ok()?.0 };
    if !env.ok {
        return None;
    }
    env.data.as_str().map(|s| s.to_string())
}

fn list_dir(path: &str) -> Option<Vec<DirEntryJSON>> {
    let env: Envelope = unsafe { host_fs_listdir(path.to_string()).ok()?.0 };
    if !env.ok {
        return None;
    }
    serde_json::from_value(env.data).ok()
}
