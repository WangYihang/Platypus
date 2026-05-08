// sys-procs-windows — Win32_Process via PowerShell + CIM.
//
// Strategy: shell out to powershell.exe with a one-liner that:
//   1. forces UTF-8 stdout encoding (default on Windows PowerShell 5
//      is UTF-16-LE; without this override the Rust JSON parser
//      sees a BOM + interleaved zeros and dies)
//   2. Get-CimInstance Win32_Process — modern replacement for WMIC
//      (deprecated since Server 2012)
//   3. Selects only the columns we need so the pipeline payload is
//      bounded
//   4. ConvertTo-Json -Compress (single-line)
//
// Wire shape: same protojson v2pb.ProcessListResponse the linux /
// darwin variants emit. The agent-side bridge unmarshals straight
// through (see internal/agent/plugin/bridge/processlist.go).
//
// Fields explicitly left empty/zero in v1:
//   - user (would need Win32_Process.GetOwner() per row, slow)
//   - cpu_percent (needs two CIM samples; deferred to v2)
//   - mem_percent (needs Win32_OperatingSystem total; deferred)
//   - status (Win32_Process.ExecutionState is mostly nil; we report
//     a generic "Running" string for transparency)

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

#[derive(Deserialize, Default)]
struct ProcessListRequest {
    #[serde(default)]
    top_n: u32,
    #[serde(default)]
    sort_by: String,
}

#[derive(Serialize, Default)]
pub struct ProcessInfo {
    pub pid: u32,
    pub ppid: u32,
    pub user: String,
    pub name: String,
    pub cmdline: String,
    pub status: String,
    #[serde(rename = "cpuPercent")]
    pub cpu_percent: f64,
    #[serde(rename = "memPercent")]
    pub mem_percent: f64,
    #[serde(rename = "rssBytes")]
    pub rss_bytes: u64,
}

#[derive(Serialize, Default)]
struct ProcessListResponse {
    processes: Vec<ProcessInfo>,
    #[serde(rename = "totalCount", skip_serializing_if = "is_zero_u32")]
    total_count: u32,
    #[serde(skip_serializing_if = "String::is_empty")]
    error: String,
}

fn is_zero_u32(n: &u32) -> bool {
    *n == 0
}

const PROC_LIST_CAP: u32 = 500;

const PS_SCRIPT: &str = "[Console]::OutputEncoding = [System.Text.Encoding]::UTF8; \
    Get-CimInstance Win32_Process | \
    Select-Object ProcessId,ParentProcessId,Name,CommandLine,WorkingSetSize | \
    ConvertTo-Json -Compress -Depth 2";

#[cfg(target_arch = "wasm32")]
#[plugin_fn]
pub fn process_list(req: Json<ProcessListRequest>) -> FnResult<String> {
    let r = req.0;

    let exec_resp = match run_powershell(25_000) {
        Ok(v) => v,
        Err(e) => {
            return Ok(serde_json::to_string(&ProcessListResponse {
                processes: Vec::new(),
                total_count: 0,
                error: e,
            })?)
        }
    };
    if exec_resp.exit_code != 0 {
        return Ok(serde_json::to_string(&ProcessListResponse {
            processes: Vec::new(),
            total_count: 0,
            error: format!(
                "powershell exit {}: {}",
                exec_resp.exit_code,
                exec_resp.stderr.trim()
            ),
        })?);
    }

    let mut procs = parse_powershell_json(&exec_resp.stdout);
    let total = procs.len() as u32;

    let sort_by = r.sort_by.as_str();
    procs.sort_by(|a, b| match sort_by {
        "mem" | "rss" => b.rss_bytes.cmp(&a.rss_bytes),
        "pid" => a.pid.cmp(&b.pid),
        // No cpu_percent populated; "cpu" falls through to RSS.
        _ => b.rss_bytes.cmp(&a.rss_bytes),
    });

    let mut top_n = r.top_n;
    if top_n == 0 || top_n > PROC_LIST_CAP {
        top_n = PROC_LIST_CAP;
    }
    if procs.len() > top_n as usize {
        procs.truncate(top_n as usize);
    }

    Ok(serde_json::to_string(&ProcessListResponse {
        processes: procs,
        total_count: total,
        error: String::new(),
    })?)
}

// parse_powershell_json handles three shapes the PowerShell pipeline
// emits depending on row count:
//   - Empty pipeline → "" (or "null"). We return [].
//   - Single row     → bare object `{...}`. We wrap into a 1-element vec.
//   - Multi row      → JSON array `[{...},{...}]`. Standard parse.
//
// Each row's fields use PowerShell's PascalCase names:
//   ProcessId / ParentProcessId / Name / CommandLine / WorkingSetSize.
pub fn parse_powershell_json(stdout: &str) -> Vec<ProcessInfo> {
    let trimmed = stdout.trim();
    if trimmed.is_empty() || trimmed == "null" {
        return Vec::new();
    }
    let v: serde_json::Value = match serde_json::from_str(trimmed) {
        Ok(v) => v,
        Err(_) => return Vec::new(),
    };
    let rows: Vec<&serde_json::Value> = match &v {
        serde_json::Value::Array(arr) => arr.iter().collect(),
        serde_json::Value::Object(_) => vec![&v],
        _ => return Vec::new(),
    };
    rows.into_iter().filter_map(extract_row).collect()
}

fn extract_row(row: &serde_json::Value) -> Option<ProcessInfo> {
    let obj = row.as_object()?;
    let pid = pick_u32(obj, "ProcessId");
    if pid == 0 {
        // PowerShell sometimes emits the System Idle Process (pid 0).
        // Drop it; consumer's UI doesn't need to show it as a row.
        return None;
    }
    let ppid = pick_u32(obj, "ParentProcessId");
    let name = obj
        .get("Name")
        .and_then(|x| x.as_str())
        .unwrap_or_default()
        .to_string();
    let cmdline = obj
        .get("CommandLine")
        .and_then(|x| x.as_str())
        .unwrap_or_default()
        .to_string();
    let rss_bytes = pick_u64(obj, "WorkingSetSize");

    Some(ProcessInfo {
        pid,
        ppid,
        user: String::new(),
        name,
        cmdline,
        status: "Running".to_string(),
        cpu_percent: 0.0,
        mem_percent: 0.0,
        rss_bytes,
    })
}

// pick_u32 / pick_u64 handle both number and stringified-number JSON
// shapes. PowerShell sometimes serialises Win32 numerics as strings
// (especially when they exceed JS-safe integer range).
fn pick_u32(obj: &serde_json::Map<String, serde_json::Value>, key: &str) -> u32 {
    let val = match obj.get(key) {
        Some(v) => v,
        None => return 0,
    };
    if let Some(n) = val.as_u64() {
        return n as u32;
    }
    if let Some(s) = val.as_str() {
        return s.parse().unwrap_or(0);
    }
    0
}

fn pick_u64(obj: &serde_json::Map<String, serde_json::Value>, key: &str) -> u64 {
    let val = match obj.get(key) {
        Some(v) => v,
        None => return 0,
    };
    if let Some(n) = val.as_u64() {
        return n;
    }
    if let Some(s) = val.as_str() {
        return s.parse().unwrap_or(0);
    }
    0
}

#[cfg(target_arch = "wasm32")]
fn run_powershell(timeout_ms: u32) -> Result<ExecResponse, String> {
    let args: Vec<String> = vec![
        "-NoProfile".to_string(),
        "-NonInteractive".to_string(),
        "-OutputFormat".to_string(),
        "Text".to_string(),
        "-Command".to_string(),
        PS_SCRIPT.to_string(),
    ];
    let req = ExecRequest {
        command: "C:\\Windows\\System32\\WindowsPowerShell\\v1.0\\powershell.exe".to_string(),
        args,
        timeout_ms,
    };
    let body = serde_json::to_string(&req).map_err(|e| format!("encode_exec_req: {}", e))?;
    let env: Envelope = unsafe {
        host_exec(body)
            .map_err(|e| format!("host_exec: {}", e))?
            .0
    };
    if !env.ok {
        return Err(env.error);
    }
    serde_json::from_value(env.data).map_err(|e| format!("decode_exec_resp: {}", e))
}

#[cfg(not(target_arch = "wasm32"))]
fn run_powershell(_timeout_ms: u32) -> Result<ExecResponse, String> {
    Err("not available on host build".to_string())
}

// ============================================================
// tests (host-build only — pure parser)
// ============================================================

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parse_array_two_rows() {
        let stdout = r#"[
            {"ProcessId":1,"ParentProcessId":0,"Name":"System","CommandLine":null,"WorkingSetSize":135168},
            {"ProcessId":4321,"ParentProcessId":1234,"Name":"powershell.exe","CommandLine":"powershell -NoProfile","WorkingSetSize":52428800}
        ]"#;
        let got = parse_powershell_json(stdout);
        assert_eq!(got.len(), 2);
        assert_eq!(got[0].pid, 1);
        assert_eq!(got[0].name, "System");
        assert_eq!(got[0].rss_bytes, 135_168);
        assert_eq!(got[0].cmdline, "");
        assert_eq!(got[1].pid, 4321);
        assert_eq!(got[1].ppid, 1234);
        assert_eq!(got[1].cmdline, "powershell -NoProfile");
        assert_eq!(got[1].rss_bytes, 52_428_800);
    }

    #[test]
    fn parse_single_object_no_wrapping_array() {
        // PowerShell ConvertTo-Json elides the array wrapper for a
        // single-row pipeline. The parser must wrap.
        let stdout = r#"{"ProcessId":42,"ParentProcessId":1,"Name":"foo.exe","CommandLine":"foo.exe --x","WorkingSetSize":1024}"#;
        let got = parse_powershell_json(stdout);
        assert_eq!(got.len(), 1);
        assert_eq!(got[0].pid, 42);
        assert_eq!(got[0].name, "foo.exe");
    }

    #[test]
    fn parse_drops_pid_zero() {
        let stdout = r#"[{"ProcessId":0,"ParentProcessId":0,"Name":"Idle","CommandLine":null,"WorkingSetSize":0}]"#;
        assert!(parse_powershell_json(stdout).is_empty());
    }

    #[test]
    fn parse_handles_stringified_numerics() {
        // Some PS environments serialize WorkingSetSize as a string.
        let stdout = r#"[{"ProcessId":"5","ParentProcessId":"1","Name":"x","CommandLine":null,"WorkingSetSize":"1048576"}]"#;
        let got = parse_powershell_json(stdout);
        assert_eq!(got.len(), 1);
        assert_eq!(got[0].pid, 5);
        assert_eq!(got[0].rss_bytes, 1_048_576);
    }

    #[test]
    fn parse_empty_or_null() {
        assert!(parse_powershell_json("").is_empty());
        assert!(parse_powershell_json("   \n  ").is_empty());
        assert!(parse_powershell_json("null").is_empty());
    }

    #[test]
    fn parse_drops_malformed_input() {
        assert!(parse_powershell_json("not json").is_empty());
    }

    #[test]
    fn parse_status_default_running() {
        let stdout =
            r#"{"ProcessId":7,"ParentProcessId":1,"Name":"a","CommandLine":null,"WorkingSetSize":1}"#;
        let got = parse_powershell_json(stdout);
        assert_eq!(got[0].status, "Running");
    }

    #[test]
    fn parse_handles_null_commandline() {
        // System processes commonly have CommandLine: null.
        let stdout = r#"{"ProcessId":4,"ParentProcessId":0,"Name":"System","CommandLine":null,"WorkingSetSize":24576}"#;
        let got = parse_powershell_json(stdout);
        assert_eq!(got[0].cmdline, "");
    }
}
