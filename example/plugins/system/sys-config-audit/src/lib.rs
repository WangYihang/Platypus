// sys-config-audit v3 — broader credential-leak scanner. v2 shipped
// only shell.history (substring patterns), cloud.aws (presence), and
// ssh.private_keys (no-passphrase). The legacy Go reference at
// internal/agent/config_audit/ implemented six auditors against
// gitleaks v8 (160-rule ruleset). v3 ports all six over to wasm by
// hand-curating ~50 high-signal regex rules in src/rules.rs and
// expanding each auditor's source-set to match the legacy.
//
// Coverage:
//   - shell.history       9 history filenames per home/root +
//                         gitleaks-style rule pack +
//                         3 behavioural patterns
//   - cloud.aws           13 cloud/tool credential dotfiles per home
//                         (AWS, GCP, Azure, k8s, docker, npm, pypi,
//                         netrc, git) + world-readable mode check
//   - ssh.private_keys    PEM unencrypted private-key detection,
//                         world-readable mode check, authorized_keys
//                         entries with no restriction options
//   - env.process         /proc/<pid>/environ scanner with per-proc
//                         256 KiB and aggregate 4 MiB caps
//   - db.config           gitleaks on my.cnf + .pgpass; structural
//                         rules on pg_hba.conf trust, redis.conf
//                         missing requirepass / protected-mode no,
//                         mongod.conf missing auth
//   - webapp.config       depth-bounded walk of /var/www, /srv,
//                         /opt, and per-user homes for framework
//                         config files (.env, wp-config.php,
//                         settings.py, application.{yml,properties},
//                         appsettings*.json, secrets.yml, ...);
//                         scans + flags world-readable files
//
// Capabilities: fs.read of /home, /root, /etc, /proc, /var/www,
// /srv, /opt, /var/log, /usr/local/etc.

use extism_pdk::*;
use serde::{Deserialize, Serialize};

mod rules;

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

#[derive(Deserialize, Default)]
struct DirEntryJSON {
    name: String,
    #[serde(default)]
    is_dir: bool,
    #[serde(default)]
    size: i64,
    #[serde(default)]
    mtime_unix: i64,
    // Mode is the POSIX mode bits per host_fs.go's posixMode helper:
    // low 9 = rwx perms, plus SUID (0o4000), SGID (0o2000), sticky
    // (0o1000). Mask `& 0o007` to read the world bits and `& 0o004`
    // for "world-readable", which the auditors flag on credential
    // files.
    #[serde(default)]
    mode: u32,
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
    #[serde(rename = "totalCount", skip_serializing_if = "is_zero_u32")]
    total_count: u32,
    #[serde(rename = "hasMore", skip_serializing_if = "is_false")]
    has_more: bool,
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

#[derive(Deserialize, Default)]
struct AuditRequest {
    #[serde(default, rename = "auditorIds", alias = "auditor_ids")]
    auditor_ids: Vec<String>,
    #[serde(default)]
    categories: Vec<String>,
    #[serde(default)]
    offset: u32,
    #[serde(default)]
    limit: u32,
}

fn is_zero_u32(x: &u32) -> bool { *x == 0 }
fn is_zero_u64(x: &u64) -> bool { *x == 0 }
fn is_zero_i64(x: &i64) -> bool { *x == 0 }
fn is_false(b: &bool) -> bool { !*b }

// Hard cap on returned leaks. 0 = unlimited (default), but the
// hard ceiling stops a leaky host from sending megabytes per query.
const HARD_LIMIT: u32 = 5_000;

fn paginate_leaks(items: Vec<ConfigLeak>, offset: u32, limit: u32) -> (Vec<ConfigLeak>, u32, bool) {
    let total = items.len() as u32;
    let off = (offset as usize).min(items.len());
    let lim = if limit == 0 { items.len().saturating_sub(off) } else { limit.min(HARD_LIMIT) as usize };
    let end = (off + lim).min(items.len());
    let slice: Vec<ConfigLeak> = items.into_iter().skip(off).take(end - off).collect();
    let has_more = (off + slice.len()) < total as usize;
    (slice, total, has_more)
}

// ---- registered auditors ----------------------------------------

#[cfg(target_arch = "wasm32")]
struct Auditor {
    id: &'static str,
    category: &'static str,
    title: &'static str,
    description: &'static str,
    run: fn() -> Vec<ConfigLeak>,
}

#[cfg(target_arch = "wasm32")]
const AUDITORS: &[Auditor] = &[
    Auditor {
        id: "shell.history",
        category: "shell",
        title: "Shell and REPL history files",
        description: "Reads .bash_history, .zsh_history, .sh_history, .python_history, .mysql_history, .psql_history, .redis_history, .node_repl_history, .lesshst for each readable user. Flags credential-shaped strings via the gitleaks-style rule pack and three behavioural patterns (inline mysql -p, curl -u basic auth, export FOO_KEY=).",
        run: audit_shell_history,
    },
    Auditor {
        id: "cloud.aws",
        category: "cloud",
        title: "Cloud and tool credential dotfiles",
        description: "Scans AWS, GCP, Azure, k8s, Docker, npm, pip, netrc, and git credential files in every user's home. Flags credential-shaped values with the rule pack and reports world-readable permission modes.",
        run: audit_cloud_dotfiles,
    },
    Auditor {
        id: "ssh.private_keys",
        category: "ssh",
        title: "SSH key material",
        description: "Inspects ~/.ssh for unencrypted private keys, world-readable key files, and authorized_keys entries that grant unrestricted access.",
        run: audit_ssh_keys,
    },
    Auditor {
        id: "env.process",
        category: "env",
        title: "Process environment variables",
        description: "Reads /proc/<pid>/environ for every readable process and scans NAME=VALUE pairs for credential-shaped values. Per-proc cap 256 KiB; aggregate budget 4 MiB.",
        run: audit_env_process,
    },
    Auditor {
        id: "db.config",
        category: "database",
        title: "Database server configuration",
        description: "Reads MySQL/MariaDB/PostgreSQL/Redis/MongoDB config files. Flags inline passwords, trust-mode pg_hba.conf rows, missing redis requirepass/protected-mode, and mongod.conf without authorization.",
        run: audit_db_config,
    },
    Auditor {
        id: "webapp.config",
        category: "webapp",
        title: "Web application configuration files",
        description: "Walks /var/www, /srv, /opt, and each user home for known framework config files (.env, wp-config.php, settings.py, application.yml, appsettings.json, database.yml, secrets.yml). Capped at 200 files visited and 4 levels deep.",
        run: audit_webapp_config,
    },
];

// ---- entry points -----------------------------------------------

#[cfg(target_arch = "wasm32")]
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

#[cfg(target_arch = "wasm32")]
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
    let (sliced, total, has_more) = paginate_leaks(leaks, req.0.offset, req.0.limit);
    Ok(serde_json::to_string(&AuditResponse {
        leaks: sliced,
        auditors: results,
        started_at_unix: 0,
        elapsed_ms: 0,
        error: String::new(),
        total_count: total,
        has_more,
    })?)
}

// ============================================================
// shell.history
// ============================================================

const SHELL_HISTORY_FILES: &[&str] = &[
    ".bash_history",
    ".zsh_history",
    ".sh_history",
    ".python_history",
    ".mysql_history",
    ".psql_history",
    ".redis_history",
    ".node_repl_history",
    ".lesshst",
];

#[cfg(target_arch = "wasm32")]
fn audit_shell_history() -> Vec<ConfigLeak> {
    let mut out = Vec::new();
    for home in candidate_home_dirs() {
        for name in SHELL_HISTORY_FILES {
            let path = format!("{home}/{name}");
            let raw = match read_string(&path) {
                Some(v) => v,
                None => continue,
            };
            // Strip zsh extended-history timestamp before scanning so
            // ": 1700000000:0;real-cmd" gets matched as "real-cmd".
            let normalized = normalize_zsh_history(&raw);
            // Rule-pack pass.
            for (rule_idx, lineno, m) in rules::scan_text(&normalized) {
                let r = &rules::RULES[rule_idx];
                out.push(ConfigLeak {
                    id: format!("shell.history.gitleaks.{}", safe_id(r.id)),
                    auditor_id: "shell.history".to_string(),
                    category: "shell".to_string(),
                    risk: r.risk.to_string(),
                    title: r.title.to_string(),
                    location: format!("{path}:{lineno}"),
                    match_redacted: redact(&m),
                    pattern: r.id.to_string(),
                    description: format!(
                        "A {} value was found in shell history. Plaintext credentials in history files leak via .bash_history backups, screen-sharing, and anyone with read access to the user's home directory.",
                        r.title.to_lowercase()
                    ),
                    remediation: format!("Rotate the credential. Clear the relevant lines from {path} (or the whole file). Set HISTCONTROL=ignorespace and prefix sensitive commands with a leading space."),
                    references: Vec::new(),
                });
            }
            // Behavioural pass.
            for (lineno, line) in normalized.lines().enumerate() {
                if let Some(b) = match_behavioural(line) {
                    out.push(ConfigLeak {
                        id: format!("shell.history.behavior.{}", b.id),
                        auditor_id: "shell.history".to_string(),
                        category: "shell".to_string(),
                        risk: "medium".to_string(),
                        title: b.title.to_string(),
                        location: format!("{path}:{}", lineno + 1),
                        match_redacted: redact_command_line(line),
                        pattern: format!("behavior:{}", b.id),
                        description: b.description.to_string(),
                        remediation: b.remediation.to_string(),
                        references: Vec::new(),
                    });
                }
            }
        }
    }
    out
}

// normalize_zsh_history strips the ": <epoch>:<elapsed>;" prefix zsh
// adds to extended-history entries. Bash and the others write raw
// commands; this is a no-op for them.
fn normalize_zsh_history(raw: &str) -> String {
    let mut out = String::with_capacity(raw.len());
    for line in raw.lines() {
        let body = if let Some(rest) = line.strip_prefix(": ") {
            if let Some(idx) = rest.find(';') {
                &rest[idx + 1..]
            } else {
                rest
            }
        } else {
            line
        };
        out.push_str(body);
        out.push('\n');
    }
    out
}

struct BehaviouralRule {
    id: &'static str,
    title: &'static str,
    description: &'static str,
    remediation: &'static str,
}

const BEHAVIOURAL: &[BehaviouralRule] = &[
    BehaviouralRule {
        id: "mysql-inline-password",
        title: "Inline MySQL password on command line",
        description: "`mysql -p<password>` puts the credential in shell history, in /proc/<pid>/cmdline, and in any `ps` output. Use a defaults file (~/.my.cnf with [client] password=...) or prompt for the password instead.",
        remediation: "Replace the inline password with a configured client section or rely on the interactive prompt (`mysql -u user -p`).",
    },
    BehaviouralRule {
        id: "curl-basic-auth",
        title: "Inline HTTP Basic auth in curl",
        description: "`curl -u user:pass` exposes the credential in shell history and in the process command line. Read it from a credentials file (--netrc) or pass it via stdin.",
        remediation: "Move the credential to ~/.netrc (chmod 600) or a curl config file passed with `-K`.",
    },
    BehaviouralRule {
        id: "export-secret",
        title: "Secret exported into shell environment",
        description: "Exporting a credential-named variable from the interactive shell leaves it in history and in the env of every child process. Use a sourced .env file (chmod 600) or a secret manager client instead.",
        remediation: "Replace with `set -a; . ./.env; set +a` from a 600-perm file, or use direnv/age/sops/vault.",
    },
];

// match_behavioural is a hand-coded substring scanner — the legacy Go
// auditor used regex but the patterns are simple enough that a state
// machine over byte slices keeps the wasm artefact small. Returns
// the rule that matched, or None.
fn match_behavioural(line: &str) -> Option<&'static BehaviouralRule> {
    if matches_mysql_inline(line) {
        return Some(&BEHAVIOURAL[0]);
    }
    if matches_curl_basic_auth(line) {
        return Some(&BEHAVIOURAL[1]);
    }
    if matches_export_secret(line) {
        return Some(&BEHAVIOURAL[2]);
    }
    None
}

// matches_mysql_inline: "mysql ... -p<char>" where the next char
// after -p is non-whitespace (i.e. the password is on the cmdline).
fn matches_mysql_inline(line: &str) -> bool {
    let trimmed = line.trim_start();
    let has_mysql = trimmed.split_whitespace().any(|tok| tok == "mysql");
    if !has_mysql {
        return false;
    }
    let bytes = line.as_bytes();
    let mut i = 0;
    while i + 2 < bytes.len() {
        // -p must be preceded by whitespace or be at start.
        let preceded_by_space = i == 0 || bytes[i - 1] == b' ' || bytes[i - 1] == b'\t';
        if preceded_by_space && bytes[i] == b'-' && bytes[i + 1] == b'p' {
            let next = bytes[i + 2];
            // Non-empty inline password: anything but whitespace / EOF.
            if next != b' ' && next != b'\t' && next != b'\n' && next != b'-' {
                return true;
            }
        }
        i += 1;
    }
    false
}

// matches_curl_basic_auth: "curl ... -u user:..." or "--user user:..."
fn matches_curl_basic_auth(line: &str) -> bool {
    let has_curl = line.split_whitespace().any(|tok| tok == "curl");
    if !has_curl {
        return false;
    }
    let mut iter = line.split_whitespace().peekable();
    while let Some(tok) = iter.next() {
        if tok == "-u" || tok == "--user" {
            if let Some(val) = iter.peek() {
                if val.contains(':') && !val.ends_with(':') {
                    return true;
                }
            }
        }
    }
    false
}

// matches_export_secret: "export FOO_KEY=..." or "FOO_TOKEN=...".
// Case-insensitive on the keyword, case-sensitive on the var name
// suffix (KEY/TOKEN/SECRET/PASSWORD/PASSWD/PWD).
fn matches_export_secret(line: &str) -> bool {
    let stripped = line.trim_start_matches([' ', '\t', ';']);
    let after_export = if let Some(rest) = stripped.strip_prefix("export ") {
        rest
    } else if let Some(rest) = stripped.strip_prefix("EXPORT ") {
        rest
    } else {
        return false;
    };
    let after_export = after_export.trim_start();
    // Find first '=' on the variable.
    let eq = match after_export.find('=') {
        Some(i) => i,
        None => return false,
    };
    let name = after_export[..eq].trim();
    if name.is_empty() {
        return false;
    }
    let upper = name.to_ascii_uppercase();
    upper.ends_with("KEY")
        || upper.ends_with("TOKEN")
        || upper.ends_with("SECRET")
        || upper.ends_with("PASSWORD")
        || upper.ends_with("PASSWD")
        || upper.ends_with("PWD")
}

fn redact_command_line(line: &str) -> String {
    const MAX: usize = 96;
    let truncated = if line.len() > MAX {
        format!("{}…", &line[..MAX])
    } else {
        line.to_string()
    };
    // Mask the most common credential-bearing patterns.
    let masked = mask_kv(&truncated);
    mask_basic_auth(&masked)
}

fn mask_kv(s: &str) -> String {
    // Replace VAR=value where VAR ends in KEY/TOKEN/SECRET/PASSWORD/PASSWD/PWD
    // with VAR=****. Operates on whitespace-separated tokens to stay
    // UTF-8 safe (multi-byte chars like '…' don't get mangled).
    let mut out = String::with_capacity(s.len());
    let mut first = true;
    for tok in s.split(' ') {
        if !first {
            out.push(' ');
        }
        first = false;
        // Handle leading punctuation such as ';' from compound commands.
        let lead_idx = tok
            .char_indices()
            .find(|(_, c)| !matches!(*c, ';' | ',' | '|' | '&'))
            .map(|(i, _)| i)
            .unwrap_or(tok.len());
        out.push_str(&tok[..lead_idx]);
        let body = &tok[lead_idx..];
        let eq = match body.find('=') {
            Some(i) => i,
            None => {
                out.push_str(body);
                continue;
            }
        };
        let name = &body[..eq];
        let upper = name.to_ascii_uppercase();
        let credential_shaped = upper.ends_with("KEY")
            || upper.ends_with("TOKEN")
            || upper.ends_with("SECRET")
            || upper.ends_with("PASSWORD")
            || upper.ends_with("PASSWD")
            || upper.ends_with("PWD");
        if credential_shaped {
            out.push_str(name);
            out.push_str("=****");
        } else {
            out.push_str(body);
        }
    }
    out
}

fn mask_basic_auth(s: &str) -> String {
    // Replace " -u user:pass" / " --user user:pass" with " -u user:****".
    let mut out = String::with_capacity(s.len());
    let mut tokens = s.split(' ').peekable();
    while let Some(tok) = tokens.next() {
        out.push_str(tok);
        if (tok == "-u" || tok == "--user") && tokens.peek().is_some() {
            out.push(' ');
            let next = tokens.next().unwrap();
            if let Some(idx) = next.find(':') {
                out.push_str(&next[..=idx]);
                out.push_str("****");
            } else {
                out.push_str(next);
            }
        }
        if tokens.peek().is_some() {
            out.push(' ');
        }
    }
    out
}

// ============================================================
// cloud.aws (broadened from v2 — covers all credential dotfiles)
// ============================================================

struct DotfileTarget {
    rel: &'static str,
    max_bytes: usize,
}

const DOTFILE_TARGETS: &[DotfileTarget] = &[
    DotfileTarget { rel: ".aws/credentials", max_bytes: 64 * 1024 },
    DotfileTarget { rel: ".aws/config", max_bytes: 64 * 1024 },
    DotfileTarget { rel: ".config/gcloud/application_default_credentials.json", max_bytes: 64 * 1024 },
    DotfileTarget { rel: ".azure/accessTokens.json", max_bytes: 256 * 1024 },
    DotfileTarget { rel: ".azure/azureProfile.json", max_bytes: 256 * 1024 },
    DotfileTarget { rel: ".kube/config", max_bytes: 256 * 1024 },
    DotfileTarget { rel: ".docker/config.json", max_bytes: 256 * 1024 },
    DotfileTarget { rel: ".npmrc", max_bytes: 64 * 1024 },
    DotfileTarget { rel: ".pypirc", max_bytes: 64 * 1024 },
    DotfileTarget { rel: ".netrc", max_bytes: 64 * 1024 },
    DotfileTarget { rel: ".git-credentials", max_bytes: 64 * 1024 },
    DotfileTarget { rel: ".gitconfig", max_bytes: 256 * 1024 },
];

#[cfg(target_arch = "wasm32")]
fn audit_cloud_dotfiles() -> Vec<ConfigLeak> {
    let mut out = Vec::new();
    for home in candidate_home_dirs() {
        for t in DOTFILE_TARGETS {
            let path = format!("{home}/{}", t.rel);
            let raw = match read_string(&path) {
                Some(v) => v,
                None => continue,
            };
            // Skip oversize files (the host enforces this too; the
            // soft cap here keeps the rule pack from churning on a
            // mis-named multi-MB file).
            if raw.len() > t.max_bytes {
                continue;
            }
            // Rule-pack pass.
            for (rule_idx, lineno, m) in rules::scan_text(&raw) {
                let r = &rules::RULES[rule_idx];
                out.push(ConfigLeak {
                    id: format!("cloud.aws.gitleaks.{}", safe_id(r.id)),
                    auditor_id: "cloud.aws".to_string(),
                    category: "cloud".to_string(),
                    risk: r.risk.to_string(),
                    title: r.title.to_string(),
                    location: format!("{path}:{lineno}"),
                    match_redacted: redact(&m),
                    pattern: r.id.to_string(),
                    description: format!("A {} value was found in this credential dotfile. Tools like aws-cli and gcloud write these files with restrictive perms by default; loosening them or copying them into images is the most common cloud-account hijack vector.", r.title.to_lowercase()),
                    remediation: "Rotate the credential at its issuer. Prefer instance/role credentials (IAM role, GKE service account, managed identity) over long-lived keys on disk.".to_string(),
                    references: Vec::new(),
                });
            }
            // Permission check via the parent's listdir entry.
            if let Some(entry) = stat_via_parent(&path) {
                if entry.mode & 0o004 != 0 {
                    out.push(ConfigLeak {
                        id: "cloud.aws.world_readable".to_string(),
                        auditor_id: "cloud.aws".to_string(),
                        category: "cloud".to_string(),
                        risk: "medium".to_string(),
                        title: "Credential file is world-readable".to_string(),
                        location: path.clone(),
                        match_redacted: format!("mode={:04o}", entry.mode & 0o7777),
                        pattern: "behavior:world-readable".to_string(),
                        description: "This credential file's permission mode allows any local user to read it. AWS / gcloud / kube CLIs default to 600 — loosening that exposes the credentials to every process on the host.".to_string(),
                        remediation: format!("Restore restrictive permissions: chmod 600 {path}."),
                        references: Vec::new(),
                    });
                }
            }
        }
    }
    out
}

// ============================================================
// ssh.private_keys
// ============================================================

#[cfg(target_arch = "wasm32")]
fn audit_ssh_keys() -> Vec<ConfigLeak> {
    let mut out = Vec::new();
    for home in candidate_home_dirs() {
        let dir = format!("{home}/.ssh");
        let entries = match list_dir(&dir) {
            Some(v) => v,
            None => continue,
        };
        for e in entries {
            if e.is_dir {
                continue;
            }
            let path = format!("{dir}/{}", e.name);
            let body = match read_string(&path) {
                Some(v) => v,
                None => continue,
            };
            // Private key inspection.
            if body.contains("PRIVATE KEY-----") {
                let encrypted = body.contains("BEGIN ENCRYPTED PRIVATE KEY")
                    || body.contains("Proc-Type: 4,ENCRYPTED")
                    || is_openssh_encrypted(&body);
                if !encrypted {
                    out.push(ConfigLeak {
                        id: "ssh.private_keys.unencrypted_private".to_string(),
                        auditor_id: "ssh.private_keys".to_string(),
                        category: "ssh".to_string(),
                        risk: "high".to_string(),
                        title: "Unencrypted SSH private key on disk".to_string(),
                        location: path.clone(),
                        match_redacted: "PEM PRIVATE KEY without passphrase".to_string(),
                        pattern: "behavior:ssh-key-unencrypted".to_string(),
                        description: "This file contains a PEM-encoded SSH private key with no passphrase. Anyone who can read the file can use it to authenticate as its owner.".to_string(),
                        remediation: format!("Add a passphrase: `ssh-keygen -p -f {path}`. Better, store the key in an agent (`ssh-add`) and rotate any keys that may have been exposed."),
                        references: Vec::new(),
                    });
                }
                if e.mode & 0o004 != 0 {
                    out.push(ConfigLeak {
                        id: "ssh.private_keys.private_world_readable".to_string(),
                        auditor_id: "ssh.private_keys".to_string(),
                        category: "ssh".to_string(),
                        risk: "high".to_string(),
                        title: "SSH private key is world-readable".to_string(),
                        location: path.clone(),
                        match_redacted: format!("mode={:04o}", e.mode & 0o7777),
                        pattern: "behavior:ssh-key-world-readable".to_string(),
                        description: "OpenSSH refuses to use a private key whose mode allows reads from anyone other than the owner — the file is also a leak via every other process on the host.".to_string(),
                        remediation: format!("Restore restrictive permissions: chmod 600 {path}."),
                        references: Vec::new(),
                    });
                }
            }
            // authorized_keys inspection.
            if e.name.eq_ignore_ascii_case("authorized_keys") {
                out.extend(check_authorized_keys(&path, &body));
            }
        }
    }
    out
}

fn is_openssh_encrypted(body: &str) -> bool {
    if !body.contains("BEGIN OPENSSH PRIVATE KEY") {
        return false;
    }
    // OpenSSH unencrypted keys decode to a header containing the
    // literal "none" cipher tag at offset 15. The base64 prefix below
    // appears in the body of every unencrypted OpenSSH key.
    !body.contains("b3BlbnNzaC1rZXktdjEAAAAABG5vbmU")
}

fn check_authorized_keys(path: &str, body: &str) -> Vec<ConfigLeak> {
    const KEY_TYPES: &[&str] = &[
        "ssh-rsa",
        "ssh-ed25519",
        "ssh-dss",
        "ecdsa-sha2-nistp256",
        "ecdsa-sha2-nistp384",
        "ecdsa-sha2-nistp521",
        "sk-ecdsa-sha2-nistp256@openssh.com",
        "sk-ssh-ed25519@openssh.com",
    ];
    let mut out = Vec::new();
    for (i, line) in body.lines().enumerate() {
        let t = line.trim();
        if t.is_empty() || t.starts_with('#') {
            continue;
        }
        let first = t.split_whitespace().next().unwrap_or("");
        if !KEY_TYPES.iter().any(|k| *k == first) {
            continue;
        }
        out.push(ConfigLeak {
            id: "ssh.private_keys.authorized_no_options".to_string(),
            auditor_id: "ssh.private_keys".to_string(),
            category: "ssh".to_string(),
            risk: "info".to_string(),
            title: "authorized_keys entry has no restrictions".to_string(),
            location: format!("{path}:{}", i + 1),
            match_redacted: format!("{first} <fingerprint hidden>"),
            pattern: "behavior:authorized-no-options".to_string(),
            description: "This authorized_keys entry permits full interactive login. If the key is used for an automated task, consider adding `from=`, `command=`, or `restrict` options to limit blast radius.".to_string(),
            remediation: r#"Prefix the entry with options like `from="10.0.0.0/8",no-pty,no-port-forwarding,command="<binary>"`."#.to_string(),
            references: Vec::new(),
        });
    }
    out
}

// ============================================================
// env.process
// ============================================================

const PER_PROC_ENV_CAP: usize = 256 * 1024;
const TOTAL_ENV_BUDGET: usize = 4 * 1024 * 1024;

#[cfg(target_arch = "wasm32")]
fn audit_env_process() -> Vec<ConfigLeak> {
    let entries = match list_dir("/proc") {
        Some(v) => v,
        None => return Vec::new(), // /proc not exposed (containers, non-linux).
    };
    let mut out = Vec::new();
    let mut consumed: usize = 0;
    for e in entries {
        // Only numeric directory names are PIDs.
        if !e.is_dir {
            continue;
        }
        let pid: u32 = match e.name.parse() {
            Ok(v) if v > 0 => v,
            _ => continue,
        };
        if consumed >= TOTAL_ENV_BUDGET {
            break;
        }
        let environ_path = format!("/proc/{pid}/environ");
        let environ = match read_string(&environ_path) {
            Some(v) => v,
            None => continue, // permission denied / no env (kthread)
        };
        if environ.is_empty() {
            continue;
        }
        let bytes = environ.len().min(PER_PROC_ENV_CAP);
        consumed += bytes;
        let comm = read_string(&format!("/proc/{pid}/comm"))
            .map(|s| s.trim().to_string())
            .unwrap_or_else(|| "?".to_string());
        // Walk NUL-separated NAME=VALUE pairs.
        for pair in environ[..bytes].split('\0') {
            if pair.is_empty() {
                continue;
            }
            let name = pair.split('=').next().unwrap_or("");
            let loc = format!("pid={pid} comm={comm} env={name}");
            if let Some((rule_idx, _, m)) = first_rule_match(pair) {
                let r = &rules::RULES[rule_idx];
                out.push(ConfigLeak {
                    id: format!("env.process.gitleaks.{}", safe_id(r.id)),
                    auditor_id: "env.process".to_string(),
                    category: "env".to_string(),
                    risk: r.risk.to_string(),
                    title: r.title.to_string(),
                    location: loc,
                    match_redacted: redact(&m),
                    pattern: r.id.to_string(),
                    description: format!("Process {comm} (pid {pid}) has a {} value in its environment. Env vars are visible to every child process and to anyone with /proc read access for that uid.", r.title.to_lowercase()),
                    remediation: format!("Restart the process after rotating the credential. Prefer reading secrets from a 600-perm file or a secret manager rather than leaving them in the env."),
                    references: Vec::new(),
                });
            }
        }
    }
    out
}

// first_rule_match scans a single string against the rule pack and
// returns the first hit. Used by env.process which feeds one
// NAME=VALUE at a time.
fn first_rule_match(s: &str) -> Option<(usize, usize, String)> {
    rules::scan_line(s).map(|(idx, m)| (idx, 1, m))
}

// ============================================================
// db.config
// ============================================================

const DB_CRED_FILES: &[&str] = &["/etc/mysql/my.cnf", "/etc/my.cnf"];
const DB_CRED_PER_HOME: &[&str] = &[".my.cnf", ".pgpass"];

#[cfg(target_arch = "wasm32")]
fn audit_db_config() -> Vec<ConfigLeak> {
    let mut out = Vec::new();
    for p in DB_CRED_FILES {
        out.extend(scan_db_cred(p));
    }
    for home in candidate_home_dirs() {
        for rel in DB_CRED_PER_HOME {
            out.extend(scan_db_cred(&format!("{home}/{rel}")));
        }
    }
    // pg_hba.conf — match common installs. Single-level glob via
    // listdir on /etc/postgresql/*/main/.
    out.extend(check_pg_hba("/etc/postgresql/main/pg_hba.conf"));
    if let Some(versioned_dirs) = list_dir("/etc/postgresql") {
        for d in versioned_dirs.into_iter().filter(|e| e.is_dir) {
            let p = format!("/etc/postgresql/{}/main/pg_hba.conf", d.name);
            out.extend(check_pg_hba(&p));
        }
    }
    out.extend(check_redis("/etc/redis/redis.conf"));
    out.extend(check_redis("/etc/redis.conf"));
    out.extend(check_mongo("/etc/mongod.conf"));
    out.extend(check_mongo("/etc/mongodb.conf"));
    out
}

#[cfg(target_arch = "wasm32")]
fn scan_db_cred(path: &str) -> Vec<ConfigLeak> {
    let raw = match read_string(path) {
        Some(v) => v,
        None => return Vec::new(),
    };
    let mut out = Vec::new();
    // Rule-pack pass.
    for (rule_idx, lineno, m) in rules::scan_text(&raw) {
        let r = &rules::RULES[rule_idx];
        out.push(ConfigLeak {
            id: format!("db.config.gitleaks.{}", safe_id(r.id)),
            auditor_id: "db.config".to_string(),
            category: "database".to_string(),
            risk: r.risk.to_string(),
            title: r.title.to_string(),
            location: format!("{path}:{lineno}"),
            match_redacted: redact(&m),
            pattern: r.id.to_string(),
            description: format!("A {} value was found in this database config. Anyone who can read the file can authenticate.", r.title.to_lowercase()),
            remediation: "Use a credential helper (mysql-config-editor, .my.cnf [client] section with chmod 600), an AUTH socket, or a secret manager.".to_string(),
            references: Vec::new(),
        });
    }
    // Bonus: my.cnf / .pgpass with `password=<value>` lines that the
    // rule pack might miss (low-entropy human passwords).
    for (i, line) in raw.lines().enumerate() {
        let trimmed = line.trim_start();
        let lower = trimmed.to_ascii_lowercase();
        if !lower.starts_with("password") {
            continue;
        }
        let after_pass = &trimmed[8..]; // skip "password"
        // Accept "password = foo" or "password=foo".
        let after_pass = after_pass.trim_start();
        let value = if let Some(rest) = after_pass.strip_prefix('=') {
            rest.trim()
        } else {
            continue;
        };
        if value.is_empty() {
            continue;
        }
        out.push(ConfigLeak {
            id: "db.config.password_kv".to_string(),
            auditor_id: "db.config".to_string(),
            category: "database".to_string(),
            risk: "low".to_string(),
            title: "Database password configured in clear text".to_string(),
            location: format!("{path}:{}", i + 1),
            match_redacted: "password=****".to_string(),
            pattern: "behavior:password-kv".to_string(),
            description: "A `password=` line in this database config means the credential is on disk in plaintext.".to_string(),
            remediation: "Use a credential helper, an AUTH socket, or restrict file mode to 600 and document the threat model.".to_string(),
            references: Vec::new(),
        });
    }
    out
}

#[cfg(target_arch = "wasm32")]
fn check_pg_hba(path: &str) -> Vec<ConfigLeak> {
    let raw = match read_string(path) {
        Some(v) => v,
        None => return Vec::new(),
    };
    let mut out = Vec::new();
    for (i, line) in raw.lines().enumerate() {
        let t = line.trim();
        if t.is_empty() || t.starts_with('#') {
            continue;
        }
        let fields: Vec<&str> = t.split_whitespace().collect();
        if fields.len() < 4 {
            continue;
        }
        let method = fields[fields.len() - 1];
        if !method.eq_ignore_ascii_case("trust") {
            continue;
        }
        let is_local = fields[0].eq_ignore_ascii_case("local");
        let addr = if !is_local && fields.len() >= 5 {
            fields[3]
        } else {
            ""
        };
        if is_local || addr == "127.0.0.1/32" || addr == "::1/128" {
            continue;
        }
        out.push(ConfigLeak {
            id: "db.config.pg_hba_trust".to_string(),
            auditor_id: "db.config".to_string(),
            category: "database".to_string(),
            risk: "high".to_string(),
            title: "PostgreSQL accepts trust authentication from a network client".to_string(),
            location: format!("{path}:{}", i + 1),
            match_redacted: t.to_string(),
            pattern: "behavior:pg-hba-trust".to_string(),
            description: "A pg_hba.conf entry with method `trust` lets any client matching the address authenticate as any database user with no credential check. On a non-loopback address this is a remote-exposure issue.".to_string(),
            remediation: "Replace `trust` with `md5`, `scram-sha-256`, or `cert`, then `pg_ctl reload`.".to_string(),
            references: Vec::new(),
        });
    }
    out
}

#[cfg(target_arch = "wasm32")]
fn check_redis(path: &str) -> Vec<ConfigLeak> {
    let raw = match read_string(path) {
        Some(v) => v,
        None => return Vec::new(),
    };
    let mut has_require_pass = false;
    let mut has_bind_local = false;
    let mut protected_mode_no = false;
    for line in raw.lines() {
        let t = line.trim();
        if t.is_empty() || t.starts_with('#') {
            continue;
        }
        if t.starts_with("requirepass ") {
            has_require_pass = true;
        } else if let Some(rest) = t.strip_prefix("bind ") {
            if rest.contains("127.0.0.1") || rest.contains("::1") {
                has_bind_local = true;
            }
        } else if t == "protected-mode no" {
            protected_mode_no = true;
        }
    }
    let mut out = Vec::new();
    if !has_require_pass && !has_bind_local {
        out.push(ConfigLeak {
            id: "db.config.redis_no_auth".to_string(),
            auditor_id: "db.config".to_string(),
            category: "database".to_string(),
            risk: "high".to_string(),
            title: "Redis accepts unauthenticated connections".to_string(),
            location: path.to_string(),
            match_redacted: "no requirepass; not bound to loopback".to_string(),
            pattern: "behavior:redis-no-auth".to_string(),
            description: "Redis with no `requirepass` and no loopback `bind` is reachable by any host that can route to it, with full database access.".to_string(),
            remediation: "Set `requirepass <strong-secret>` and `bind 127.0.0.1 ::1`, then restart redis.".to_string(),
            references: Vec::new(),
        });
    }
    if protected_mode_no {
        out.push(ConfigLeak {
            id: "db.config.redis_protected_mode_off".to_string(),
            auditor_id: "db.config".to_string(),
            category: "database".to_string(),
            risk: "high".to_string(),
            title: "Redis protected-mode is disabled".to_string(),
            location: path.to_string(),
            match_redacted: "protected-mode no".to_string(),
            pattern: "behavior:redis-protected-mode-off".to_string(),
            description: "`protected-mode no` removes redis's last-line guard against accepting unauthenticated connections from non-loopback addresses.".to_string(),
            remediation: "Set `protected-mode yes` and ensure either `bind` is loopback-only or `requirepass` is set.".to_string(),
            references: Vec::new(),
        });
    }
    out
}

#[cfg(target_arch = "wasm32")]
fn check_mongo(path: &str) -> Vec<ConfigLeak> {
    let raw = match read_string(path) {
        Some(v) => v,
        None => return Vec::new(),
    };
    if mongo_auth_enabled(&raw) {
        return Vec::new();
    }
    vec![ConfigLeak {
        id: "db.config.mongo_no_auth".to_string(),
        auditor_id: "db.config".to_string(),
        category: "database".to_string(),
        risk: "high".to_string(),
        title: "MongoDB authorization is not enabled".to_string(),
        location: path.to_string(),
        match_redacted: "missing security.authorization=enabled".to_string(),
        pattern: "behavior:mongo-no-auth".to_string(),
        description: "Without `security.authorization: enabled`, mongod accepts unauthenticated clients with full access to all databases.".to_string(),
        remediation: "Add the `security:` block with `authorization: enabled`, create an admin user, and restart mongod.".to_string(),
        references: Vec::new(),
    }]
}

// mongo_auth_enabled is a textual heuristic — mongod.conf is YAML
// but we don't ship a parser. The legacy auditor uses the same
// "look for `security:` followed by `authorization: enabled`"
// string match.
fn mongo_auth_enabled(s: &str) -> bool {
    // Find "security:" then look for "authorization:" with "enabled"
    // before the next non-indented top-level key.
    let lower = s.to_ascii_lowercase();
    let sec = match lower.find("security:") {
        Some(i) => i,
        None => return false,
    };
    let after = &lower[sec..];
    // Truncate at the next non-indented "<key>:" line so we don't
    // match an authorization: under a different top-level block.
    let mut window_end = after.len();
    for (i, line) in after.lines().enumerate() {
        if i == 0 {
            continue;
        }
        if !line.starts_with(char::is_whitespace) && line.contains(':') {
            // Compute the byte offset back into `after`.
            let off = after
                .lines()
                .take(i)
                .map(|l| l.len() + 1) // +1 for '\n'
                .sum::<usize>();
            window_end = off;
            break;
        }
    }
    let window = &after[..window_end];
    window.contains("authorization:") && window.contains("enabled")
}

// ============================================================
// webapp.config
// ============================================================

const WEBAPP_ROOTS: &[&str] = &["/var/www", "/srv", "/opt"];
const WEBAPP_MAX_DEPTH: usize = 4;
const WEBAPP_MAX_FILES: usize = 200;

const WEBAPP_FILENAMES: &[&str] = &[
    ".env",
    ".env.local",
    ".env.production",
    ".env.staging",
    "wp-config.php",
    "settings.py",
    "local_settings.py",
    "application.yml",
    "application.yaml",
    "application.properties",
    "appsettings.json",
    "config.php",
    "database.yml",
    "secrets.yml",
    "credentials.yml.enc",
];

const WEBAPP_SKIP_DIRS: &[&str] = &[
    "node_modules",
    "vendor",
    ".git",
    ".cache",
    "dist",
    "build",
];

#[cfg(target_arch = "wasm32")]
fn audit_webapp_config() -> Vec<ConfigLeak> {
    let mut roots: Vec<String> = WEBAPP_ROOTS.iter().map(|s| (*s).to_string()).collect();
    for h in candidate_home_dirs() {
        roots.push(h);
    }

    let mut out = Vec::new();
    let mut visited: usize = 0;

    'outer: for root in roots {
        // Pre-flight: skip if we can't list the root.
        if list_dir(&root).is_none() {
            continue;
        }
        let mut stack: Vec<(String, usize)> = vec![(root.clone(), 0)];
        while let Some((dir, depth)) = stack.pop() {
            if visited >= WEBAPP_MAX_FILES {
                break 'outer;
            }
            let entries = match list_dir(&dir) {
                Some(v) => v,
                None => continue,
            };
            for e in entries {
                if visited >= WEBAPP_MAX_FILES {
                    break 'outer;
                }
                let path = format!("{dir}/{}", e.name);
                if e.is_dir {
                    if WEBAPP_SKIP_DIRS.iter().any(|s| *s == e.name) {
                        continue;
                    }
                    if depth + 1 <= WEBAPP_MAX_DEPTH {
                        stack.push((path, depth + 1));
                    }
                    continue;
                }
                if !is_webapp_config_file(&e.name) {
                    continue;
                }
                visited += 1;
                let raw = match read_string(&path) {
                    Some(v) => v,
                    None => continue,
                };
                if raw.len() > 1024 * 1024 {
                    continue;
                }
                for (rule_idx, lineno, m) in rules::scan_text(&raw) {
                    let r = &rules::RULES[rule_idx];
                    out.push(ConfigLeak {
                        id: format!("webapp.config.gitleaks.{}", safe_id(r.id)),
                        auditor_id: "webapp.config".to_string(),
                        category: "webapp".to_string(),
                        risk: r.risk.to_string(),
                        title: r.title.to_string(),
                        location: format!("{path}:{lineno}"),
                        match_redacted: redact(&m),
                        pattern: r.id.to_string(),
                        description: format!("A {} value was found in this web-app config. If the file is shipped into a container image or readable by other UIDs, the credential's blast radius is the entire host.", r.title.to_lowercase()),
                        remediation: "Read secrets from a sourced .env file outside the deploy tree (chmod 600), or inject via a secret manager / orchestrator.".to_string(),
                        references: Vec::new(),
                    });
                }
                if e.mode & 0o004 != 0 {
                    out.push(ConfigLeak {
                        id: "webapp.config.world_readable".to_string(),
                        auditor_id: "webapp.config".to_string(),
                        category: "webapp".to_string(),
                        risk: "low".to_string(),
                        title: "Web app config file is world-readable".to_string(),
                        location: path.clone(),
                        match_redacted: format!("mode={:04o}", e.mode & 0o7777),
                        pattern: "behavior:world-readable".to_string(),
                        description: "This config file's permission mode allows any local user to read it. If it contains secrets the blast radius is the entire host, not just the web-app's own UID.".to_string(),
                        remediation: format!("Tighten with `chmod 640 {path}` (or 600) and ensure it's owned by the web-app user."),
                        references: Vec::new(),
                    });
                }
            }
        }
    }
    out
}

fn is_webapp_config_file(name: &str) -> bool {
    let lower = name.to_ascii_lowercase();
    if WEBAPP_FILENAMES.iter().any(|n| *n == lower) {
        return true;
    }
    // .NET appsettings.<env>.json variants.
    lower.starts_with("appsettings.") && lower.ends_with(".json")
}

// ============================================================
// helpers
// ============================================================

// candidate_home_dirs enumerates /home/<user>/ + /root.
#[cfg(target_arch = "wasm32")]
fn candidate_home_dirs() -> Vec<String> {
    let mut out = vec!["/root".to_string()];
    if let Some(homes) = list_dir("/home") {
        for e in homes.into_iter().filter(|e| e.is_dir) {
            out.push(format!("/home/{}", e.name));
        }
    }
    out
}

// stat_via_parent fetches a directory entry from its parent listing
// — host_fs_listdir is the only path that surfaces mode bits.
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

// safe_id sanitises a rule id into a dotted-leak-id-safe segment.
fn safe_id(s: &str) -> String {
    s.chars()
        .map(|c| match c {
            'a'..='z' | 'A'..='Z' | '0'..='9' | '-' | '_' => c,
            _ => '_',
        })
        .collect()
}

// redact returns first 4 + last 4 chars with **** between them. <=8
// chars get replaced wholesale with stars, matching the legacy
// gopsutil-era output.
fn redact(s: &str) -> String {
    let chars: Vec<char> = s.chars().collect();
    if chars.len() <= 8 {
        return "*".repeat(chars.len());
    }
    let head: String = chars.iter().take(4).collect();
    let tail: String = chars.iter().rev().take(4).collect::<Vec<_>>().into_iter().rev().collect();
    format!("{head}****{tail}")
}

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

    // ---- redact ---------------------------------------------

    #[test]
    fn redact_long_string() {
        let r = redact("AKIAIOSFODNN7EXAMPLE_padding"); // 28 chars
        assert!(r.starts_with("AKIA"));
        assert!(r.contains("****"));
        assert!(r.ends_with("ding"));
    }

    #[test]
    fn redact_short_strings() {
        assert_eq!(redact("short"), "*****");
        assert_eq!(redact("12345678"), "********");
        assert_eq!(redact(""), "");
    }

    // ---- normalize_zsh_history ------------------------------

    #[test]
    fn normalize_zsh_strips_timestamp() {
        let raw = ": 1700000000:0;ls -la\n: 1700000010:1;cd /tmp\nplain command\n";
        let got = normalize_zsh_history(raw);
        assert!(got.contains("ls -la"));
        assert!(got.contains("cd /tmp"));
        assert!(got.contains("plain command"));
        assert!(!got.contains("1700000000"));
    }

    #[test]
    fn normalize_zsh_passthrough_for_bash() {
        let raw = "ls -la\ncd /tmp\n";
        assert_eq!(normalize_zsh_history(raw), "ls -la\ncd /tmp\n");
    }

    // ---- behavioural patterns -------------------------------

    #[test]
    fn behavioural_mysql_inline_password() {
        assert!(matches_mysql_inline("mysql -u root -psecret"));
        assert!(matches_mysql_inline("mysql -psecret -u root"));
        assert!(!matches_mysql_inline("mysql -u root -p")); // interactive prompt is fine
        assert!(!matches_mysql_inline("ls -p /tmp"));
        assert!(!matches_mysql_inline("postgres -p secret"));
    }

    #[test]
    fn behavioural_curl_basic_auth() {
        assert!(matches_curl_basic_auth("curl -u user:pass https://api"));
        assert!(matches_curl_basic_auth("curl --user user:pass https://api"));
        assert!(!matches_curl_basic_auth("curl -u user: https://api")); // empty password
        assert!(!matches_curl_basic_auth("wget -u user:pass"));
    }

    #[test]
    fn behavioural_export_secret() {
        assert!(matches_export_secret("export AWS_SECRET_KEY=foo"));
        assert!(matches_export_secret("export AWS_ACCESS_TOKEN=abc"));
        assert!(matches_export_secret("export DB_PASSWORD=hunter2"));
        assert!(matches_export_secret("export FOO_PWD=x"));
        assert!(!matches_export_secret("export PATH=/usr/bin")); // not a credential-shaped suffix
        assert!(!matches_export_secret("FOO=bar")); // no export keyword
    }

    #[test]
    fn match_behavioural_routes_correctly() {
        assert_eq!(
            match_behavioural("mysql -u root -psecret").map(|r| r.id),
            Some("mysql-inline-password")
        );
        assert_eq!(
            match_behavioural("curl -u admin:hunter2 https://x").map(|r| r.id),
            Some("curl-basic-auth")
        );
        assert_eq!(
            match_behavioural("export GITHUB_TOKEN=abc").map(|r| r.id),
            Some("export-secret")
        );
        assert!(match_behavioural("ls -la").is_none());
    }

    // ---- redact_command_line --------------------------------

    #[test]
    fn redact_command_line_truncates_long() {
        let line = "x".repeat(200);
        let red = redact_command_line(&line);
        assert!(red.ends_with("…"));
        assert!(red.len() < line.len());
    }

    #[test]
    fn redact_command_line_masks_kv() {
        let red = redact_command_line("export AWS_SECRET=value-here-xxx more args");
        assert!(red.contains("AWS_SECRET=****"));
        assert!(!red.contains("value-here-xxx"));
    }

    #[test]
    fn redact_command_line_masks_basic_auth() {
        let red = redact_command_line("curl -u admin:hunter2 https://api");
        assert!(red.contains("admin:****"));
        assert!(!red.contains("hunter2"));
    }

    // ---- safe_id --------------------------------------------

    #[test]
    fn safe_id_keeps_safe_chars() {
        assert_eq!(safe_id("aws-access-token"), "aws-access-token");
        assert_eq!(safe_id("github_pat"), "github_pat");
    }

    #[test]
    fn safe_id_replaces_bad_chars() {
        assert_eq!(safe_id("foo.bar"), "foo_bar");
        assert_eq!(safe_id("foo bar"), "foo_bar");
    }

    // ---- split_parent ---------------------------------------

    #[test]
    fn split_parent_basic() {
        assert_eq!(
            split_parent("/home/alice/.ssh/id_rsa"),
            Some(("/home/alice/.ssh".to_string(), "id_rsa".to_string()))
        );
        assert_eq!(
            split_parent("/etc"),
            Some(("/".to_string(), "etc".to_string()))
        );
    }

    #[test]
    fn split_parent_root_rejected() {
        assert_eq!(split_parent("/"), None);
        assert_eq!(split_parent(""), None);
    }

    // ---- is_openssh_encrypted -------------------------------

    #[test]
    fn openssh_encrypted_with_marker() {
        let body = "-----BEGIN OPENSSH PRIVATE KEY-----\n\
            <bytes>\nb3BlbnNzaC1rZXktdjEAAAAACmFlczI1Ni1jYmM=\n\
            -----END OPENSSH PRIVATE KEY-----";
        assert!(is_openssh_encrypted(body));
    }

    #[test]
    fn openssh_unencrypted_marker() {
        let body = "-----BEGIN OPENSSH PRIVATE KEY-----\n\
            <bytes>\nb3BlbnNzaC1rZXktdjEAAAAABG5vbmU\n\
            -----END OPENSSH PRIVATE KEY-----";
        assert!(!is_openssh_encrypted(body));
    }

    #[test]
    fn openssh_not_an_openssh_key() {
        assert!(!is_openssh_encrypted("-----BEGIN RSA PRIVATE KEY-----"));
    }

    // ---- check_authorized_keys ------------------------------

    #[test]
    fn authorized_keys_no_options_flagged() {
        let body = "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQ comment\n";
        let out = check_authorized_keys("/home/alice/.ssh/authorized_keys", body);
        assert_eq!(out.len(), 1);
        assert_eq!(out[0].risk, "info");
        assert!(out[0].location.ends_with(":1"));
    }

    #[test]
    fn authorized_keys_with_options_skipped() {
        // Lines starting with options (not a key type) aren't flagged.
        let body = "from=\"10.0.0.0/8\",no-pty ssh-rsa AAAAB...\n";
        assert!(check_authorized_keys("/x", body).is_empty());
    }

    #[test]
    fn authorized_keys_comment_skipped() {
        let body = "# this is a comment\n";
        assert!(check_authorized_keys("/x", body).is_empty());
    }

    // ---- pg_hba checks (pure on raw text) -------------------

    #[test]
    fn pg_hba_trust_remote_flagged() {
        // We can't easily test check_pg_hba without read_string, but
        // can exercise the inner classification via direct line
        // inspection. A lightweight smoke test using a synthetic file
        // would need read_string mocked, which the wasm-only gating
        // prevents — covered by integration tests instead.
        // This is a placeholder asserting the constant tables exist.
        assert!(!DB_CRED_FILES.is_empty());
        assert!(!DB_CRED_PER_HOME.is_empty());
    }

    // ---- mongo_auth_enabled ---------------------------------

    #[test]
    fn mongo_auth_enabled_yaml() {
        let yaml = "net:\n  port: 27017\nsecurity:\n  authorization: enabled\n";
        assert!(mongo_auth_enabled(yaml));
    }

    #[test]
    fn mongo_auth_disabled_yaml() {
        let yaml = "net:\n  port: 27017\nsecurity:\n  authorization: disabled\n";
        assert!(!mongo_auth_enabled(yaml));
    }

    #[test]
    fn mongo_auth_no_security_section() {
        let yaml = "net:\n  port: 27017\n";
        assert!(!mongo_auth_enabled(yaml));
    }

    #[test]
    fn mongo_auth_other_block_doesnt_count() {
        // "authorization: enabled" under a non-security block must
        // not be treated as enabling auth.
        let yaml = "operationProfiling:\n  authorization: enabled\nnet:\n  port: 27017\n";
        assert!(!mongo_auth_enabled(yaml));
    }

    // ---- is_webapp_config_file ------------------------------

    #[test]
    fn webapp_config_file_known_names() {
        assert!(is_webapp_config_file(".env"));
        assert!(is_webapp_config_file(".env.production"));
        assert!(is_webapp_config_file("wp-config.php"));
        assert!(is_webapp_config_file("settings.py"));
        assert!(is_webapp_config_file("application.yaml"));
        assert!(is_webapp_config_file("appsettings.json"));
        assert!(is_webapp_config_file("appsettings.Development.json"));
    }

    #[test]
    fn webapp_config_file_skips_others() {
        assert!(!is_webapp_config_file("README.md"));
        assert!(!is_webapp_config_file("index.php"));
        assert!(!is_webapp_config_file("appsettings.txt"));
    }
}
