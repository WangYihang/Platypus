// sys-pkg-windows — Get-Package (universal) + winget (upgradable).
//
// Get-Package is part of the PackageManagement PowerShell module
// that ships with every Windows 8.1+ install. It enumerates every
// installed package the WMI/MSI subsystems know about — slow first
// invocation (5-10 s while WMI initializes) but reliable.
//
// winget covers the upgradable side. It's not on every host (App
// Installer on Win 10 must be installed from the Store; Win 11
// bundles it). When winget is missing the upgradable response
// carries `error="winget_not_installed"` rather than failing —
// the operator UI shows "winget not installed on this host" as
// a per-row disposition.

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
struct ListInstalledRequest {
    #[serde(default)]
    query: String,
    #[serde(default)]
    max_results: u32,
}

#[derive(Deserialize, Default)]
struct ListUpgradableRequest {}

#[derive(Serialize, Default)]
pub struct Package {
    pub name: String,
    pub version: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub arch: String,
}

#[derive(Serialize, Default)]
pub struct Update {
    pub name: String,
    #[serde(rename = "currentVersion", skip_serializing_if = "String::is_empty")]
    pub current_version: String,
    #[serde(rename = "availableVersion")]
    pub available_version: String,
}

#[derive(Serialize, Default)]
struct ListInstalledResponse {
    packages: Vec<Package>,
    backend: String,
    #[serde(skip_serializing_if = "is_zero_u32")]
    truncated_at: u32,
    #[serde(skip_serializing_if = "String::is_empty")]
    error: String,
}

#[derive(Serialize, Default)]
struct ListUpgradableResponse {
    updates: Vec<Update>,
    backend: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    error: String,
}

fn is_zero_u32(n: &u32) -> bool {
    *n == 0
}

const DEFAULT_MAX_RESULTS: u32 = 5_000;

const LIST_INSTALLED_PS: &str = "[Console]::OutputEncoding = [System.Text.Encoding]::UTF8; \
    Get-Package | Select-Object Name,Version,@{Name='Source';Expression={$_.ProviderName}} | \
    ConvertTo-Json -Compress -Depth 2";

// `winget list --upgrade-available` listing format is text-only
// (the cmdlet's `--output json` was added late and isn't on every
// build). We invoke through cmd.exe so winget is found even when
// the operator's PATH is sparse.
const LIST_UPGRADABLE_PS: &str = "[Console]::OutputEncoding = [System.Text.Encoding]::UTF8; \
    if (Get-Command winget -ErrorAction SilentlyContinue) { \
        winget upgrade --include-unknown --disable-interactivity 2>&1 \
    } else { \
        Write-Output 'WINGET_NOT_INSTALLED' \
    }";

#[cfg(target_arch = "wasm32")]
#[plugin_fn]
pub fn list_installed(req: Json<ListInstalledRequest>) -> FnResult<String> {
    let r = req.0;
    let exec_resp = match run_powershell(LIST_INSTALLED_PS, 80_000) {
        Ok(v) => v,
        Err(e) => {
            return Ok(serde_json::to_string(&ListInstalledResponse {
                packages: Vec::new(),
                backend: "get-package".to_string(),
                truncated_at: 0,
                error: e,
            })?)
        }
    };
    if exec_resp.exit_code != 0 {
        return Ok(serde_json::to_string(&ListInstalledResponse {
            packages: Vec::new(),
            backend: "get-package".to_string(),
            truncated_at: 0,
            error: format!(
                "powershell exit {}: {}",
                exec_resp.exit_code,
                exec_resp.stderr.trim()
            ),
        })?);
    }
    let mut packages = parse_get_package_json(&exec_resp.stdout);
    if !r.query.is_empty() {
        packages.retain(|p| p.name.contains(&r.query));
    }
    let cap = if r.max_results == 0 {
        DEFAULT_MAX_RESULTS
    } else {
        r.max_results
    };
    let mut truncated_at = 0;
    if packages.len() > cap as usize {
        truncated_at = packages.len() as u32;
        packages.truncate(cap as usize);
    }
    Ok(serde_json::to_string(&ListInstalledResponse {
        packages,
        backend: "get-package".to_string(),
        truncated_at,
        error: String::new(),
    })?)
}

#[cfg(target_arch = "wasm32")]
#[plugin_fn]
pub fn list_upgradable(_: Json<ListUpgradableRequest>) -> FnResult<String> {
    let exec_resp = match run_powershell(LIST_UPGRADABLE_PS, 80_000) {
        Ok(v) => v,
        Err(e) => {
            return Ok(serde_json::to_string(&ListUpgradableResponse {
                updates: Vec::new(),
                backend: "winget".to_string(),
                error: e,
            })?)
        }
    };
    if exec_resp.stdout.contains("WINGET_NOT_INSTALLED") {
        return Ok(serde_json::to_string(&ListUpgradableResponse {
            updates: Vec::new(),
            backend: String::new(),
            error: "winget_not_installed".to_string(),
        })?);
    }
    let updates = parse_winget_upgrade_table(&exec_resp.stdout);
    Ok(serde_json::to_string(&ListUpgradableResponse {
        updates,
        backend: "winget".to_string(),
        error: String::new(),
    })?)
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

// ---------- pure parsers ----------

// parse_get_package_json handles array / single-object / empty
// shapes (same as the other windows plugins). Each row's fields
// from the PS expression: Name, Version, Source (the provider —
// e.g. "msi", "Programs", "Chocolatey").
pub fn parse_get_package_json(stdout: &str) -> Vec<Package> {
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
    rows.into_iter().filter_map(extract_package).collect()
}

fn extract_package(row: &serde_json::Value) -> Option<Package> {
    let obj = row.as_object()?;
    let name = obj
        .get("Name")
        .and_then(|x| x.as_str())
        .unwrap_or_default()
        .to_string();
    if name.is_empty() {
        return None;
    }
    let version = obj
        .get("Version")
        .and_then(|x| x.as_str())
        .unwrap_or_default()
        .to_string();
    // Source / ProviderName goes into `arch` to keep the wire shape
    // uniform with linux ("x86_64", "amd64") and darwin (channel
    // suffix). The operator UI knows it's a free-form qualifier.
    let source = obj
        .get("Source")
        .and_then(|x| x.as_str())
        .unwrap_or_default()
        .to_string();
    Some(Package {
        name,
        version,
        arch: source,
    })
}

// parse_winget_upgrade_table reads the human-formatted output of
// `winget upgrade`:
//
//   Name                Id                  Version  Available  Source
//   ------------------- ------------------- -------- ---------- ------
//   Visual Studio Code  Microsoft.VSCode    1.92.0   1.93.1     winget
//   Mozilla Firefox     Mozilla.Firefox     130.0    130.0.1    winget
//
// Variable column widths. Strategy: find the header row's column
// positions (the dashes line is a useful fallback), then slice each
// data row at those positions. Defensive — winget's column padding
// has shifted between v1.5 and v1.7.
pub fn parse_winget_upgrade_table(stdout: &str) -> Vec<Update> {
    let mut lines = stdout.lines().filter(|l| !l.trim().is_empty());
    let header = match lines.next() {
        Some(l) => l,
        None => return Vec::new(),
    };
    // Column positions = column-name start indexes from the header.
    let positions = column_starts(header, &["Name", "Id", "Version", "Available", "Source"]);
    if positions.len() != 5 {
        return Vec::new();
    }
    let mut out = Vec::new();
    for line in lines {
        // Skip the dashes separator and any non-data lines (winget
        // prints a "X upgrades available" footer, "Some packages..."
        // header, or progress indicators).
        if line.starts_with('-') {
            continue;
        }
        if line.len() < positions[positions.len() - 1] {
            continue;
        }
        let name = slice_col(line, positions[0], positions[1]);
        // skip Id (positions[1] → positions[2])
        let current = slice_col(line, positions[2], positions[3]);
        let available = slice_col(line, positions[3], positions[4]);
        if name.is_empty() || available.is_empty() {
            continue;
        }
        out.push(Update {
            name,
            current_version: current,
            available_version: available,
        });
    }
    out
}

fn column_starts(header: &str, names: &[&str]) -> Vec<usize> {
    let mut out = Vec::with_capacity(names.len());
    for n in names {
        match header.find(n) {
            Some(i) => out.push(i),
            None => return Vec::new(),
        }
    }
    out
}

fn slice_col(line: &str, start: usize, end: usize) -> String {
    let chars: Vec<char> = line.chars().collect();
    let s = start.min(chars.len());
    let e = end.min(chars.len());
    if s >= e {
        return String::new();
    }
    chars[s..e].iter().collect::<String>().trim().to_string()
}

// ============================================================
// tests
// ============================================================

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parse_get_package_array() {
        let stdout = r#"[
            {"Name":"Visual Studio Code","Version":"1.93.1","Source":"Programs"},
            {"Name":"7-Zip 24.07","Version":"24.07.00.0","Source":"msi"}
        ]"#;
        let got = parse_get_package_json(stdout);
        assert_eq!(got.len(), 2);
        assert_eq!(got[0].name, "Visual Studio Code");
        assert_eq!(got[0].version, "1.93.1");
        assert_eq!(got[0].arch, "Programs");
        assert_eq!(got[1].arch, "msi");
    }

    #[test]
    fn parse_get_package_single_object() {
        let stdout = r#"{"Name":"PowerShell-7","Version":"7.4.1","Source":"msi"}"#;
        let got = parse_get_package_json(stdout);
        assert_eq!(got.len(), 1);
        assert_eq!(got[0].name, "PowerShell-7");
    }

    #[test]
    fn parse_get_package_drops_empty_name() {
        let stdout = r#"[{"Version":"1.0","Source":"msi"}]"#;
        assert!(parse_get_package_json(stdout).is_empty());
    }

    #[test]
    fn parse_get_package_garbage_or_empty() {
        assert!(parse_get_package_json("").is_empty());
        assert!(parse_get_package_json("null").is_empty());
        assert!(parse_get_package_json("not json").is_empty());
    }

    #[test]
    fn parse_winget_upgrade_basic() {
        let stdout = "\
Name                Id                  Version  Available  Source
------------------- ------------------- -------- ---------- ------
Visual Studio Code  Microsoft.VSCode    1.92.0   1.93.1     winget
Mozilla Firefox     Mozilla.Firefox     130.0    130.0.1    winget
2 upgrades available.
";
        let got = parse_winget_upgrade_table(stdout);
        assert_eq!(got.len(), 2);
        assert_eq!(got[0].name, "Visual Studio Code");
        assert_eq!(got[0].current_version, "1.92.0");
        assert_eq!(got[0].available_version, "1.93.1");
    }

    #[test]
    fn parse_winget_upgrade_empty() {
        let stdout = "\
Name  Id  Version  Available  Source
----  --  -------  ---------  ------
";
        assert!(parse_winget_upgrade_table(stdout).is_empty());
    }

    #[test]
    fn parse_winget_upgrade_returns_empty_on_no_header() {
        let stdout = "junk output\n";
        assert!(parse_winget_upgrade_table(stdout).is_empty());
    }

    #[test]
    fn slice_col_handles_short_lines() {
        assert_eq!(slice_col("abc", 0, 100), "abc");
        assert_eq!(slice_col("abc", 5, 10), "");
    }
}
