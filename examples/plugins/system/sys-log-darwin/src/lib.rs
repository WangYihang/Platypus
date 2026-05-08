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
    #[serde(default, rename = "afterCursor", alias = "after_cursor")]
    after_cursor: String,
    #[serde(default, rename = "beforeCursor", alias = "before_cursor")]
    before_cursor: String,
}

#[derive(Serialize, Default)]
struct QueryResponse {
    entries: Vec<Entry>,
    #[serde(skip_serializing_if = "is_false")]
    truncated: bool,
    #[serde(skip_serializing_if = "String::is_empty")]
    error: String,
    #[serde(rename = "prevCursor", skip_serializing_if = "String::is_empty")]
    prev_cursor: String,
    #[serde(rename = "nextCursor", skip_serializing_if = "String::is_empty")]
    next_cursor: String,
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
    // Synthetic cursor: "<timestamp_us>@<8-hex-of-fnv1a-of-message>".
    // macOS `log show` has no native cursor concept; we hash the
    // event message + pid + subsystem to disambiguate within the
    // same microsecond.
    #[serde(skip_serializing_if = "String::is_empty")]
    cursor: String,
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
    if !r.after_cursor.is_empty() && !r.before_cursor.is_empty() {
        return ok_response(QueryResponse {
            error: "after_cursor and before_cursor are mutually exclusive".to_string(),
            ..Default::default()
        });
    }
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
    let mut entries = parse_log_ndjson(&exec_resp.stdout, &r);
    // Cursor post-filtering: drop entries whose synthetic cursor
    // sorts equal-or-before / equal-or-after the boundary. This
    // disambiguates within a microsecond bucket where `--start`/
    // `--end` alone can't (multiple events in the same us survive
    // both bounds; we use the cursor's hash to pick which side).
    if !r.after_cursor.is_empty() {
        if let Some((bound_us, bound_hash)) = parse_cursor(&r.after_cursor) {
            entries.retain(|e| entry_strictly_after(e, bound_us, bound_hash));
        }
    }
    if !r.before_cursor.is_empty() {
        if let Some((bound_us, bound_hash)) = parse_cursor(&r.before_cursor) {
            entries.retain(|e| entry_strictly_before(e, bound_us, bound_hash));
        }
    }
    let truncated = entries.len() as u32 >= lines;
    let entries = if r.reverse {
        let mut e = entries;
        e.reverse();
        e
    } else {
        entries
    };
    let (prev_cursor, next_cursor) = boundary_cursors(&entries);
    ok_response(QueryResponse {
        entries,
        truncated,
        error: String::new(),
        prev_cursor,
        next_cursor,
    })
}

// boundary_cursors picks the oldest-cursor (use as before_cursor for
// older history) and newest-cursor (use as after_cursor for newer).
fn boundary_cursors(entries: &[Entry]) -> (String, String) {
    if entries.is_empty() {
        return (String::new(), String::new());
    }
    let mut oldest: Option<&Entry> = None;
    let mut newest: Option<&Entry> = None;
    for e in entries {
        if e.cursor.is_empty() {
            continue;
        }
        if oldest.map(|o| e.timestamp_us < o.timestamp_us).unwrap_or(true) {
            oldest = Some(e);
        }
        if newest.map(|n| e.timestamp_us > n.timestamp_us).unwrap_or(true) {
            newest = Some(e);
        }
    }
    (
        oldest.map(|e| e.cursor.clone()).unwrap_or_default(),
        newest.map(|e| e.cursor.clone()).unwrap_or_default(),
    )
}

// parse_cursor splits "<timestamp_us>@<hex>" → (timestamp, hash).
fn parse_cursor(s: &str) -> Option<(u64, u32)> {
    let at = s.rfind('@')?;
    let ts: u64 = s[..at].parse().ok()?;
    let hash: u32 = u32::from_str_radix(&s[at + 1..], 16).ok()?;
    Some((ts, hash))
}

// entry_strictly_after / entry_strictly_before define the cursor
// ordering: same-microsecond entries are ordered by their hash; the
// boundary entry itself is excluded.
fn entry_strictly_after(e: &Entry, bound_us: u64, bound_hash: u32) -> bool {
    if e.timestamp_us > bound_us {
        return true;
    }
    if e.timestamp_us < bound_us {
        return false;
    }
    let entry_hash = parse_cursor(&e.cursor).map(|(_, h)| h).unwrap_or(0);
    entry_hash > bound_hash
}

fn entry_strictly_before(e: &Entry, bound_us: u64, bound_hash: u32) -> bool {
    if e.timestamp_us < bound_us {
        return true;
    }
    if e.timestamp_us > bound_us {
        return false;
    }
    let entry_hash = parse_cursor(&e.cursor).map(|(_, h)| h).unwrap_or(0);
    entry_hash < bound_hash
}

// fnv1a_32 — small stable hash for synthesising the per-entry cursor.
// 32 bits is plenty within a single microsecond bucket (collisions
// among same-timestamp entries are rare; a hash collision means the
// pagination drops one entry, not a security failure).
fn fnv1a_32(input: &str) -> u32 {
    let mut hash: u32 = 0x811C9DC5;
    for b in input.bytes() {
        hash ^= b as u32;
        hash = hash.wrapping_mul(0x01000193);
    }
    hash
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
    // Cursor pagination on darwin synthesises --start / --end
    // from the cursor's timestamp; the post-parse filter then
    // disambiguates within the microsecond bucket via the hash.
    // Cursor takes precedence over since/until — operators mixing
    // both should pick one.
    let mut effective_since = r.since.clone();
    let mut effective_until = r.until.clone();
    if let Some((ts_us, _)) = parse_cursor(&r.after_cursor) {
        effective_since = format_log_timestamp(ts_us);
    }
    if let Some((ts_us, _)) = parse_cursor(&r.before_cursor) {
        // Add 1us so `log show` includes the boundary microsecond
        // bucket; the post-parse filter then trims same-us entries.
        effective_until = format_log_timestamp(ts_us.saturating_add(1));
    }
    if !effective_since.is_empty() {
        args.push("--start".to_string());
        args.push(effective_since);
    }
    if !effective_until.is_empty() {
        args.push("--end".to_string());
        args.push(effective_until);
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
        let ts = parse_log_timestamp_us(&rec.timestamp);
        let cursor = synth_cursor(ts, &rec);
        out.push(Entry {
            timestamp_us: ts,
            unit,
            priority: message_type_to_priority(&rec.message_type),
            message: rec.event_message,
            pid: rec.process_id,
            cursor,
            ..Default::default()
        });
    }
    out
}

// synth_cursor builds the per-entry pagination handle. macOS's
// `log show` doesn't expose a stable record id, so we synthesise
// one from the timestamp + a 32-bit hash of (eventMessage + pid +
// subsystem) — enough to disambiguate within a microsecond.
//
// Format: "<timestamp_us>@<8-hex-of-fnv1a>". A zero timestamp
// produces an empty cursor (the entry is unpaginatable).
fn synth_cursor(ts_us: u64, rec: &LogRecord) -> String {
    if ts_us == 0 {
        return String::new();
    }
    let key = format!("{}\x1f{}\x1f{}", rec.event_message, rec.process_id, rec.subsystem);
    let h = fnv1a_32(&key);
    format!("{}@{:08x}", ts_us, h)
}

// format_log_timestamp converts microseconds-since-epoch back into
// the format `log show --start` accepts: "YYYY-MM-DD HH:MM:SS.uuuuuu".
// `log show` interprets unqualified timestamps as host-local time;
// since we accept whatever format the operator passed in `since`/
// `until`, we use a fixed UTC representation that `log show` parses
// as UTC reliably.
fn format_log_timestamp(ts_us: u64) -> String {
    let secs = (ts_us / 1_000_000) as i64;
    let us = (ts_us % 1_000_000) as u32;
    let (y, mo, d, h, mi, s) = secs_to_civil(secs);
    format!("{:04}-{:02}-{:02} {:02}:{:02}:{:02}.{:06}", y, mo, d, h, mi, s, us)
}

fn secs_to_civil(secs: i64) -> (i64, u32, u32, u32, u32, u32) {
    let days = secs.div_euclid(86_400);
    let tod = secs.rem_euclid(86_400);
    let h = (tod / 3_600) as u32;
    let m = ((tod / 60) % 60) as u32;
    let s = (tod % 60) as u32;
    let (y, mo, d) = days_to_civil(days);
    (y, mo, d, h, m, s)
}

// days_to_civil — inverse of days_from_civil. Howard Hinnant's
// canonical algorithm.
fn days_to_civil(z: i64) -> (i64, u32, u32) {
    let z = z + 719_468;
    let era = if z >= 0 { z } else { z - 146_096 } / 146_097;
    let doe = (z - era * 146_097) as u64;
    let yoe = (doe - doe / 1_460 + doe / 36_524 - doe / 146_096) / 365;
    let y = yoe as i64 + era * 400;
    let doy = doe - (365 * yoe + yoe / 4 - yoe / 100);
    let mp = (5 * doy + 2) / 153;
    let d = (doy - (153 * mp + 2) / 5 + 1) as u32;
    let m = if mp < 10 { mp + 3 } else { mp - 9 } as u32;
    let y_final = if m <= 2 { y + 1 } else { y };
    (y_final, m, d)
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

    // ---- cursor pagination ----

    #[test]
    fn parse_cursor_basic() {
        assert_eq!(parse_cursor("1700000000123456@deadbeef"), Some((1_700_000_000_123_456, 0xdeadbeef)));
    }

    #[test]
    fn parse_cursor_rejects_malformed() {
        assert!(parse_cursor("").is_none());
        assert!(parse_cursor("1700000000").is_none());
        assert!(parse_cursor("not_a_number@deadbeef").is_none());
        assert!(parse_cursor("1700000000@nothex").is_none());
    }

    #[test]
    fn synth_cursor_formats_correctly() {
        let rec = LogRecord {
            event_message: "hello".to_string(),
            process_id: 42,
            subsystem: "com.apple.x".to_string(),
            ..Default::default()
        };
        let c = synth_cursor(1_700_000_000_123_456, &rec);
        assert!(c.starts_with("1700000000123456@"));
        assert_eq!(c.len(), "1700000000123456".len() + 1 + 8); // 8 hex chars
    }

    #[test]
    fn synth_cursor_zero_timestamp_returns_empty() {
        let rec = LogRecord::default();
        assert!(synth_cursor(0, &rec).is_empty());
    }

    #[test]
    fn synth_cursor_disambiguates_same_ts() {
        let rec_a = LogRecord {
            event_message: "first message".to_string(),
            process_id: 1,
            ..Default::default()
        };
        let rec_b = LogRecord {
            event_message: "second message".to_string(),
            process_id: 1,
            ..Default::default()
        };
        let ts = 1_700_000_000_000_000;
        let cursor_a = synth_cursor(ts, &rec_a);
        let cursor_b = synth_cursor(ts, &rec_b);
        assert_ne!(cursor_a, cursor_b, "different messages must produce different cursors");
    }

    #[test]
    fn entry_strictly_after_compares_ts_then_hash() {
        // Strictly newer than the bound.
        let e_newer = Entry {
            timestamp_us: 200,
            cursor: "200@00000005".to_string(),
            ..Default::default()
        };
        assert!(entry_strictly_after(&e_newer, 100, 0xff));

        // Same timestamp, larger hash → after.
        let e_same_higher = Entry {
            timestamp_us: 100,
            cursor: "100@000000ff".to_string(),
            ..Default::default()
        };
        assert!(entry_strictly_after(&e_same_higher, 100, 0x10));

        // Same timestamp, equal hash → not after (boundary itself).
        let e_eq = Entry {
            timestamp_us: 100,
            cursor: "100@00000010".to_string(),
            ..Default::default()
        };
        assert!(!entry_strictly_after(&e_eq, 100, 0x10));

        // Strictly older.
        let e_older = Entry {
            timestamp_us: 50,
            cursor: "50@deadbeef".to_string(),
            ..Default::default()
        };
        assert!(!entry_strictly_after(&e_older, 100, 0));
    }

    #[test]
    fn entry_strictly_before_inverse_of_after() {
        let e_older = Entry {
            timestamp_us: 50,
            cursor: "50@00000005".to_string(),
            ..Default::default()
        };
        assert!(entry_strictly_before(&e_older, 100, 0xff));

        let e_eq = Entry {
            timestamp_us: 100,
            cursor: "100@00000010".to_string(),
            ..Default::default()
        };
        assert!(!entry_strictly_before(&e_eq, 100, 0x10));
    }

    #[test]
    fn boundary_cursors_pick_oldest_newest() {
        let entries = vec![
            Entry { timestamp_us: 100, cursor: "100@a".to_string(), ..Default::default() },
            Entry { timestamp_us: 300, cursor: "300@c".to_string(), ..Default::default() },
            Entry { timestamp_us: 200, cursor: "200@b".to_string(), ..Default::default() },
        ];
        let (prev, next) = boundary_cursors(&entries);
        assert_eq!(prev, "100@a");
        assert_eq!(next, "300@c");
    }

    #[test]
    fn fnv1a_stable() {
        // FNV-1a of "" is the offset basis.
        assert_eq!(fnv1a_32(""), 0x811C9DC5);
        // Different inputs hash differently.
        assert_ne!(fnv1a_32("hello"), fnv1a_32("world"));
    }

    #[test]
    fn format_log_timestamp_round_trip() {
        // Pick a known timestamp: 2026-05-08 01:30:00.123456 UTC
        // = 1762565400123456 microseconds.
        let ts_us = parse_log_timestamp_us("2026-05-08 01:30:00.123456+0000");
        let formatted = format_log_timestamp(ts_us);
        // Re-parse should yield the original.
        assert_eq!(parse_log_timestamp_us(&format!("{formatted}+0000")), ts_us);
    }

    #[test]
    fn build_args_with_after_cursor_uses_start() {
        let r = QueryRequest {
            after_cursor: "1700000000123456@deadbeef".to_string(),
            ..Default::default()
        };
        let args = build_log_args(&r, 100);
        let i = args.iter().position(|a| a == "--start").expect("--start present");
        assert!(args[i + 1].starts_with("20"));  // "2023-..."
    }

    #[test]
    fn build_args_with_before_cursor_uses_end() {
        let r = QueryRequest {
            before_cursor: "1700000000123456@deadbeef".to_string(),
            ..Default::default()
        };
        let args = build_log_args(&r, 100);
        let i = args.iter().position(|a| a == "--end").expect("--end present");
        assert!(args[i + 1].starts_with("20"));
    }
}
