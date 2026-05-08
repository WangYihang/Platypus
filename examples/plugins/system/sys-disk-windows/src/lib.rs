// sys-disk-windows — Get-PSDrive-FileSystem via PowerShell.
//
// PS one-liner emits JSON per drive:
//   { Name: "C", Used: 53687091200, Free: 184372224000, Root: "C:\\" }
//
// Get-PSDrive is the canonical cross-PowerShell-version interface
// (works on Windows PowerShell 5 + 7). WMI's Win32_LogicalDisk would
// give us fstype but is slow; the operator UI doesn't gain enough
// from "ntfs" vs "refs" labelling to justify the latency.
//
// Encoding: same UTF-8 force as sys-procs-windows. Without it,
// PowerShell 5 writes UTF-16-LE which serde_json can't parse.

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
pub struct ListRequest {
    #[serde(default = "default_true")]
    skip_pseudo: bool,
    #[serde(default)]
    only_mountpoints: Vec<String>,
    #[serde(default)]
    min_size_bytes: u64,
}

fn default_true() -> bool {
    true
}

#[derive(Serialize, Default)]
struct ListResponse {
    filesystems: Vec<Filesystem>,
    #[serde(skip_serializing_if = "String::is_empty")]
    error: String,
}

#[derive(Serialize, Default, Debug, PartialEq)]
pub struct Filesystem {
    pub source: String,
    pub fstype: String,
    pub mountpoint: String,
    #[serde(rename = "sizeBytes")]
    pub size_bytes: u64,
    #[serde(rename = "usedBytes")]
    pub used_bytes: u64,
    #[serde(rename = "availableBytes")]
    pub available_bytes: u64,
    #[serde(rename = "percentUsed")]
    pub percent_used: u8,
}

const PS_SCRIPT: &str = "[Console]::OutputEncoding = [System.Text.Encoding]::UTF8; \
    Get-PSDrive -PSProvider FileSystem | \
    Select-Object Name,Used,Free,Root | \
    ConvertTo-Json -Compress -Depth 2";

#[cfg(target_arch = "wasm32")]
#[plugin_fn]
pub fn list_filesystems(req: Json<ListRequest>) -> FnResult<String> {
    let r = req.0;

    let exec_resp = match run_powershell(12_000) {
        Ok(v) => v,
        Err(e) => {
            return Ok(serde_json::to_string(&ListResponse {
                filesystems: Vec::new(),
                error: e,
            })?)
        }
    };
    if exec_resp.exit_code != 0 {
        return Ok(serde_json::to_string(&ListResponse {
            filesystems: Vec::new(),
            error: format!(
                "powershell exit {}: {}",
                exec_resp.exit_code,
                exec_resp.stderr.trim()
            ),
        })?);
    }
    let parsed = parse_powershell_json(&exec_resp.stdout);
    let filtered = apply_filters(parsed, &r);
    Ok(serde_json::to_string(&ListResponse {
        filesystems: filtered,
        error: String::new(),
    })?)
}

// parse_powershell_json handles three pipeline shapes:
//   - "" / "null"          → empty list
//   - bare object {…}      → wrap as 1-element list (PS elides the
//                            array on single-row pipelines)
//   - array [{…},{…}]      → standard parse
//
// Each row's PowerShell-emitted PascalCase keys: Name, Used, Free,
// Root. Used/Free may arrive as JSON numbers OR strings depending
// on PowerShell version (5 stringifies large integers).
pub fn parse_powershell_json(stdout: &str) -> Vec<Filesystem> {
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

fn extract_row(row: &serde_json::Value) -> Option<Filesystem> {
    let obj = row.as_object()?;
    let name = obj.get("Name").and_then(|x| x.as_str()).unwrap_or_default();
    let root = obj.get("Root").and_then(|x| x.as_str()).unwrap_or_default();
    let used = pick_u64(obj, "Used");
    let free = pick_u64(obj, "Free");
    let size = used.saturating_add(free);
    let percent_used = if size > 0 {
        let raw = (used as u128 * 100 / size as u128) as u32;
        if raw > 100 {
            100
        } else {
            raw as u8
        }
    } else {
        0
    };
    let mountpoint = if root.is_empty() {
        format!("{}:", name)
    } else {
        root.to_string()
    };
    Some(Filesystem {
        source: format!("{}:", name),
        fstype: "ntfs".to_string(),
        mountpoint,
        size_bytes: size,
        used_bytes: used,
        available_bytes: free,
        percent_used,
    })
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

pub fn apply_filters(input: Vec<Filesystem>, req: &ListRequest) -> Vec<Filesystem> {
    input
        .into_iter()
        .filter(|fs| {
            if req.skip_pseudo && fs.size_bytes == 0 {
                // Disconnected mapped network drive, A:/B: with no
                // floppy, etc. Used + Free == 0 is the canonical
                // "this drive isn't actually usable right now" sign.
                return false;
            }
            if !req.only_mountpoints.is_empty()
                && !req.only_mountpoints.contains(&fs.mountpoint)
            {
                return false;
            }
            if fs.size_bytes < req.min_size_bytes {
                return false;
            }
            true
        })
        .collect()
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
    fn parse_array_two_drives() {
        let stdout = r#"[
            {"Name":"C","Used":53687091200,"Free":184372224000,"Root":"C:\\"},
            {"Name":"D","Used":1073741824,"Free":107374182400,"Root":"D:\\"}
        ]"#;
        let got = parse_powershell_json(stdout);
        assert_eq!(got.len(), 2);
        assert_eq!(got[0].source, "C:");
        assert_eq!(got[0].mountpoint, "C:\\");
        assert_eq!(got[0].used_bytes, 53_687_091_200);
        assert_eq!(got[0].available_bytes, 184_372_224_000);
        assert_eq!(got[0].size_bytes, 53_687_091_200 + 184_372_224_000);
        // 53GB used / ~238GB total = 22%
        assert!(got[0].percent_used >= 21 && got[0].percent_used <= 23);
        assert_eq!(got[0].fstype, "ntfs");
        assert_eq!(got[1].source, "D:");
    }

    #[test]
    fn parse_single_drive_no_array_wrapper() {
        // PS ConvertTo-Json elides the array on single-row output.
        let stdout = r#"{"Name":"C","Used":1,"Free":1,"Root":"C:\\"}"#;
        let got = parse_powershell_json(stdout);
        assert_eq!(got.len(), 1);
        assert_eq!(got[0].source, "C:");
    }

    #[test]
    fn parse_handles_zero_size_drive() {
        // Disconnected mapped drive: Used + Free == 0. Parser
        // creates the row; apply_filters drops it when skip_pseudo.
        let stdout = r#"[{"Name":"Z","Used":0,"Free":0,"Root":"\\\\server\\share\\"}]"#;
        let got = parse_powershell_json(stdout);
        assert_eq!(got.len(), 1);
        assert_eq!(got[0].percent_used, 0);
        assert_eq!(got[0].size_bytes, 0);
    }

    #[test]
    fn parse_stringified_numerics() {
        let stdout = r#"[{"Name":"C","Used":"100","Free":"100","Root":"C:\\"}]"#;
        let got = parse_powershell_json(stdout);
        assert_eq!(got.len(), 1);
        assert_eq!(got[0].size_bytes, 200);
        assert_eq!(got[0].used_bytes, 100);
        assert_eq!(got[0].percent_used, 50);
    }

    #[test]
    fn parse_empty_or_null() {
        assert!(parse_powershell_json("").is_empty());
        assert!(parse_powershell_json("null").is_empty());
        assert!(parse_powershell_json("not json").is_empty());
    }

    #[test]
    fn apply_filters_skip_pseudo_drops_zero_size() {
        let input = vec![
            Filesystem {
                source: "C:".into(),
                mountpoint: "C:\\".into(),
                size_bytes: 1_000_000,
                used_bytes: 500_000,
                available_bytes: 500_000,
                ..Default::default()
            },
            Filesystem {
                source: "Z:".into(),
                mountpoint: "Z:\\".into(),
                size_bytes: 0,
                ..Default::default()
            },
        ];
        let req = ListRequest {
            skip_pseudo: true,
            ..Default::default()
        };
        let out = apply_filters(input, &req);
        assert_eq!(out.len(), 1);
        assert_eq!(out[0].source, "C:");
    }

    #[test]
    fn apply_filters_only_mountpoints() {
        let input = vec![
            Filesystem {
                mountpoint: "C:\\".into(),
                size_bytes: 1,
                ..Default::default()
            },
            Filesystem {
                mountpoint: "D:\\".into(),
                size_bytes: 1,
                ..Default::default()
            },
        ];
        let req = ListRequest {
            skip_pseudo: false,
            only_mountpoints: vec!["D:\\".into()],
            ..Default::default()
        };
        let out = apply_filters(input, &req);
        assert_eq!(out.len(), 1);
        assert_eq!(out[0].mountpoint, "D:\\");
    }

    #[test]
    fn percent_used_clamped_to_100() {
        // Pathological case: Used > Used+Free (PowerShell rounding).
        // Should clamp at 100, not overflow.
        let stdout = r#"{"Name":"C","Used":999999999999,"Free":1,"Root":"C:\\"}"#;
        let got = parse_powershell_json(stdout);
        assert!(got[0].percent_used <= 100);
    }
}
