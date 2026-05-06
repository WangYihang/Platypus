// sys-services-darwin — macOS service control via /bin/launchctl.
//
// `launchctl list` output (tab-separated, header line first):
//
//   PID\tStatus\tLabel
//   123\t0\tcom.apple.WindowServer
//   -\t0\tcom.apple.SafariBookmarksSyncAgent
//   -\t1\tcom.example.failed-job
//
// PID is "-" for inactive jobs. Status is the last exit code; 0 =
// successful, non-zero = error. Label is the launchd job id
// (reverse-DNS like style).
//
// Action subcommands map to the legacy `launchctl <verb> <label>`
// form (Big Sur kept it working alongside the bootstrap/bootout
// path). For "enable" / "disable", launchctl needs the
// "system/<label>" domain target.

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

// ---------- list_units ----------

#[derive(Deserialize, Default)]
struct ListUnitsRequest {
    /// Optional substring filter applied to label. Empty = all.
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
    pub label: String,
    /// Unix process id (0 if inactive — launchctl reports "-").
    pub pid: u32,
    /// Last exit code (0 = ok, non-zero = error).
    pub status: i32,
    /// "active" if pid > 0, "inactive" otherwise. Mirrors the
    /// systemd-style boolean state surface.
    pub active: String,
}

#[cfg(target_arch = "wasm32")]
#[plugin_fn]
pub fn list_units(req: Json<ListUnitsRequest>) -> FnResult<String> {
    let r = req.0;
    let exec_resp = match run_launchctl(vec!["list".to_string()], 10_000) {
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
                "launchctl exit {}: {}",
                exec_resp.exit_code,
                exec_resp.stderr.trim()
            ),
        })?);
    }
    let mut units = parse_launchctl_list(&exec_resp.stdout);
    if !r.filter.is_empty() {
        units.retain(|u| u.label.contains(&r.filter));
    }
    Ok(serde_json::to_string(&ListUnitsResponse {
        units,
        error: String::new(),
    })?)
}

// parse_launchctl_list parses tab-separated launchctl list output.
// First line is the header (PID Status Label); skip it. Each data
// line has exactly 3 fields: PID, Status, Label.  PID is "-" for
// inactive jobs (encoded as 0 in the wire shape).  Malformed rows
// are dropped silently.
pub fn parse_launchctl_list(stdout: &str) -> Vec<UnitListEntry> {
    let mut out = Vec::new();
    let mut lines = stdout.lines();
    // Skip header.
    match lines.next() {
        Some(h) if h.trim_start().starts_with("PID") => {}
        _ => return out,
    }
    for line in lines {
        let trimmed = line.trim_end();
        if trimmed.is_empty() {
            continue;
        }
        // Try tab split first (the documented format), fall back to
        // any-whitespace split on systems where launchctl emits
        // space-padded columns instead.
        let cols: Vec<&str> = if trimmed.contains('\t') {
            trimmed.split('\t').collect()
        } else {
            trimmed.split_whitespace().collect()
        };
        if cols.len() < 3 {
            continue;
        }
        let pid = if cols[0] == "-" {
            0
        } else {
            cols[0].parse::<u32>().unwrap_or(0)
        };
        let status: i32 = cols[1].parse().unwrap_or(0);
        let label = cols[2].to_string();
        let active = if pid > 0 { "active" } else { "inactive" };
        out.push(UnitListEntry {
            label,
            pid,
            status,
            active: active.to_string(),
        });
    }
    out
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

const ALLOWED_ACTIONS: &[&str] = &[
    "start",
    "stop",
    "kickstart",
    "enable",
    "disable",
    "load",
    "unload",
];

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
    if let Err(e) = validate_label(&r.name) {
        return Ok(serde_json::to_string(&UnitActionResponse {
            ok: false,
            error: e,
            ..Default::default()
        })?);
    }
    let args = build_action_args(&r.action, &r.name);
    let exec_resp = match run_launchctl(args, 10_000) {
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

// build_action_args translates the request action into the modern
// launchctl invocation form. enable / disable / kickstart need a
// "system/<label>" domain target; start / stop / load / unload
// take the bare label.
pub fn build_action_args(action: &str, label: &str) -> Vec<String> {
    let domain_target = format!("system/{}", label);
    match action {
        "kickstart" => vec![
            "kickstart".to_string(),
            "-k".to_string(), // restart if already running
            domain_target,
        ],
        "enable" | "disable" => vec![action.to_string(), domain_target],
        // start / stop / load / unload use the legacy form.
        _ => vec![action.to_string(), label.to_string()],
    }
}

// validate_label rejects shell-injection foot-guns even though
// host_exec args bypass the shell. Specifically:
//   - Empty label
//   - Leading "-"  (launchctl would treat as a flag)
//   - \0 / \n     (would corrupt audit log lines)
pub fn validate_label(label: &str) -> Result<(), String> {
    if label.is_empty() {
        return Err("name is required".to_string());
    }
    if label.starts_with('-') {
        return Err("name must not start with '-'".to_string());
    }
    if label.contains('\0') || label.contains('\n') {
        return Err("name contains forbidden characters".to_string());
    }
    Ok(())
}

// ---------- exec helper ----------

#[cfg(target_arch = "wasm32")]
fn run_launchctl(args: Vec<String>, timeout_ms: u32) -> Result<ExecResponse, String> {
    let req = ExecRequest {
        command: "/bin/launchctl".to_string(),
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
fn run_launchctl(_args: Vec<String>, _timeout_ms: u32) -> Result<ExecResponse, String> {
    Err("not available on host build".to_string())
}

// ============================================================
// tests (host-build only — pure parsers + helpers)
// ============================================================

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parse_basic_three_rows() {
        let stdout = "PID\tStatus\tLabel\n\
123\t0\tcom.apple.WindowServer\n\
-\t0\tcom.apple.SafariBookmarksSyncAgent\n\
-\t1\tcom.example.failed-job\n";
        let got = parse_launchctl_list(stdout);
        assert_eq!(got.len(), 3);
        assert_eq!(got[0].label, "com.apple.WindowServer");
        assert_eq!(got[0].pid, 123);
        assert_eq!(got[0].active, "active");
        assert_eq!(got[1].pid, 0);
        assert_eq!(got[1].active, "inactive");
        assert_eq!(got[2].status, 1);
    }

    #[test]
    fn parse_handles_whitespace_separated_format() {
        // Some launchctl variants emit space-padded rather than tab.
        let stdout = "PID  Status  Label\n\
99   0       com.example.alpha\n\
-    0       com.example.beta\n";
        let got = parse_launchctl_list(stdout);
        assert_eq!(got.len(), 2);
        assert_eq!(got[0].pid, 99);
        assert_eq!(got[1].label, "com.example.beta");
    }

    #[test]
    fn parse_skips_short_rows() {
        let stdout = "PID\tStatus\tLabel\nincomplete\n";
        assert!(parse_launchctl_list(stdout).is_empty());
    }

    #[test]
    fn parse_returns_empty_on_missing_header() {
        let stdout = "123\t0\tcom.apple.WindowServer\n";
        assert!(parse_launchctl_list(stdout).is_empty());
    }

    #[test]
    fn parse_empty_input() {
        assert!(parse_launchctl_list("").is_empty());
    }

    #[test]
    fn build_action_args_kickstart_uses_domain_form() {
        let got = build_action_args("kickstart", "com.example.x");
        assert_eq!(got, vec!["kickstart", "-k", "system/com.example.x"]);
    }

    #[test]
    fn build_action_args_enable_uses_domain_form() {
        let got = build_action_args("enable", "com.example.y");
        assert_eq!(got, vec!["enable", "system/com.example.y"]);
    }

    #[test]
    fn build_action_args_start_uses_legacy_form() {
        let got = build_action_args("start", "com.example.z");
        assert_eq!(got, vec!["start", "com.example.z"]);
    }

    #[test]
    fn validate_label_accepts_reverse_dns() {
        assert!(validate_label("com.apple.WindowServer").is_ok());
    }

    #[test]
    fn validate_label_rejects_dash_prefix_and_nul() {
        assert!(validate_label("-evil").is_err());
        assert!(validate_label("").is_err());
        assert!(validate_label("ok\0bad").is_err());
        assert!(validate_label("ok\nbad").is_err());
    }
}
