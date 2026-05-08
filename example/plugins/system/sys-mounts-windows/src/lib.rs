// sys-mounts-windows — Windows volume inventory via Get-Volume.
//
// PowerShell pipeline:
//   Get-Volume | Select-Object DriveLetter, FileSystemLabel,
//   FileSystem, DriveType, OperationalStatus, Path,
//   @{Name='ReadOnly';Expression={[bool]$_.ReadOnly}}
//   | ConvertTo-Json -Compress -Depth 3
//
// We map each volume into the cross-OS Mount shape:
//   source     ← Path or "<DriveLetter>:\"
//   mountpoint ← "<DriveLetter>:\" (if a drive letter is assigned)
//                 — empty when the volume is unmounted
//   fstype     ← FileSystem (NTFS / FAT32 / exFAT / ReFS / "")
//   read_only  ← ReadOnly bool
//   pseudo     ← DriveType in {Removable, CDRom, Network, ...} we
//                 leave false; the only "pseudo" volumes on Windows
//                 are System Reserved partitions which Get-Volume
//                 reports as DriveType=Fixed already.

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

// ---- request / response wire shapes ----

#[derive(Deserialize, Default)]
struct ListRequest {
    #[serde(default)]
    include_pseudo: bool,
    #[serde(default)]
    include_active_fstab: bool,
}

#[derive(Serialize, Default)]
struct ListResponse {
    mounts: Vec<Mount>,
    fstab: Vec<()>,
    #[serde(skip_serializing_if = "String::is_empty")]
    error: String,
}

#[derive(Serialize, Default, Debug, PartialEq)]
struct Mount {
    source: String,
    mountpoint: String,
    fstype: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    options: String,
    #[serde(rename = "readOnly", skip_serializing_if = "is_false")]
    read_only: bool,
    #[serde(skip_serializing_if = "is_false")]
    nosuid: bool,
    #[serde(skip_serializing_if = "is_false")]
    nodev: bool,
    #[serde(skip_serializing_if = "is_false")]
    noexec: bool,
    #[serde(rename = "fsId", skip_serializing_if = "String::is_empty")]
    fs_id: String,
    #[serde(skip_serializing_if = "is_false")]
    pseudo: bool,
}

fn is_false(b: &bool) -> bool { !*b }

// ---- entry point ----

const PS_SCRIPT: &str = r#"[Console]::OutputEncoding = [System.Text.Encoding]::UTF8;
$out = Get-Volume | Select-Object `
    @{Name='driveLetter'; Expression={if ($_.DriveLetter) { [string]$_.DriveLetter } else { '' }}}, `
    @{Name='label';        Expression={[string]$_.FileSystemLabel}}, `
    @{Name='fileSystem';   Expression={[string]$_.FileSystem}}, `
    @{Name='driveType';    Expression={[string]$_.DriveType}}, `
    @{Name='path';         Expression={[string]$_.Path}}, `
    @{Name='readOnly';     Expression={if ($_.PSObject.Properties.Match('ReadOnly').Count -gt 0) { [bool]$_.ReadOnly } else { $false }}}
$out | ConvertTo-Json -Compress -Depth 3"#;

#[derive(Deserialize, Default)]
struct PsVolume {
    #[serde(default, rename = "driveLetter")]
    drive_letter: String,
    #[serde(default)]
    label: String,
    #[serde(default, rename = "fileSystem")]
    file_system: String,
    #[serde(default, rename = "driveType")]
    drive_type: String,
    #[serde(default)]
    path: String,
    #[serde(default, rename = "readOnly")]
    read_only: bool,
}

#[cfg(target_arch = "wasm32")]
#[plugin_fn]
pub fn list_mounts(req: Json<ListRequest>) -> FnResult<String> {
    let r = req.0;
    let exec_resp = match run_powershell(PS_SCRIPT, 25_000) {
        Ok(v) => v,
        Err(e) => {
            return Ok(serde_json::to_string(&ListResponse {
                mounts: Vec::new(),
                fstab: Vec::new(),
                error: e,
            })?)
        }
    };
    if exec_resp.exit_code != 0 {
        return Ok(serde_json::to_string(&ListResponse {
            mounts: Vec::new(),
            fstab: Vec::new(),
            error: format!(
                "powershell exit {}: {}",
                exec_resp.exit_code,
                exec_resp.stderr.trim()
            ),
        })?);
    }
    let mut mounts = parse_powershell_output(&exec_resp.stdout);
    if !r.include_pseudo {
        mounts.retain(|m| !m.pseudo);
    }
    Ok(serde_json::to_string(&ListResponse {
        mounts,
        fstab: Vec::new(),
        error: String::new(),
    })?)
}

// ---- pure parser ----

fn parse_powershell_output(stdout: &str) -> Vec<Mount> {
    let trimmed = stdout.trim();
    if trimmed.is_empty() || trimmed == "null" {
        return Vec::new();
    }
    let raw: Vec<PsVolume> = if let Ok(v) = serde_json::from_str::<Vec<PsVolume>>(trimmed) {
        v
    } else if let Ok(v) = serde_json::from_str::<PsVolume>(trimmed) {
        vec![v]
    } else {
        return Vec::new();
    };
    raw.into_iter().map(volume_to_mount).collect()
}

fn volume_to_mount(v: PsVolume) -> Mount {
    let mountpoint = if !v.drive_letter.is_empty() {
        format!("{}:\\", v.drive_letter)
    } else {
        v.path.clone()
    };
    let source = if !v.path.is_empty() {
        v.path.clone()
    } else {
        mountpoint.clone()
    };
    Mount {
        source,
        mountpoint,
        fstype: v.file_system.clone(),
        options: v.drive_type.clone(),
        read_only: v.read_only,
        fs_id: v.label,
        // No notion of "pseudo" on Windows volumes the way Linux has
        // tmpfs/proc — every Get-Volume entry is a real volume. Leave
        // false unconditionally so include_pseudo doesn't filter
        // anything by accident.
        pseudo: false,
        ..Default::default()
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
        host_exec(body)
            .map_err(|e| format!("host_exec: {e}"))?
            .0
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
    fn parse_array_two_volumes() {
        let json = r#"[
            {"driveLetter":"C","label":"OS","fileSystem":"NTFS","driveType":"Fixed","path":"C:\\","readOnly":false},
            {"driveLetter":"D","label":"Data","fileSystem":"NTFS","driveType":"Fixed","path":"D:\\","readOnly":true}
        ]"#;
        let mounts = parse_powershell_output(json);
        assert_eq!(mounts.len(), 2);
        assert_eq!(mounts[0].mountpoint, "C:\\");
        assert_eq!(mounts[0].fstype, "NTFS");
        assert!(!mounts[0].read_only);
        assert_eq!(mounts[1].mountpoint, "D:\\");
        assert!(mounts[1].read_only);
        assert_eq!(mounts[1].fs_id, "Data");
    }

    #[test]
    fn parse_single_volume_object() {
        let json = r#"{"driveLetter":"C","label":"","fileSystem":"NTFS","driveType":"Fixed","path":"C:\\","readOnly":false}"#;
        let mounts = parse_powershell_output(json);
        assert_eq!(mounts.len(), 1);
        assert_eq!(mounts[0].mountpoint, "C:\\");
    }

    #[test]
    fn parse_volume_without_drive_letter_uses_path() {
        let json = r#"{"driveLetter":"","label":"System Reserved","fileSystem":"NTFS","driveType":"Fixed","path":"\\\\?\\Volume{abc}\\","readOnly":false}"#;
        let mounts = parse_powershell_output(json);
        assert_eq!(mounts.len(), 1);
        assert!(mounts[0].mountpoint.starts_with("\\\\?\\Volume"));
        assert_eq!(mounts[0].source, mounts[0].mountpoint);
    }

    #[test]
    fn parse_empty_output() {
        assert!(parse_powershell_output("").is_empty());
        assert!(parse_powershell_output("null").is_empty());
    }

    #[test]
    fn drive_type_passes_through_to_options() {
        let json = r#"[{"driveLetter":"E","fileSystem":"FAT32","driveType":"Removable","path":"E:\\","readOnly":false}]"#;
        let mounts = parse_powershell_output(json);
        assert_eq!(mounts[0].options, "Removable");
    }
}
