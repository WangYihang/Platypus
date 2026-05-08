// sys-log-windows — Windows Event Log query via Get-WinEvent.
//
// Builds a -FilterHashtable from the journald-style filter knobs,
// pipes Get-WinEvent through a Select-Object that renders each
// event into a stable JSON shape, and parses the resulting array
// into the cross-OS JournalEntry shape used by sys-journald-linux
// and sys-log-darwin.
//
// Windows Event level → syslog priority mapping (LevelDisplayName):
//   "Critical"   → 0
//   "Error"      → 3
//   "Warning"    → 4
//   "Information"→ 6
//   "Verbose"    → 7
// Levels carry over via the .Level int when DisplayName is missing.

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
    boot: String,
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

// PowerShell record shape — matches the projection in PS_TEMPLATE.
#[derive(Deserialize, Default)]
struct PsRecord {
    #[serde(default, rename = "providerName")]
    provider_name: String,
    #[serde(default, rename = "logName")]
    log_name: String,
    #[serde(default, rename = "level")]
    level: u8,
    #[serde(default, rename = "levelDisplayName")]
    level_display_name: String,
    #[serde(default)]
    message: String,
    // unix milliseconds since epoch (TimeCreated.ToUniversalTime
    // → Subtract epoch → TotalMilliseconds).
    #[serde(default, rename = "timestampMs")]
    timestamp_ms: u64,
    #[serde(default, rename = "machineName")]
    machine_name: String,
    #[serde(default, rename = "processId")]
    process_id: u32,
    #[serde(default, rename = "userId")]
    user_id: String,
}

// ---- entry point ----

#[cfg(target_arch = "wasm32")]
#[plugin_fn]
pub fn query(req: Json<QueryRequest>) -> FnResult<String> {
    let r = req.0;
    let lines = effective_line_cap(r.lines);
    let script = build_ps_script(&r, lines);
    let exec_resp = match run_powershell(&script, 60_000) {
        Ok(v) => v,
        Err(e) => return ok_response(QueryResponse { error: e, ..Default::default() }),
    };
    if exec_resp.exit_code != 0 {
        return ok_response(QueryResponse {
            error: format!(
                "powershell exit {}: {}",
                exec_resp.exit_code,
                exec_resp.stderr.trim()
            ),
            ..Default::default()
        });
    }
    let entries = parse_ps_output(&exec_resp.stdout);
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

// build_ps_script composes the PowerShell pipeline. The key
// trick: assemble the FilterHashtable as a here-string, then pipe
// through Get-WinEvent | Select-Object | ConvertTo-Json so the
// shape is stable across PS versions.
fn build_ps_script(r: &QueryRequest, lines: u32) -> String {
    let mut filters: Vec<String> = Vec::new();
    // Default to the System log when no unit is given — most
    // operator queries land there.
    let log_name = if r.unit.contains('/') || r.unit.eq_ignore_ascii_case("System")
        || r.unit.eq_ignore_ascii_case("Application") || r.unit.eq_ignore_ascii_case("Security")
    {
        // unit is itself a LogName.
        r.unit.clone()
    } else {
        "System".to_string()
    };
    filters.push(format!("LogName = '{}'", escape_ps_str(&log_name)));
    if !r.unit.is_empty() && log_name == "System" && !r.unit.eq_ignore_ascii_case("System") {
        filters.push(format!("ProviderName = '{}'", escape_ps_str(&r.unit)));
    }
    if !r.priority.is_empty() {
        if let Some(level) = priority_to_level(&r.priority) {
            filters.push(format!("Level = {}", level));
        }
    }
    if !r.since.is_empty() {
        filters.push(format!("StartTime = (Get-Date '{}')", escape_ps_str(&r.since)));
    }
    if !r.until.is_empty() {
        filters.push(format!("EndTime = (Get-Date '{}')", escape_ps_str(&r.until)));
    }
    let hashtable = format!("@{{ {} }}", filters.join("; "));
    let grep_filter = if r.grep.is_empty() {
        String::new()
    } else {
        format!("| Where-Object {{ $_.Message -match '{}' }}", escape_ps_regex(&r.grep))
    };
    format!(
        r#"[Console]::OutputEncoding = [System.Text.Encoding]::UTF8;
$evts = Get-WinEvent -FilterHashtable {hashtable} -MaxEvents {lines} -ErrorAction SilentlyContinue {grep_filter}
$out = $evts | Select-Object `
    @{{Name='providerName';      Expression={{[string]$_.ProviderName}}}}, `
    @{{Name='logName';           Expression={{[string]$_.LogName}}}}, `
    @{{Name='level';             Expression={{[int]$_.Level}}}}, `
    @{{Name='levelDisplayName';  Expression={{[string]$_.LevelDisplayName}}}}, `
    @{{Name='message';           Expression={{[string]$_.Message}}}}, `
    @{{Name='timestampMs';       Expression={{[int64]($_.TimeCreated.ToUniversalTime().Subtract([datetime]'1970-01-01').TotalMilliseconds)}}}}, `
    @{{Name='machineName';       Expression={{[string]$_.MachineName}}}}, `
    @{{Name='processId';         Expression={{[int]$_.ProcessId}}}}, `
    @{{Name='userId';            Expression={{if ($_.UserId -ne $null) {{ [string]$_.UserId }} else {{ '' }}}}}}
$out | ConvertTo-Json -Compress -Depth 4"#
    )
}

fn priority_to_level(priority: &str) -> Option<u8> {
    // Windows Event Levels (Get-WinEvent -Level): 1=Critical,
    // 2=Error, 3=Warning, 4=Informational, 5=Verbose.
    Some(match priority {
        "0" | "emerg" | "alert" | "crit" | "critical" => 1,
        "3" | "err" | "error" => 2,
        "4" | "warn" | "warning" => 3,
        "6" | "info" => 4,
        "7" | "debug" | "verbose" => 5,
        _ => return None,
    })
}

fn escape_ps_str(s: &str) -> String {
    s.replace('\'', "''")
}

fn escape_ps_regex(s: &str) -> String {
    // Inside a Where-Object -match, Powershell uses .NET regex.
    // Escape single quotes (terminate the literal) and backslashes;
    // leave other regex metacharacters alone — operators may want
    // them literal or as patterns at their discretion.
    s.replace('\'', "''").replace('\\', r"\\")
}

// ---- pure parser ----

fn parse_ps_output(stdout: &str) -> Vec<Entry> {
    let trimmed = stdout.trim();
    if trimmed.is_empty() || trimmed == "null" {
        return Vec::new();
    }
    let raw: Vec<PsRecord> = if let Ok(v) = serde_json::from_str::<Vec<PsRecord>>(trimmed) {
        v
    } else if let Ok(v) = serde_json::from_str::<PsRecord>(trimmed) {
        vec![v]
    } else {
        return Vec::new();
    };
    raw.into_iter().map(record_to_entry).collect()
}

fn record_to_entry(rec: PsRecord) -> Entry {
    let unit = if !rec.provider_name.is_empty() {
        rec.provider_name.clone()
    } else {
        rec.log_name.clone()
    };
    Entry {
        timestamp_us: rec.timestamp_ms.saturating_mul(1_000),
        unit,
        priority: level_to_priority(&rec.level_display_name, rec.level),
        message: rec.message,
        hostname: rec.machine_name,
        pid: rec.process_id,
        identifier: rec.provider_name,
        comm: rec.user_id,
        ..Default::default()
    }
}

fn level_to_priority(display: &str, level: u8) -> u8 {
    let by_name = match display.to_ascii_lowercase().as_str() {
        "critical" => Some(0),
        "error" => Some(3),
        "warning" => Some(4),
        "informational" | "information" => Some(6),
        "verbose" => Some(7),
        _ => None,
    };
    if let Some(p) = by_name {
        return p;
    }
    match level {
        1 => 0,
        2 => 3,
        3 => 4,
        4 => 6,
        5 => 7,
        _ => 0,
    }
}

// ---- exec helper ----

#[cfg(target_arch = "wasm32")]
fn run_powershell(script: &str, timeout_ms: u32) -> Result<ExecResponse, String> {
    let req = ExecRequest {
        command: "C:\\Windows\\System32\\WindowsPowerShell\\v1.0\\powershell.exe"
            .to_string(),
        args: vec![
            "-NoProfile".to_string(),
            "-NonInteractive".to_string(),
            "-OutputFormat".to_string(),
            "Text".to_string(),
            "-Command".to_string(),
            script.to_string(),
        ],
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
fn run_powershell(_script: &str, _timeout_ms: u32) -> Result<ExecResponse, String> {
    Err("not available on host build".to_string())
}

// ============================================================
// Pure-function unit tests
// ============================================================
#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn cap_lines_default_and_max() {
        assert_eq!(effective_line_cap(0), DEFAULT_LINES);
        assert_eq!(effective_line_cap(50), 50);
        assert_eq!(effective_line_cap(99_999), HARD_LINES_CAP);
    }

    #[test]
    fn priority_to_level_mapping() {
        assert_eq!(priority_to_level("error"), Some(2));
        assert_eq!(priority_to_level("warn"), Some(3));
        assert_eq!(priority_to_level("info"), Some(4));
        assert_eq!(priority_to_level("debug"), Some(5));
        assert_eq!(priority_to_level("crit"), Some(1));
        assert_eq!(priority_to_level("nonsense"), None);
        assert_eq!(priority_to_level("3"), Some(2));
    }

    #[test]
    fn level_to_priority_by_display_name() {
        assert_eq!(level_to_priority("Critical", 0), 0);
        assert_eq!(level_to_priority("Error", 0), 3);
        assert_eq!(level_to_priority("Warning", 0), 4);
        assert_eq!(level_to_priority("Information", 0), 6);
    }

    #[test]
    fn level_to_priority_falls_back_to_int() {
        assert_eq!(level_to_priority("", 1), 0);
        assert_eq!(level_to_priority("", 2), 3);
        assert_eq!(level_to_priority("", 4), 6);
        assert_eq!(level_to_priority("Unknown", 5), 7);
    }

    #[test]
    fn build_script_includes_log_name() {
        let r = QueryRequest::default();
        let s = build_ps_script(&r, 100);
        assert!(s.contains("LogName = 'System'"));
        assert!(s.contains("Get-WinEvent"));
        assert!(s.contains("MaxEvents 100"));
    }

    #[test]
    fn build_script_with_unit_routes_provider() {
        let r = QueryRequest { unit: "sshd".to_string(), ..Default::default() };
        let s = build_ps_script(&r, 100);
        assert!(s.contains("ProviderName = 'sshd'"));
        // No '/' in unit → defaults to System log + ProviderName filter.
        assert!(s.contains("LogName = 'System'"));
    }

    #[test]
    fn build_script_with_log_name_uses_it_directly() {
        let r = QueryRequest { unit: "Application".to_string(), ..Default::default() };
        let s = build_ps_script(&r, 100);
        assert!(s.contains("LogName = 'Application'"));
    }

    #[test]
    fn build_script_with_priority_adds_level() {
        let r = QueryRequest { priority: "error".to_string(), ..Default::default() };
        let s = build_ps_script(&r, 100);
        assert!(s.contains("Level = 2"));
    }

    #[test]
    fn build_script_with_grep_adds_match() {
        let r = QueryRequest { grep: "failed".to_string(), ..Default::default() };
        let s = build_ps_script(&r, 100);
        assert!(s.contains("Where-Object"));
        assert!(s.contains("'failed'"));
    }

    #[test]
    fn parse_ps_array_output() {
        let json = r#"[
            {"providerName":"Service Control Manager","logName":"System","level":4,"levelDisplayName":"Information","message":"hello","timestampMs":1700000000000,"machineName":"WIN10","processId":1234,"userId":"S-1-5-18"},
            {"providerName":"foo","logName":"System","level":2,"levelDisplayName":"Error","message":"world","timestampMs":1700000001000,"machineName":"WIN10","processId":42,"userId":""}
        ]"#;
        let entries = parse_ps_output(json);
        assert_eq!(entries.len(), 2);
        assert_eq!(entries[0].unit, "Service Control Manager");
        assert_eq!(entries[0].priority, 6);
        assert_eq!(entries[0].pid, 1234);
        assert_eq!(entries[0].timestamp_us, 1700000000000 * 1_000);
        assert_eq!(entries[1].priority, 3);
    }

    #[test]
    fn parse_ps_single_object() {
        let json = r#"{"providerName":"x","logName":"System","level":3,"levelDisplayName":"Warning","message":"y","timestampMs":0}"#;
        let entries = parse_ps_output(json);
        assert_eq!(entries.len(), 1);
        assert_eq!(entries[0].priority, 4);
    }

    #[test]
    fn parse_empty_yields_empty() {
        assert!(parse_ps_output("").is_empty());
        assert!(parse_ps_output("null").is_empty());
        assert!(parse_ps_output("not json").is_empty());
    }

    #[test]
    fn escape_ps_str_doubles_quote() {
        assert_eq!(escape_ps_str("alice's"), "alice''s");
    }
}
