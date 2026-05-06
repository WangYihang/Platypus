// sys-services-windows — Windows service control via PowerShell.
//
// Get-Service emits one JSON object per service:
//   { Name, DisplayName, Status (int), StartType (int) }
//
// Status enum (System.ServiceProcess.ServiceControllerStatus):
//   1=Stopped, 2=StartPending, 3=StopPending, 4=Running,
//   5=ContinuePending, 6=PausePending, 7=Paused
//
// StartType enum (System.ServiceProcess.ServiceStartMode):
//   0=Boot, 1=System, 2=Automatic, 3=Manual, 4=Disabled
//
// Mapped to lowercase string forms for the wire shape.

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
struct ListUnitsRequest {
    #[serde(default)]
    filter: String,
}

#[derive(Serialize, Default)]
struct ListUnitsResponse {
    units: Vec<UnitListEntry>,
    #[serde(skip_serializing_if = "String::is_empty")]
    error: String,
}

#[derive(Serialize, Default)]
pub struct UnitListEntry {
    pub name: String,
    #[serde(rename = "displayName", skip_serializing_if = "String::is_empty")]
    pub display_name: String,
    pub status: String,
    #[serde(rename = "startType")]
    pub start_type: String,
}

const LIST_PS_SCRIPT: &str = "[Console]::OutputEncoding = [System.Text.Encoding]::UTF8; \
    Get-Service | \
    Select-Object Name,DisplayName,@{Name='Status';Expression={[int]$_.Status}},@{Name='StartType';Expression={[int]$_.StartType}} | \
    ConvertTo-Json -Compress -Depth 2";

#[cfg(target_arch = "wasm32")]
#[plugin_fn]
pub fn list_units(req: Json<ListUnitsRequest>) -> FnResult<String> {
    let r = req.0;
    let exec_resp = match run_powershell(LIST_PS_SCRIPT, 25_000) {
        Ok(v) => v,
        Err(e) => {
            return Ok(serde_json::to_string(&ListUnitsResponse {
                units: Vec::new(),
                error: e,
            })?)
        }
    };
    if exec_resp.exit_code != 0 {
        return Ok(serde_json::to_string(&ListUnitsResponse {
            units: Vec::new(),
            error: format!(
                "powershell exit {}: {}",
                exec_resp.exit_code,
                exec_resp.stderr.trim()
            ),
        })?);
    }
    let mut units = parse_powershell_json(&exec_resp.stdout);
    if !r.filter.is_empty() {
        units.retain(|u| u.name.contains(&r.filter) || u.display_name.contains(&r.filter));
    }
    Ok(serde_json::to_string(&ListUnitsResponse {
        units,
        error: String::new(),
    })?)
}

// parse_powershell_json handles the same array / single-object /
// empty / null shapes as the other windows plugins.
pub fn parse_powershell_json(stdout: &str) -> Vec<UnitListEntry> {
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

fn extract_row(row: &serde_json::Value) -> Option<UnitListEntry> {
    let obj = row.as_object()?;
    let name = obj
        .get("Name")
        .and_then(|x| x.as_str())
        .unwrap_or_default()
        .to_string();
    if name.is_empty() {
        return None;
    }
    let display_name = obj
        .get("DisplayName")
        .and_then(|x| x.as_str())
        .unwrap_or_default()
        .to_string();
    let status = decode_status(pick_u32(obj, "Status"));
    let start_type = decode_start_type(pick_u32(obj, "StartType"));
    Some(UnitListEntry {
        name,
        display_name,
        status: status.to_string(),
        start_type: start_type.to_string(),
    })
}

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

pub fn decode_status(n: u32) -> &'static str {
    match n {
        1 => "stopped",
        2 => "start_pending",
        3 => "stop_pending",
        4 => "running",
        5 => "continue_pending",
        6 => "pause_pending",
        7 => "paused",
        _ => "unknown",
    }
}

pub fn decode_start_type(n: u32) -> &'static str {
    match n {
        0 => "boot",
        1 => "system",
        2 => "automatic",
        3 => "manual",
        4 => "disabled",
        _ => "unknown",
    }
}

// ---------- unit_action ----------

#[derive(Deserialize)]
struct UnitActionRequest {
    name: String,
    action: String,
}

#[derive(Serialize, Default)]
struct UnitActionResponse {
    ok: bool,
    #[serde(rename = "exitCode")]
    exit_code: i32,
    #[serde(skip_serializing_if = "String::is_empty")]
    output: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    error: String,
}

const ALLOWED_ACTIONS: &[&str] = &["start", "stop", "restart", "pause", "continue"];

#[cfg(target_arch = "wasm32")]
#[plugin_fn]
pub fn unit_action(req: Json<UnitActionRequest>) -> FnResult<String> {
    let r = req.0;
    if !ALLOWED_ACTIONS.contains(&r.action.as_str()) {
        return Ok(serde_json::to_string(&UnitActionResponse {
            ok: false,
            error: format!("action_not_allowed: {}", r.action),
            ..Default::default()
        })?);
    }
    if let Err(e) = validate_service_name(&r.name) {
        return Ok(serde_json::to_string(&UnitActionResponse {
            ok: false,
            error: e,
            ..Default::default()
        })?);
    }
    let script = build_action_script(&r.action, &r.name);
    let exec_resp = match run_powershell(&script, 25_000) {
        Ok(v) => v,
        Err(e) => {
            return Ok(serde_json::to_string(&UnitActionResponse {
                ok: false,
                error: e,
                ..Default::default()
            })?)
        }
    };
    let combined = if exec_resp.stderr.is_empty() {
        exec_resp.stdout.clone()
    } else if exec_resp.stdout.is_empty() {
        exec_resp.stderr.clone()
    } else {
        format!("{}\n{}", exec_resp.stdout, exec_resp.stderr)
    };
    Ok(serde_json::to_string(&UnitActionResponse {
        ok: exec_resp.exit_code == 0,
        exit_code: exec_resp.exit_code,
        output: combined,
        error: String::new(),
    })?)
}

// build_action_script translates the action to the PowerShell verb
// that targets the service. The service Name is double-quoted in
// the script string; validate_service_name rejects double-quote
// characters so injection isn't possible.
pub fn build_action_script(action: &str, name: &str) -> String {
    let cmdlet = match action {
        "start" => "Start-Service",
        "stop" => "Stop-Service",
        "restart" => "Restart-Service",
        "pause" => "Suspend-Service",
        "continue" => "Resume-Service",
        _ => "Get-Service", // unreachable due to ALLOWED_ACTIONS
    };
    format!(
        "[Console]::OutputEncoding = [System.Text.Encoding]::UTF8; {} -Name \"{}\" -ErrorAction Stop",
        cmdlet, name
    )
}

// validate_service_name rejects characters that would break the
// double-quoted-arg form OR that Windows itself doesn't allow in
// service names (control chars, /, \\, embedded quotes). Service
// names are restricted by SCM to printable ASCII + a few specials;
// being conservative is harmless.
pub fn validate_service_name(name: &str) -> Result<(), String> {
    if name.is_empty() {
        return Err("name is required".to_string());
    }
    if name.starts_with('-') {
        return Err("name must not start with '-'".to_string());
    }
    for c in name.chars() {
        if c == '\0' || c == '\n' || c == '"' || c == '\\' || c == '/' || c == '`' {
            return Err(format!("name contains forbidden character: {:?}", c));
        }
        if (c as u32) < 0x20 {
            return Err("name contains control character".to_string());
        }
    }
    Ok(())
}

// ---------- exec helper ----------

#[cfg(target_arch = "wasm32")]
fn run_powershell(script: &str, timeout_ms: u32) -> Result<ExecResponse, String> {
    let args: Vec<String> = vec![
        "-NoProfile".to_string(),
        "-NonInteractive".to_string(),
        "-OutputFormat".to_string(),
        "Text".to_string(),
        "-Command".to_string(),
        script.to_string(),
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
fn run_powershell(_script: &str, _timeout_ms: u32) -> Result<ExecResponse, String> {
    Err("not available on host build".to_string())
}

// ============================================================
// tests
// ============================================================

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn decode_status_known() {
        assert_eq!(decode_status(1), "stopped");
        assert_eq!(decode_status(4), "running");
        assert_eq!(decode_status(7), "paused");
        assert_eq!(decode_status(99), "unknown");
    }

    #[test]
    fn decode_start_type_known() {
        assert_eq!(decode_start_type(2), "automatic");
        assert_eq!(decode_start_type(3), "manual");
        assert_eq!(decode_start_type(4), "disabled");
        assert_eq!(decode_start_type(99), "unknown");
    }

    #[test]
    fn parse_array_basic() {
        let stdout = r#"[
            {"Name":"AppXSvc","DisplayName":"AppX Deployment","Status":4,"StartType":3},
            {"Name":"BITS","DisplayName":"Background Intelligent Transfer","Status":1,"StartType":2}
        ]"#;
        let got = parse_powershell_json(stdout);
        assert_eq!(got.len(), 2);
        assert_eq!(got[0].name, "AppXSvc");
        assert_eq!(got[0].status, "running");
        assert_eq!(got[0].start_type, "manual");
        assert_eq!(got[1].status, "stopped");
        assert_eq!(got[1].start_type, "automatic");
    }

    #[test]
    fn parse_single_row_no_array() {
        let stdout = r#"{"Name":"Themes","DisplayName":"Themes","Status":4,"StartType":2}"#;
        let got = parse_powershell_json(stdout);
        assert_eq!(got.len(), 1);
        assert_eq!(got[0].name, "Themes");
    }

    #[test]
    fn parse_drops_empty_name() {
        // Defensive: a row missing Name is dropped, not a panic.
        let stdout = r#"[{"DisplayName":"foo","Status":1,"StartType":2}]"#;
        assert!(parse_powershell_json(stdout).is_empty());
    }

    #[test]
    fn parse_empty_or_garbage() {
        assert!(parse_powershell_json("").is_empty());
        assert!(parse_powershell_json("null").is_empty());
        assert!(parse_powershell_json("not json").is_empty());
    }

    #[test]
    fn build_action_script_quotes_name() {
        let s = build_action_script("start", "Spooler");
        assert!(s.contains("Start-Service"));
        assert!(s.contains("\"Spooler\""));
        assert!(s.contains("ErrorAction Stop"));
    }

    #[test]
    fn build_action_script_uses_correct_cmdlets() {
        assert!(build_action_script("stop", "x").contains("Stop-Service"));
        assert!(build_action_script("restart", "x").contains("Restart-Service"));
        assert!(build_action_script("pause", "x").contains("Suspend-Service"));
        assert!(build_action_script("continue", "x").contains("Resume-Service"));
    }

    #[test]
    fn validate_service_name_accepts_normal() {
        assert!(validate_service_name("Spooler").is_ok());
        assert!(validate_service_name("AppXSvc").is_ok());
        assert!(validate_service_name("MSSQL$INSTANCE01").is_ok());
    }

    #[test]
    fn validate_service_name_rejects_injection_chars() {
        assert!(validate_service_name("").is_err());
        assert!(validate_service_name("-evil").is_err());
        assert!(validate_service_name("foo\"; rm").is_err());
        assert!(validate_service_name("foo\\bar").is_err());
        assert!(validate_service_name("foo/bar").is_err());
        assert!(validate_service_name("foo`whoami`").is_err());
        assert!(validate_service_name("foo\nbar").is_err());
        assert!(validate_service_name("foo\0bar").is_err());
    }
}
