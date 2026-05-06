// sys-disk-darwin — `df -k -P` on macOS.
//
// macOS df differs from GNU df in two ways the parser must handle:
//   1. No `-B1` (GNU-only). We pass `-k` (1024-byte blocks) and
//      scale up. `-P` enforces POSIX one-line-per-fs output —
//      avoids the wrapped-long-source case sys-disk-linux had to
//      defend against.
//   2. `-T <fstype-list>` works differently: GNU's `-T` adds a
//      Type column, BSD's `-T` filters by fstype. We don't use
//      either flag here; instead we parse without the Type column
//      and recover fstype from a parallel `mount` call... or, more
//      pragmatically, from the FILESYSTEM column when it's an
//      apfs container path (e.g. /dev/disk1s1).
//
// macOS POSIX df output (-k -P):
//
//   Filesystem  1024-blocks       Used Available Capacity Mounted on
//   /dev/disk1s5  244277768   12345678 231932090     6%   /
//   devfs               376        376         0   100%   /dev
//
// fstype is NOT in the output. Approach: the source paths give it
// away (anything starting with /dev/disk is APFS / HFS+ on Apple
// Silicon / Intel respectively; anything ELSE is pseudo). We
// classify post-parse using the source prefix and the fact that
// mountpoints like /dev, /System/Volumes/* etc. are well-known
// pseudo-fs locations.

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

// Heuristic: source paths starting with these prefixes are pseudo
// (synthesised by macOS for sandboxing or system snapshots) and
// should be filtered when skip_pseudo=true.
const PSEUDO_SOURCE_PREFIXES: &[&str] = &[
    "devfs",
    "map ",       // "map auto_home", "map -hosts"
    "autofs",
    "/dev/disk1s4", // common APFS swap snapshot on Intel; rough
];

// macOS pseudo mountpoints — these are firmlinks / bind mounts
// the OS creates for sandbox isolation; they double-count storage.
const PSEUDO_MOUNTPOINT_PREFIXES: &[&str] =
    &["/System/Volumes/Update", "/System/Volumes/VM", "/private/var/vm"];

#[cfg(target_arch = "wasm32")]
#[plugin_fn]
pub fn list_filesystems(req: Json<ListRequest>) -> FnResult<String> {
    let r = req.0;

    let exec_resp = match run_df(7_000) {
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
                "df exit {}: {}",
                exec_resp.exit_code,
                exec_resp.stderr.trim()
            ),
        })?);
    }
    let parsed = parse_df_output(&exec_resp.stdout);
    let filtered = apply_filters(parsed, &r);
    Ok(serde_json::to_string(&ListResponse {
        filesystems: filtered,
        error: String::new(),
    })?)
}

// parse_df_output expects `df -k -P` shape: 6 columns per row
// (Filesystem, 1024-blocks, Used, Available, Capacity, "Mounted on"+).
// Numbers are 1024-byte blocks; we scale up to bytes here.
//
// fstype is absent from the output. We classify it by source pattern
// (apfs for /dev/disk*s* on Apple Silicon; devfs for "devfs") in a
// best-effort post-pass. The operator-visible value is informational.
pub fn parse_df_output(stdout: &str) -> Vec<Filesystem> {
    let mut out = Vec::new();
    let mut lines = stdout.lines();

    // Skip header.
    match lines.next() {
        Some(h) if h.trim_start().starts_with("Filesystem") => {}
        _ => return out,
    }

    for line in lines {
        let trimmed = line.trim_end();
        if trimmed.is_empty() {
            continue;
        }
        let tokens: Vec<&str> = trimmed.split_whitespace().collect();
        if tokens.len() < 6 {
            continue;
        }
        // Right-anchored parse: last token is mountpoint, then
        // capacity, available, used, blocks. Source is everything
        // before. Mountpoints with embedded spaces aren't handled;
        // POSIX df with -P typically guarantees no wrapping, so a
        // multi-word mount is rare-but-possible.
        let n = tokens.len();
        let mountpoint = tokens[n - 1].to_string();
        let cap_str = tokens[n - 2];
        let avail_str = tokens[n - 3];
        let used_str = tokens[n - 4];
        let blocks_str = tokens[n - 5];
        let source = tokens[..n - 5].join(" ");

        let percent_used = parse_percent(cap_str);
        let blocks: u64 = blocks_str.parse().unwrap_or(0);
        let used_kb: u64 = used_str.parse().unwrap_or(0);
        let avail_kb: u64 = avail_str.parse().unwrap_or(0);

        let fstype = classify_fstype(&source, &mountpoint);

        out.push(Filesystem {
            source,
            fstype,
            mountpoint,
            // df -k reports values in 1024-byte blocks.
            size_bytes: blocks * 1024,
            used_bytes: used_kb * 1024,
            available_bytes: avail_kb * 1024,
            percent_used,
        });
    }
    out
}

// classify_fstype is a shallow heuristic — macOS df doesn't carry the
// fstype column. Best-effort labels so the operator UI doesn't show
// a column of empty strings.
fn classify_fstype(source: &str, mountpoint: &str) -> String {
    if source == "devfs" {
        return "devfs".to_string();
    }
    if source.starts_with("map ") {
        return "autofs".to_string();
    }
    if source.starts_with("/dev/disk") {
        // Modern macOS is APFS by default. The HFS+ era is decade-old.
        return "apfs".to_string();
    }
    if mountpoint.starts_with("/System/Volumes") {
        return "apfs".to_string();
    }
    "unknown".to_string()
}

fn parse_percent(s: &str) -> u8 {
    let stripped = s.trim_end_matches('%');
    if stripped == "-" {
        return 0;
    }
    let n: u32 = stripped.parse().unwrap_or(0);
    if n > 100 {
        100
    } else {
        n as u8
    }
}

pub fn apply_filters(input: Vec<Filesystem>, req: &ListRequest) -> Vec<Filesystem> {
    input
        .into_iter()
        .filter(|fs| {
            if req.skip_pseudo && is_pseudo(&fs.source, &fs.mountpoint) {
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

fn is_pseudo(source: &str, mountpoint: &str) -> bool {
    for prefix in PSEUDO_SOURCE_PREFIXES {
        if source.starts_with(prefix) {
            return true;
        }
    }
    for prefix in PSEUDO_MOUNTPOINT_PREFIXES {
        if mountpoint.starts_with(prefix) {
            return true;
        }
    }
    false
}

#[cfg(target_arch = "wasm32")]
fn run_df(timeout_ms: u32) -> Result<ExecResponse, String> {
    let args: Vec<String> = vec!["-k".to_string(), "-P".to_string()];
    let req = ExecRequest {
        command: "/bin/df".to_string(),
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
fn run_df(_timeout_ms: u32) -> Result<ExecResponse, String> {
    Err("not available on host build".to_string())
}

// ============================================================
// tests (host-build only — pure parser)
// ============================================================

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parse_basic_three_rows() {
        let stdout = "\
Filesystem    1024-blocks      Used Available Capacity  Mounted on
/dev/disk1s5    244277768  12345678 231932090     6% /
devfs                 376       376         0   100% /dev
map -hosts              0         0         0     -  /net
";
        let got = parse_df_output(stdout);
        assert_eq!(got.len(), 3);
        assert_eq!(got[0].source, "/dev/disk1s5");
        assert_eq!(got[0].fstype, "apfs");
        assert_eq!(got[0].mountpoint, "/");
        // 244,277,768 KB × 1024 = ~244 GB
        assert_eq!(got[0].size_bytes, 244_277_768u64 * 1024);
        assert_eq!(got[0].used_bytes, 12_345_678u64 * 1024);
        assert_eq!(got[0].available_bytes, 231_932_090u64 * 1024);
        assert_eq!(got[0].percent_used, 6);

        assert_eq!(got[1].fstype, "devfs");
        assert_eq!(got[2].fstype, "autofs");
        assert_eq!(got[2].percent_used, 0); // "-" → 0
    }

    #[test]
    fn parse_returns_empty_on_missing_header() {
        let stdout = "/dev/disk1 1 0 1 0% /\n";
        assert!(parse_df_output(stdout).is_empty());
    }

    #[test]
    fn parse_skips_short_rows() {
        let stdout = "Filesystem 1024-blocks Used Available Capacity Mounted on\n/dev/disk1 100\n";
        assert!(parse_df_output(stdout).is_empty());
    }

    #[test]
    fn classify_fstype_known_prefixes() {
        assert_eq!(classify_fstype("/dev/disk2s4", "/"), "apfs");
        assert_eq!(classify_fstype("devfs", "/dev"), "devfs");
        assert_eq!(classify_fstype("map auto_home", "/home"), "autofs");
        assert_eq!(classify_fstype("//user@server/share", "/Volumes/share"), "unknown");
    }

    #[test]
    fn apply_filters_skip_pseudo_drops_devfs_and_map() {
        let input = vec![
            Filesystem {
                source: "/dev/disk1s5".into(),
                fstype: "apfs".into(),
                mountpoint: "/".into(),
                size_bytes: 1_000_000,
                ..Default::default()
            },
            Filesystem {
                source: "devfs".into(),
                fstype: "devfs".into(),
                mountpoint: "/dev".into(),
                size_bytes: 0,
                ..Default::default()
            },
            Filesystem {
                source: "map -hosts".into(),
                fstype: "autofs".into(),
                mountpoint: "/net".into(),
                ..Default::default()
            },
            Filesystem {
                source: "/dev/disk1s2".into(),
                fstype: "apfs".into(),
                mountpoint: "/System/Volumes/Update".into(),
                ..Default::default()
            },
        ];
        let req = ListRequest {
            skip_pseudo: true,
            ..Default::default()
        };
        let out = apply_filters(input, &req);
        assert_eq!(out.len(), 1);
        assert_eq!(out[0].mountpoint, "/");
    }

    #[test]
    fn apply_filters_only_mountpoints() {
        let input = vec![
            Filesystem {
                source: "/dev/disk1".into(),
                fstype: "apfs".into(),
                mountpoint: "/".into(),
                size_bytes: 1000,
                ..Default::default()
            },
            Filesystem {
                source: "/dev/disk2".into(),
                fstype: "apfs".into(),
                mountpoint: "/Volumes/data".into(),
                size_bytes: 1000,
                ..Default::default()
            },
        ];
        let req = ListRequest {
            skip_pseudo: false,
            only_mountpoints: vec!["/Volumes/data".into()],
            ..Default::default()
        };
        let out = apply_filters(input, &req);
        assert_eq!(out.len(), 1);
        assert_eq!(out[0].mountpoint, "/Volumes/data");
    }

    #[test]
    fn parse_percent_clamps_and_handles_dash() {
        assert_eq!(parse_percent("0%"), 0);
        assert_eq!(parse_percent("100%"), 100);
        assert_eq!(parse_percent("-"), 0);
        assert_eq!(parse_percent(""), 0);
    }
}
