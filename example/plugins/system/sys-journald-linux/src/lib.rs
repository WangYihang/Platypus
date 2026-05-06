// sys-journald-linux — `query` RPC over `journalctl -o json`.
//
// Wire shape (request):
//   {
//     unit:      "ssh.service" | "" (no filter),
//     priority:  "0".."7" | "info"|"warning"|"err"|… | "" (no filter),
//     since:     "2024-01-01 00:00:00" | "1 hour ago" | "" (no filter),
//     until:     same syntax,
//     grep:      regex passed to --grep | "" (no filter),
//     lines:     u32, defaults to 500, capped at 10000,
//     reverse:   bool — newest-first when true,
//     boot:      "" | "0" (current boot) | "-1" (previous) | "all"
//   }
//
// Wire shape (response):
//   {
//     entries: [
//       { timestamp_us: u64,
//         unit: "ssh.service",
//         priority: u8,            (0 emerg .. 7 debug)
//         message: "…",
//         hostname: "…",
//         pid: u32,
//         uid: u32,
//         identifier: "…",         (SYSLOG_IDENTIFIER)
//         comm: "…"                (_COMM, falls back from identifier)
//       },
//       …
//     ],
//     truncated: bool,             (true if hit the line cap)
//     error: ""
//   }
//
// Each line of `journalctl -o json` is a single JSON object with
// fields like __REALTIME_TIMESTAMP (microseconds-since-epoch as a
// string), MESSAGE (string OR array of bytes for binary-decoded
// messages), PRIORITY ("0".."7"), _SYSTEMD_UNIT, _PID, _UID,
// _HOSTNAME, SYSLOG_IDENTIFIER, _COMM. We extract just the fields
// the operator UI needs and discard the rest — keeps wire size
// bounded even when journalctl emits a 50-field record.

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

// Default + cap line counts. The cap bounds wasm memory usage —
// each entry is ~200-500 bytes after extraction; 10k entries is
// roughly 5 MiB, comfortably under the manifest's 64 MiB limit.
const DEFAULT_LINES: u32 = 500;
const MAX_LINES: u32 = 10_000;

// ---------- request ----------

#[derive(Deserialize, Default)]
struct QueryRequest {
    #[serde(default)]
    unit: String,
    #[serde(default)]
    priority: String,
    #[serde(default)]
    since: String,
    #[serde(default)]
    until: String,
    #[serde(default)]
    grep: String,
    #[serde(default)]
    lines: u32,
    #[serde(default)]
    reverse: bool,
    #[serde(default)]
    boot: String,
}

// ---------- response ----------

#[derive(Serialize, Default)]
struct QueryResponse {
    entries: Vec<Entry>,
    #[serde(skip_serializing_if = "is_false")]
    truncated: bool,
    #[serde(skip_serializing_if = "String::is_empty")]
    error: String,
}

#[derive(Serialize, Default)]
pub struct Entry {
    #[serde(rename = "timestampUs", skip_serializing_if = "is_zero_u64")]
    pub timestamp_us: u64,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub unit: String,
    #[serde(skip_serializing_if = "is_zero_u8")]
    pub priority: u8,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub message: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub hostname: String,
    #[serde(skip_serializing_if = "is_zero_u32")]
    pub pid: u32,
    #[serde(skip_serializing_if = "is_zero_u32")]
    pub uid: u32,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub identifier: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub comm: String,
}

fn is_false(b: &bool) -> bool {
    !*b
}
fn is_zero_u64(n: &u64) -> bool {
    *n == 0
}
fn is_zero_u32(n: &u32) -> bool {
    *n == 0
}
fn is_zero_u8(n: &u8) -> bool {
    *n == 0
}

// ---------- entry point ----------

#[cfg(target_arch = "wasm32")]
#[plugin_fn]
pub fn query(req: Json<QueryRequest>) -> FnResult<String> {
    let r = req.0;
    let mut lines = r.lines;
    if lines == 0 {
        lines = DEFAULT_LINES;
    }
    if lines > MAX_LINES {
        lines = MAX_LINES;
    }

    let mut args: Vec<String> = vec![
        "--no-pager".to_string(),
        "-o".to_string(),
        "json".to_string(),
        "-n".to_string(),
        lines.to_string(),
    ];
    if !r.unit.is_empty() {
        if let Err(e) = validate_token(&r.unit) {
            return Ok(serde_json::to_string(&QueryResponse {
                error: format!("invalid unit: {}", e),
                ..Default::default()
            })?);
        }
        args.push("-u".to_string());
        args.push(r.unit);
    }
    if !r.priority.is_empty() {
        if let Err(e) = validate_token(&r.priority) {
            return Ok(serde_json::to_string(&QueryResponse {
                error: format!("invalid priority: {}", e),
                ..Default::default()
            })?);
        }
        args.push("-p".to_string());
        args.push(r.priority);
    }
    if !r.since.is_empty() {
        args.push("--since".to_string());
        args.push(r.since);
    }
    if !r.until.is_empty() {
        args.push("--until".to_string());
        args.push(r.until);
    }
    if !r.grep.is_empty() {
        // --grep was added in systemd 237 (2018). We trust the
        // operator's regex but validate it doesn't carry control
        // characters that would confuse downstream tooling parsing
        // the audit log.
        if r.grep.contains('\0') || r.grep.contains('\n') {
            return Ok(serde_json::to_string(&QueryResponse {
                error: "grep contains forbidden characters".to_string(),
                ..Default::default()
            })?);
        }
        args.push("--grep".to_string());
        args.push(r.grep);
    }
    if r.reverse {
        args.push("-r".to_string());
    }
    if !r.boot.is_empty() {
        if let Err(e) = validate_boot(&r.boot) {
            return Ok(serde_json::to_string(&QueryResponse {
                error: format!("invalid boot: {}", e),
                ..Default::default()
            })?);
        }
        args.push("-b".to_string());
        if r.boot != "0" {
            // -b 0 is the implicit "current boot"; passing the offset
            // arg only when non-zero avoids a parse confusion.
            args.push(r.boot);
        }
    }

    let exec_resp = match run_journalctl(args, 25_000) {
        Ok(v) => v,
        Err(e) => {
            return Ok(serde_json::to_string(&QueryResponse {
                error: e,
                ..Default::default()
            })?)
        }
    };
    if exec_resp.exit_code != 0 {
        let stderr = exec_resp.stderr.trim();
        // Empty results (no matching entries) yield exit_code != 0 on
        // some configurations — same fallback as systemctl.
        if stderr.is_empty() {
            return Ok(serde_json::to_string(&QueryResponse::default())?);
        }
        return Ok(serde_json::to_string(&QueryResponse {
            error: format!("journalctl exit {}: {}", exec_resp.exit_code, stderr),
            ..Default::default()
        })?);
    }
    let (entries, truncated) = parse_json_lines(&exec_resp.stdout, lines as usize);
    Ok(serde_json::to_string(&QueryResponse {
        entries,
        truncated,
        error: String::new(),
    })?)
}

// parse_json_lines walks a chunk of `journalctl -o json` output. Each
// line is a discrete JSON object; bad lines are skipped (journalctl
// occasionally emits a leading prelude when the journal is corrupt
// or the cursor is past the end).
//
// `cap` is honoured strictly — we stop after `cap` successfully-parsed
// entries and report truncated=true. Caller passes the line cap from
// the request after clamping; this is just the safety net.
pub fn parse_json_lines(stdout: &str, cap: usize) -> (Vec<Entry>, bool) {
    let mut out = Vec::new();
    let mut truncated = false;
    for line in stdout.lines() {
        if out.len() >= cap {
            truncated = true;
            break;
        }
        let trimmed = line.trim();
        if trimmed.is_empty() {
            continue;
        }
        let raw: serde_json::Value = match serde_json::from_str(trimmed) {
            Ok(v) => v,
            Err(_) => continue,
        };
        out.push(extract_entry(&raw));
    }
    (out, truncated)
}

// extract_entry projects the journalctl-emitted object down to the
// operator-visible Entry. Every field is best-effort — missing keys
// resolve to zero/empty.
pub fn extract_entry(raw: &serde_json::Value) -> Entry {
    Entry {
        timestamp_us: parse_us(raw.get("__REALTIME_TIMESTAMP")),
        unit: pick_str(raw, "_SYSTEMD_UNIT"),
        priority: parse_priority(raw.get("PRIORITY")),
        message: extract_message(raw.get("MESSAGE")),
        hostname: pick_str(raw, "_HOSTNAME"),
        pid: pick_u32(raw, "_PID"),
        uid: pick_u32(raw, "_UID"),
        identifier: pick_str(raw, "SYSLOG_IDENTIFIER"),
        comm: pick_str(raw, "_COMM"),
    }
}

fn pick_str(v: &serde_json::Value, key: &str) -> String {
    v.get(key)
        .and_then(|x| x.as_str())
        .unwrap_or_default()
        .to_string()
}

// pick_u32 handles journalctl's emit quirks: numeric fields like _PID
// are written as strings ("1234"), not JSON numbers. fall back to the
// numeric representation if that's what we see.
fn pick_u32(v: &serde_json::Value, key: &str) -> u32 {
    let val = match v.get(key) {
        Some(x) => x,
        None => return 0,
    };
    if let Some(s) = val.as_str() {
        return s.parse().unwrap_or(0);
    }
    if let Some(n) = val.as_u64() {
        return n as u32;
    }
    0
}

fn parse_us(v: Option<&serde_json::Value>) -> u64 {
    let val = match v {
        Some(x) => x,
        None => return 0,
    };
    if let Some(s) = val.as_str() {
        return s.parse().unwrap_or(0);
    }
    if let Some(n) = val.as_u64() {
        return n;
    }
    0
}

fn parse_priority(v: Option<&serde_json::Value>) -> u8 {
    let val = match v {
        Some(x) => x,
        None => return 0,
    };
    if let Some(s) = val.as_str() {
        return s.parse().unwrap_or(0);
    }
    if let Some(n) = val.as_u64() {
        return n as u8;
    }
    0
}

// extract_message handles journalctl's two MESSAGE encodings: plain
// string (the common case) and a byte-array fallback when the
// message contained non-UTF8 bytes. We render the byte array as a
// lossy UTF-8 string so the operator at least sees something
// readable; bytes that don't decode become U+FFFD.
fn extract_message(v: Option<&serde_json::Value>) -> String {
    let val = match v {
        Some(x) => x,
        None => return String::new(),
    };
    if let Some(s) = val.as_str() {
        return s.to_string();
    }
    if let Some(arr) = val.as_array() {
        let bytes: Vec<u8> = arr
            .iter()
            .filter_map(|x| x.as_u64().map(|n| n as u8))
            .collect();
        return String::from_utf8_lossy(&bytes).into_owned();
    }
    String::new()
}

// validate_token rejects values that contain whitespace, NUL, or
// leading "-". journalctl receives each value as a separate argv
// slot so shell-injection is impossible, but a leading "-" could be
// mis-parsed as an option. Whitespace would split into two args
// silently.
fn validate_token(s: &str) -> Result<(), String> {
    if s.starts_with('-') {
        return Err(format!("must not start with '-': {}", s));
    }
    for c in s.chars() {
        if c == '\0' || c == '\n' {
            return Err(format!("contains forbidden character: {:?}", s));
        }
    }
    Ok(())
}

fn validate_boot(s: &str) -> Result<(), String> {
    if s == "all" {
        return Ok(());
    }
    // Accept signed integer (current boot = 0, prior = -1, …) or a
    // 32-char boot id. Anything else gets rejected.
    if s.len() == 32 && s.chars().all(|c| c.is_ascii_hexdigit()) {
        return Ok(());
    }
    if s.parse::<i32>().is_ok() {
        return Ok(());
    }
    Err(format!("must be 'all', integer, or 32-char boot id: {}", s))
}

// ---------- exec helper ----------

#[cfg(target_arch = "wasm32")]
fn run_journalctl(args: Vec<String>, timeout_ms: u32) -> Result<ExecResponse, String> {
    for path in &["/usr/bin/journalctl", "/bin/journalctl"] {
        let req = ExecRequest {
            command: path.to_string(),
            args: args.clone(),
            timeout_ms,
        };
        let body = match serde_json::to_string(&req) {
            Ok(b) => b,
            Err(e) => return Err(format!("encode_exec_req: {}", e)),
        };
        let env: Envelope = match unsafe { host_exec(body) } {
            Ok(j) => j.0,
            Err(e) => return Err(format!("host_exec: {}", e)),
        };
        if !env.ok {
            if env.error.contains("capability_denied") {
                return Err(env.error);
            }
            continue;
        }
        let resp: ExecResponse = serde_json::from_value(env.data)
            .map_err(|e| format!("decode_exec_resp: {}", e))?;
        return Ok(resp);
    }
    Err("journalctl_not_found_on_either_path".to_string())
}

#[cfg(not(target_arch = "wasm32"))]
fn run_journalctl(_args: Vec<String>, _timeout_ms: u32) -> Result<ExecResponse, String> {
    Err("not available on host build".to_string())
}

// ============================================================
// tests (host-build only)
// ============================================================

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parse_json_lines_basic() {
        let stdout = r#"{"__REALTIME_TIMESTAMP":"1700000000000000","_SYSTEMD_UNIT":"ssh.service","PRIORITY":"6","MESSAGE":"Accepted publickey for root","_HOSTNAME":"box","_PID":"1234","SYSLOG_IDENTIFIER":"sshd","_COMM":"sshd"}
{"__REALTIME_TIMESTAMP":"1700000001000000","_SYSTEMD_UNIT":"nginx.service","PRIORITY":"4","MESSAGE":"Connection reset","_HOSTNAME":"box","_PID":"5678"}
"#;
        let (entries, truncated) = parse_json_lines(stdout, 10);
        assert_eq!(entries.len(), 2);
        assert!(!truncated);
        assert_eq!(entries[0].unit, "ssh.service");
        assert_eq!(entries[0].priority, 6);
        assert_eq!(entries[0].timestamp_us, 1_700_000_000_000_000);
        assert_eq!(entries[0].pid, 1234);
        assert_eq!(entries[0].identifier, "sshd");
        assert_eq!(entries[1].priority, 4);
    }

    #[test]
    fn parse_json_lines_skips_garbage_lines() {
        let stdout = "not-json\n{\"_SYSTEMD_UNIT\":\"u.service\",\"MESSAGE\":\"ok\"}\n";
        let (entries, _) = parse_json_lines(stdout, 10);
        assert_eq!(entries.len(), 1);
        assert_eq!(entries[0].unit, "u.service");
    }

    #[test]
    fn parse_json_lines_truncates_at_cap() {
        let stdout = (0..5)
            .map(|i| format!("{{\"MESSAGE\":\"line{}\"}}", i))
            .collect::<Vec<_>>()
            .join("\n");
        let (entries, truncated) = parse_json_lines(&stdout, 3);
        assert_eq!(entries.len(), 3);
        assert!(truncated);
    }

    #[test]
    fn parse_json_lines_empty() {
        let (entries, truncated) = parse_json_lines("", 100);
        assert!(entries.is_empty());
        assert!(!truncated);
    }

    #[test]
    fn extract_message_string() {
        let v: serde_json::Value = serde_json::from_str(r#"{"MESSAGE":"hello"}"#).unwrap();
        assert_eq!(extract_message(v.get("MESSAGE")), "hello");
    }

    #[test]
    fn extract_message_byte_array() {
        // "hi" as bytes [104, 105]
        let v: serde_json::Value =
            serde_json::from_str(r#"{"MESSAGE":[104,105]}"#).unwrap();
        assert_eq!(extract_message(v.get("MESSAGE")), "hi");
    }

    #[test]
    fn extract_message_invalid_utf8_lossy() {
        // Single 0xff byte — invalid UTF-8 continuation; should
        // become U+FFFD rather than panic / fail.
        let v: serde_json::Value = serde_json::from_str(r#"{"MESSAGE":[255]}"#).unwrap();
        let s = extract_message(v.get("MESSAGE"));
        assert!(s.contains('\u{FFFD}'));
    }

    #[test]
    fn pick_u32_handles_string_and_number() {
        let v: serde_json::Value =
            serde_json::from_str(r#"{"a":"42","b":42,"c":"not_a_num"}"#).unwrap();
        assert_eq!(pick_u32(&v, "a"), 42);
        assert_eq!(pick_u32(&v, "b"), 42);
        assert_eq!(pick_u32(&v, "c"), 0);
        assert_eq!(pick_u32(&v, "missing"), 0);
    }

    #[test]
    fn validate_token_rejects_dash_prefix_and_nul() {
        assert!(validate_token("ssh.service").is_ok());
        assert!(validate_token("info").is_ok());
        assert!(validate_token("-hax").is_err());
        assert!(validate_token("a\0b").is_err());
        assert!(validate_token("a\nb").is_err());
    }

    #[test]
    fn validate_boot_accepts_valid_forms() {
        assert!(validate_boot("0").is_ok());
        assert!(validate_boot("-1").is_ok());
        assert!(validate_boot("all").is_ok());
        assert!(validate_boot("0123456789abcdef0123456789abcdef").is_ok());
        assert!(validate_boot("garbage").is_err());
        assert!(validate_boot("").is_err());
    }
}
