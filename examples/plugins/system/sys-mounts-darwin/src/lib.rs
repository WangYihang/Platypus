// sys-mounts-darwin — macOS mount inventory via `/sbin/mount`.
//
// `mount` output has one record per line:
//   /dev/disk1s5s1 on / (apfs, sealed, local, read-only, journaled)
//   map auto_home on /System/Volumes/Data/home (autofs, automounted, nobrowse)
//
// Format: "<source> on <mountpoint> (<comma-separated options>)"
// We pre-parse common option flags (ro / nosuid / nodev / noexec /
// read-only) so the typed-bridge UI shows the same booleans as the
// linux sibling.

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
    fstab: Vec<()>, // always empty on darwin; preserved for shape parity
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

const PSEUDO_FS: &[&str] = &[
    "devfs", "autofs", "tmpfs", "fdesc", "lifs", "ctlfs", "nullfs",
];

// ---- entry point ----

#[cfg(target_arch = "wasm32")]
#[plugin_fn]
pub fn list_mounts(req: Json<ListRequest>) -> FnResult<String> {
    let r = req.0;
    let exec_resp = match run_mount(15_000) {
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
                "mount exit {}: {}",
                exec_resp.exit_code,
                exec_resp.stderr.trim()
            ),
        })?);
    }
    let mut mounts = parse_mount_output(&exec_resp.stdout);
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

// parse_mount_output handles the BSD `mount` line format:
//   "<source> on <mountpoint> (<fstype>, opt1, opt2, ...)"
// The mountpoint may contain spaces ("/Volumes/My Drive") and the
// fstype is always the first token inside the parens.
fn parse_mount_output(stdout: &str) -> Vec<Mount> {
    let mut out = Vec::new();
    for line in stdout.lines() {
        let line = line.trim();
        if line.is_empty() {
            continue;
        }
        // Split on " on " (space-bracketed) to peel source.
        let on_idx = match line.find(" on ") {
            Some(i) => i,
            None => continue,
        };
        let source = line[..on_idx].trim().to_string();
        let rest = line[on_idx + 4..].trim();
        // Find the trailing "(<options>)".
        let paren_open = match rest.rfind('(') {
            Some(i) => i,
            None => continue,
        };
        let paren_close = match rest.rfind(')') {
            Some(i) => i,
            None => continue,
        };
        if paren_close <= paren_open {
            continue;
        }
        let mountpoint = rest[..paren_open].trim().to_string();
        let inside = &rest[paren_open + 1..paren_close];
        let parts: Vec<&str> = inside.split(',').map(|s| s.trim()).collect();
        if parts.is_empty() {
            continue;
        }
        let fstype = parts[0].to_string();
        let options_vec: Vec<&str> = parts.iter().skip(1).copied().collect();
        let options = options_vec.join(",");
        let mut m = Mount {
            source,
            mountpoint,
            fstype: fstype.clone(),
            options,
            ..Default::default()
        };
        for opt in &options_vec {
            match *opt {
                "read-only" | "ro" => m.read_only = true,
                "nosuid" => m.nosuid = true,
                "nodev" => m.nodev = true,
                "noexec" => m.noexec = true,
                _ => {}
            }
        }
        m.pseudo = is_pseudo_fs(&fstype);
        out.push(m);
    }
    out
}

fn is_pseudo_fs(fstype: &str) -> bool {
    PSEUDO_FS.iter().any(|p| *p == fstype)
}

// ---- exec helper ----

#[cfg(target_arch = "wasm32")]
fn run_mount(timeout_ms: u32) -> Result<ExecResponse, String> {
    let req = ExecRequest {
        command: "/sbin/mount".to_string(),
        args: Vec::new(),
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
fn run_mount(_timeout_ms: u32) -> Result<ExecResponse, String> {
    Err("not available on host build".to_string())
}

// ============================================================
// Pure-function unit tests
// ============================================================
#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parse_apfs_root() {
        let stdout = "/dev/disk1s5s1 on / (apfs, sealed, local, read-only, journaled)\n";
        let mounts = parse_mount_output(stdout);
        assert_eq!(mounts.len(), 1);
        assert_eq!(mounts[0].source, "/dev/disk1s5s1");
        assert_eq!(mounts[0].mountpoint, "/");
        assert_eq!(mounts[0].fstype, "apfs");
        assert!(mounts[0].read_only);
        assert!(mounts[0].options.contains("sealed"));
        assert!(!mounts[0].pseudo);
    }

    #[test]
    fn parse_devfs() {
        let stdout = "devfs on /dev (devfs, local, nobrowse)\n";
        let mounts = parse_mount_output(stdout);
        assert_eq!(mounts.len(), 1);
        assert!(mounts[0].pseudo);
        assert_eq!(mounts[0].fstype, "devfs");
    }

    #[test]
    fn parse_with_nosuid_nodev_noexec() {
        let stdout = "tmpfs on /private/tmp (tmpfs, local, noexec, nosuid, nodev)\n";
        let mounts = parse_mount_output(stdout);
        assert_eq!(mounts.len(), 1);
        assert!(mounts[0].nosuid);
        assert!(mounts[0].nodev);
        assert!(mounts[0].noexec);
    }

    #[test]
    fn parse_multiline_output() {
        let stdout = "\
/dev/disk1s5s1 on / (apfs, sealed, local, read-only, journaled)
devfs on /dev (devfs, local, nobrowse)
/dev/disk1s2 on /System/Volumes/Preboot (apfs, sealed, local, journaled, nobrowse)
";
        let mounts = parse_mount_output(stdout);
        assert_eq!(mounts.len(), 3);
    }

    #[test]
    fn parse_skips_malformed() {
        let stdout = "\
not a mount line
/dev/foo missing opens

/dev/sda1 on /target (ext4)
";
        let mounts = parse_mount_output(stdout);
        // "not a mount line" — no " on ", skipped.
        // "/dev/foo missing opens" — no parens, skipped.
        // The valid line at the bottom should be picked up.
        assert_eq!(mounts.len(), 1);
        assert_eq!(mounts[0].source, "/dev/sda1");
        assert_eq!(mounts[0].fstype, "ext4");
    }

    #[test]
    fn parse_mountpoint_with_space() {
        let stdout = "/dev/disk2s1 on /Volumes/My Drive (msdos, local, nodev)\n";
        let mounts = parse_mount_output(stdout);
        assert_eq!(mounts.len(), 1);
        assert_eq!(mounts[0].mountpoint, "/Volumes/My Drive");
        assert_eq!(mounts[0].fstype, "msdos");
        assert!(mounts[0].nodev);
    }
}
