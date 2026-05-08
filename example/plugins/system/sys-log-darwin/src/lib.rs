// sys-log-darwin — macOS Unified Logging via `log show --style ndjson`.
//
// `log show` outputs one JSON object per line (--style ndjson).
// Field naming differs from journald: traceID, processImagePath,
// messageType, eventMessage, … We translate to the JournalEntry
// shape (timestamp_us, unit, priority, message, hostname, pid,
// uid, identifier, comm) used by sys-journald-linux so a unified
// "fleet logs" UI works the same way everywhere.

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

// ---- request / response wire shapes (mirror sys-journald-linux) ----

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
    boot: String, // unused on darwin (no boot-id concept exposed by `log show`)
}

#[derive(Serialize, Default)]
struct QueryResponse {
    entries: Vec<Entry>,
    #[serde(skip_serializing_if = "is_false")]
    truncated: bool,
    #[serde(skip_serializing_if = "String::is_empty")]
    error: String,
}

#[derive(Serialize, Default, Debug, PartialEq)]
struct Entry {
    #[serde(rename = "timestampUs", skip_serializing_if = "is_zero_u64")]
    timestamp_us: u64,
    #[serde(skip_serializing_if = "String::is_empty")]
    unit: String,
    #[serde(skip_serializing_if = "is_zero_u8")]
    priority: u8,
    #[serde(skip_serializing_if = "String::is_empty")]
    message: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    hostname: String,
    #[serde(skip_serializing_if = "is_zero_u32")]
    pid: u32,
    #[serde(skip_serializing_if = "is_zero_u32")]
    uid: u32,
    #[serde(skip_serializing_if = "String::is_empty")]
    identifier: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    comm: String,
}

fn is_false(b: &bool) -> bool { !*b }
fn is_zero_u64(n: &u64) -> bool { *n == 0 }
fn is_zero_u32(n: &u32) -> bool { *n == 0 }
fn is_zero_u8(n: &u8) -> bool { *n == 0 }

const DEFAULT_LINES: u32 = 200;
const HARD_LINES_CAP: u32 = 5_000;

// `log show` ndjson record shape.
#[derive(Deserialize, Default)]
struct LogRecord {
    #[serde(default, rename = "eventMessage")]
    event_message: String,
    #[serde(default, rename = "processID")]
    process_id: u32,
    #[serde(default, rename = "processImagePath")]
    process_image_path: String,
    #[serde(default, rename = "subsystem")]
    subsystem: String,
    #[serde(default, rename = "category")]
    category: String,
    #[serde(default, rename = "messageType")]
    message_type: String,
    #[serde(default, rename = "timestamp")]
    timestamp: String, // "2026-05-08 01:30:00.123456+0000"
}

// ---- entry point ----

#[cfg(target_arch = "wasm32")]
#[plugin_fn]
pub fn query(req: Json<QueryRequest>) -> FnResult<String> {
    let r = req.0;
    let lines = effective_line_cap(r.lines);
    let args = build_log_args(&r, lines);
    let exec_resp = match run_log(&args, 60_000) {
        Ok(v) => v,
        Err(e) => return ok_response(QueryResponse { error: e, ..Default::default() }),
    };
    if exec_resp.exit_code != 0 {
        let lower = exec_resp.stderr.to_ascii_lowercase();
        let err = if lower.contains("operation not permitted")
            || lower.contains("permission denied")
        {
            "permission_denied".to_string()
        } else {
            format!("log exit {}: {}", exec_resp.exit_code, exec_resp.stderr.trim())
        };
        return ok_response(QueryResponse { error: err, ..Default::default() });
    }
    let entries = parse_log_ndjson(&exec_resp.stdout, &r);
    let truncated = entries.len() as u32 >= lines;
    let entries = if r.reverse {
        let mut e = entries;
        e.reverse();
        e
    } else {
        entries
    };
    ok_response(QueryResponse { entries, truncated, error: String::new() })
}

fn ok_response(resp: QueryResponse) -> FnResult<String> {
    Ok(serde_json::to_string(&resp)?)
}

fn effective_line_cap(requested: u32) -> u32 {
    let n = if requested == 0 { DEFAULT_LINES } else { requested };
    n.min(HARD_LINES_CAP)
}

// build_log_args composes the `log show` argv from the request.
// Filters compose into a single --predicate string when more than
// one is set. `--style ndjson` is critical; without it we get the
// human-readable columnar format which is unparseable.
fn build_log_args(r: &QueryRequest, lines: u32) -> Vec<String> {
    let mut args = vec![
        "show".to_string(),
        "--style".to_string(),
        "ndjson".to_string(),
        "--info".to_string(),
        "--debug".to_string(),
    ];
    if !r.since.is_empty() {
        args.push("--start".to_string());
        args.push(r.since.clone());
    }
    if !r.until.is_empty() {
        args.push("--end".to_string());
        args.push(r.until.clone());
    }
    let mut predicates: Vec<String> = Vec::new();
    if let Some(p) = build_unit_predicate(&r.unit) {
        predicates.push(p);
    }
    if let Some(p) = build_priority_predicate(&r.priority) {
        predicates.push(p);
    }
    if !r.grep.is_empty() {
        predicates.push(format!(r#"eventMessage CONTAINS[c] "{}""#, escape_predicate_str(&r.grep)));
    }
    if !predicates.is_empty() {
        args.push("--predicate".to_string());
        args.push(predicates.join(" AND "));
    }
    args.push("--max".to_string());
    args.push(lines.to_string());
    args
}

fn build_unit_predicate(unit: &str) -> Option<String> {
    if unit.is_empty() {
        return None;
    }
    Some(format!(
        r#"(process == "{u}" OR processImagePath CONTAINS[c] "{u}" OR subsystem == "{u}")"#,
        u = escape_predicate_str(unit)
    ))
}

// Map syslog priority strings/numbers to a `log show` messageType
// predicate. Apple's levels:
//   0 default  1 info  2 debug  16 error  17 fault
fn build_priority_predicate(priority: &str) -> Option<String> {
    if priority.is_empty() {
        return None;
    }
    let level = match priority {
        "0" | "emerg" | "alert" | "crit" | "fault" => "fault",
        "3" | "err" | "error" => "error",
        "4" | "warn" | "warning" => "error", // log show has no warn level; map up
        "5" | "notice" => "default",
        "6" | "info" => "info",
        "7" | "debug" => "debug",
        _ => "default",
    };
    Some(format!(r#"messageType == "{level}""#))
}

fn escape_predicate_str(s: &str) -> String {
    s.replace('\\', "\\\\").replace('"', "\\\"")
}

// parse_log_ndjson walks `log show --style ndjson` output. Each
// line is one JSON object; some lines are non-record headers
// ("Filtering the log data...") that fail to parse — silently
// skip them.
fn parse_log_ndjson(stdout: &str, req: &QueryRequest) -> Vec<Entry> {
    let mut out = Vec::new();
    for line in stdout.lines() {
        let trimmed = line.trim();
        if trimmed.is_empty() || !trimmed.starts_with('{') {
            continue;
        }
        let rec: LogRecord = match serde_json::from_str(trimmed) {
            Ok(r) => r,
            Err(_) => continue,
        };
        let unit = pick_unit(&rec);
        // Apply unit/grep filter client-side too as defense in
        // depth — `log show`'s predicate is applied server-side
        // but we re-check here so the typed-bridge response is
        // always self-consistent with the request.
        if !req.unit.is_empty() && !unit_matches(&unit, &rec, &req.unit) {
            continue;
        }
        if !req.grep.is_empty() && !rec.event_message.to_ascii_lowercase().contains(&req.grep.to_ascii_lowercase()) {
            continue;
        }
        out.push(Entry {
            timestamp_us: parse_log_timestamp_us(&rec.timestamp),
            unit,
            priority: message_type_to_priority(&rec.message_type),
            message: rec.event_message,
            pid: rec.process_id,
            ..Default::default()
        });
    }
    out
}

fn pick_unit(rec: &LogRecord) -> String {
    if !rec.subsystem.is_empty() {
        return rec.subsystem.clone();
    }
    // Strip path → basename for processImagePath.
    if !rec.process_image_path.is_empty() {
        return rec
            .process_image_path
            .rsplit('/')
            .next()
            .unwrap_or(&rec.process_image_path)
            .to_string();
    }
    String::new()
}

fn unit_matches(resolved: &str, rec: &LogRecord, want: &str) -> bool {
    let lower = want.to_ascii_lowercase();
    resolved.to_ascii_lowercase().contains(&lower)
        || rec.process_image_path.to_ascii_lowercase().contains(&lower)
        || rec.subsystem.to_ascii_lowercase().contains(&lower)
}

fn message_type_to_priority(t: &str) -> u8 {
    match t.to_ascii_lowercase().as_str() {
        "fault" => 0,
        "error" => 3,
        "default" => 5,
        "info" => 6,
        "debug" => 7,
        _ => 0,
    }
}

// parse_log_timestamp_us turns "2026-05-08 01:30:00.123456+0000"
// into microseconds since epoch. Best-effort: an unparseable
// timestamp returns 0 (the field is skip_serializing_if zero).
// Hand-rolled parser to avoid pulling in chrono / time crates
// across the wasm boundary for a single field.
fn parse_log_timestamp_us(s: &str) -> u64 {
    if s.len() < 19 {
        return 0;
    }
    // YYYY-MM-DD HH:MM:SS[.frac][+/-HHMM]
    let bytes = s.as_bytes();
    let year: i64 = match std::str::from_utf8(&bytes[0..4]).ok().and_then(|x| x.parse().ok()) {
        Some(v) => v, None => return 0,
    };
    let month: u32 = match std::str::from_utf8(&bytes[5..7]).ok().and_then(|x| x.parse().ok()) {
        Some(v) => v, None => return 0,
    };
    let day: u32 = match std::str::from_utf8(&bytes[8..10]).ok().and_then(|x| x.parse().ok()) {
        Some(v) => v, None => return 0,
    };
    let hour: u32 = match std::str::from_utf8(&bytes[11..13]).ok().and_then(|x| x.parse().ok()) {
        Some(v) => v, None => return 0,
    };
    let min: u32 = match std::str::from_utf8(&bytes[14..16]).ok().and_then(|x| x.parse().ok()) {
        Some(v) => v, None => return 0,
    };
    let sec: u32 = match std::str::from_utf8(&bytes[17..19]).ok().and_then(|x| x.parse().ok()) {
        Some(v) => v, None => return 0,
    };
    // Parse optional .ffffff fractional seconds.
    let mut us = 0u32;
    let mut idx = 19;
    if idx < s.len() && bytes[idx] == b'.' {
        idx += 1;
        let frac_start = idx;
        while idx < s.len() && bytes[idx].is_ascii_digit() {
            idx += 1;
        }
        let frac_str = &s[frac_start..idx];
        // Pad / truncate to 6 digits.
        let mut padded = frac_str.to_string();
        while padded.len() < 6 {
            padded.push('0');
        }
        if padded.len() > 6 {
            padded.truncate(6);
        }
        us = padded.parse().unwrap_or(0);
    }
    let epoch = days_from_civil(year, month, day) * 86_400
        + (hour as i64) * 3_600
        + (min as i64) * 60
        + (sec as i64);
    if epoch < 0 {
        return 0;
    }
    (epoch as u64) * 1_000_000 + us as u64
}

// days_from_civil — Howard Hinnant's algorithm (chrono-free).
// Works for any Gregorian date; we trust the input from `log show`.
fn days_from_civil(y: i64, m: u32, d: u32) -> i64 {
    let y = if m <= 2 { y - 1 } else { y };
    let era = if y >= 0 { y } else { y - 399 } / 400;
    let yoe = (y - era * 400) as u64;
    let doy = ((153 * (if m > 2 { m - 3 } else { m + 9 } as u32) + 2) / 5
        + d - 1) as u64;
    let doe = yoe * 365 + yoe / 4 - yoe / 100 + doy;
    era * 146_097 + doe as i64 - 719_468
}

// ---- exec helper ----

#[cfg(target_arch = "wasm32")]
fn run_log(args: &[String], timeout_ms: u32) -> Result<ExecResponse, String> {
    let req = ExecRequest {
        command: "/usr/bin/log".to_string(),
        args: args.to_vec(),
        timeout_ms,
    };
    let body = serde_json::to_string(&req).map_err(|e| format!("encode_exec_req: {e}"))?;
    let env: Envelope = unsafe {
        host_exec(body).map_err(|e| format!("host_exec: {e}"))?.0
    };
    if !env.ok {
        return Err(env.error);
    }
    serde_json::from_value(env.data).map_err(|e| format!("decode_exec_resp: {e}"))
}

#[cfg(not(target_arch = "wasm32"))]
#[allow(dead_code)]
fn run_log(_args: &[String], _timeout_ms: u32) -> Result<ExecResponse, String> {
    Err("not available on host build".to_string())
}

// ============================================================
// Pure-function unit tests
// ============================================================
#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn cap_lines_default() {
        assert_eq!(effective_line_cap(0), DEFAULT_LINES);
        assert_eq!(effective_line_cap(50), 50);
        assert_eq!(effective_line_cap(10_000), HARD_LINES_CAP);
    }

    #[test]
    fn build_args_minimal() {
        let req = QueryRequest::default();
        let args = build_log_args(&req, 100);
        assert!(args.contains(&"show".to_string()));
        assert!(args.contains(&"ndjson".to_string()));
        assert!(args.contains(&"--max".to_string()));
        // No predicate when no filters set.
        assert!(!args.contains(&"--predicate".to_string()));
    }

    #[test]
    fn build_args_with_unit_predicate() {
        let req = QueryRequest { unit: "sshd".to_string(), ..Default::default() };
        let args = build_log_args(&req, 100);
        let pi = args.iter().position(|a| a == "--predicate").unwrap();
        let pred = &args[pi + 1];
        assert!(pred.contains("process == \"sshd\""));
    }

    #[test]
    fn build_args_with_priority_predicate() {
        let req = QueryRequest { priority: "err".to_string(), ..Default::default() };
        let args = build_log_args(&req, 100);
        let pi = args.iter().position(|a| a == "--predicate").unwrap();
        let pred = &args[pi + 1];
        assert!(pred.contains("messageType == \"error\""));
    }

    #[test]
    fn build_args_combines_predicates_with_and() {
        let req = QueryRequest {
            unit: "sshd".to_string(),
            grep: "Failed".to_string(),
            ..Default::default()
        };
        let args = build_log_args(&req, 100);
        let pi = args.iter().position(|a| a == "--predicate").unwrap();
        let pred = &args[pi + 1];
        assert!(pred.contains("AND"));
        assert!(pred.contains("CONTAINS[c] \"Failed\""));
    }

    #[test]
    fn parse_ndjson_basic() {
        let stdout = r#"{"timestamp":"2026-05-08 01:30:00.123456+0000","eventMessage":"hello","processID":42,"processImagePath":"/usr/sbin/sshd","subsystem":"","messageType":"info"}
{"timestamp":"2026-05-08 01:30:01.000000+0000","eventMessage":"world","processID":99,"processImagePath":"/usr/bin/launchd","messageType":"error"}
"#;
        let entries = parse_log_ndjson(stdout, &QueryRequest::default());
        assert_eq!(entries.len(), 2);
        assert_eq!(entries[0].message, "hello");
        assert_eq!(entries[0].unit, "sshd");
        assert_eq!(entries[0].priority, 6); // info
        assert!(entries[0].timestamp_us > 0);
        assert_eq!(entries[1].priority, 3); // error
    }

    #[test]
    fn parse_ndjson_skips_garbage_lines() {
        let stdout = "Filtering the log data...\n{\"timestamp\":\"2026-05-08 01:30:00.000000+0000\",\"eventMessage\":\"x\",\"processID\":1,\"processImagePath\":\"/sbin/launchd\",\"messageType\":\"info\"}\nnot json\n";
        let entries = parse_log_ndjson(stdout, &QueryRequest::default());
        assert_eq!(entries.len(), 1);
        assert_eq!(entries[0].message, "x");
    }

    #[test]
    fn parse_ndjson_filters_grep() {
        let stdout = r#"{"timestamp":"2026-05-08 01:30:00.000000+0000","eventMessage":"hello","processID":1,"processImagePath":"/x"}
{"timestamp":"2026-05-08 01:30:01.000000+0000","eventMessage":"world","processID":2,"processImagePath":"/y"}
"#;
        let req = QueryRequest { grep: "world".to_string(), ..Default::default() };
        let entries = parse_log_ndjson(stdout, &req);
        assert_eq!(entries.len(), 1);
        assert_eq!(entries[0].message, "world");
    }

    #[test]
    fn timestamp_parses_to_us() {
        // 2026-05-08 01:30:00.123456 UTC.
        // Reference: 2026-01-01 = 56_768 days since epoch (1970-01-01).
        let us = parse_log_timestamp_us("2026-05-08 01:30:00.123456+0000");
        assert!(us > 0);
        // Check microsecond fraction passed through.
        assert_eq!(us % 1_000_000, 123_456);
    }

    #[test]
    fn timestamp_zero_on_garbage() {
        assert_eq!(parse_log_timestamp_us(""), 0);
        assert_eq!(parse_log_timestamp_us("garbage"), 0);
    }

    #[test]
    fn unit_picks_subsystem_first() {
        let rec = LogRecord {
            subsystem: "com.apple.sshd".to_string(),
            process_image_path: "/usr/sbin/sshd".to_string(),
            ..Default::default()
        };
        assert_eq!(pick_unit(&rec), "com.apple.sshd");
    }

    #[test]
    fn unit_falls_back_to_basename() {
        let rec = LogRecord {
            process_image_path: "/usr/sbin/sshd".to_string(),
            ..Default::default()
        };
        assert_eq!(pick_unit(&rec), "sshd");
    }

    #[test]
    fn message_type_priorities() {
        assert_eq!(message_type_to_priority("fault"), 0);
        assert_eq!(message_type_to_priority("error"), 3);
        assert_eq!(message_type_to_priority("default"), 5);
        assert_eq!(message_type_to_priority("info"), 6);
        assert_eq!(message_type_to_priority("debug"), 7);
        assert_eq!(message_type_to_priority(""), 0);
    }

    #[test]
    fn escape_predicate_str_handles_quotes() {
        assert_eq!(escape_predicate_str(r#"foo"bar"#), r#"foo\"bar"#);
        assert_eq!(escape_predicate_str(r#"a\b"#), r#"a\\b"#);
    }
}
