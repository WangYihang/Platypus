// sys-disk-linux — `list_filesystems` RPC over `df -B1 -T`.
//
// Why `df` and not /proc/self/mountinfo + statvfs(2): wasm32 has no
// statvfs binding, /proc/mounts gives mount metadata but no
// size/used/avail numbers. Shelling to df is cheap (<10ms typical),
// produces a stable, well-documented format, and the parse target is
// fixed-width-ish columns that match across every modern distro
// (Debian/Ubuntu/RHEL/Arch). A v2 may switch to a host_fs_statfs
// host fn if wire-stability becomes a problem, but it isn't yet.
//
// Wire shape (request):
//   {
//     skip_pseudo:       bool, default true   (drop tmpfs/devtmpfs/proc/sysfs/cgroup/…)
//     only_mountpoints:  ["/", "/var/lib"],   default empty (no filter)
//     min_size_bytes:    u64, default 0       (drop fs smaller than this)
//   }
//
// Wire shape (response):
//   {
//     filesystems: [
//       { source: "/dev/vda1",
//         fstype: "ext4",
//         mountpoint: "/",
//         size_bytes: u64, used_bytes: u64, available_bytes: u64,
//         percent_used: u8 (0..=100, derived from df's "Use%" column)
//       }, …
//     ],
//     error: ""
//   }

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

// Pseudo-filesystem types that are noise in 99% of operator queries.
// Operators who care about cgroup/tmpfs accounting can disable the
// filter via skip_pseudo=false.
const PSEUDO_FSTYPES: &[&str] = &[
    "tmpfs",
    "devtmpfs",
    "proc",
    "sysfs",
    "cgroup",
    "cgroup2",
    "pstore",
    "bpf",
    "tracefs",
    "debugfs",
    "configfs",
    "fusectl",
    "hugetlbfs",
    "mqueue",
    "ramfs",
    "rpc_pipefs",
    "securityfs",
    "selinuxfs",
    "binfmt_misc",
    "autofs",
    "efivarfs",
    "nsfs",
    "fuse.gvfsd-fuse",
    "fuse.portal",
    "fuse.snapfuse",
    "overlay",
    "squashfs",
];

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

// parse_df_output handles the `df -B1 -T` format:
//   Filesystem     Type     1B-blocks   Used    Available Use% Mounted on
//   /dev/vda1      ext4     270553174016 14199996928 25780891648 36% /
//   tmpfs          tmpfs    8430878720   0       8430878720  0% /dev/shm
//
// Whitespace columns. The Filesystem name MAY contain spaces in
// theory (rare; mostly NFS/SMB mounts with paths). For robustness
// we lock onto the LAST 6 whitespace-separated tokens (Type, blocks,
// Used, Available, Use%, mountpoint) — the leftover prefix is the
// source. Mountpoints with embedded spaces would break this; df has
// no quoting convention so the rule "mountpoint = last token" is
// the established practice in every shell parser of df output.
pub fn parse_df_output(stdout: &str) -> Vec<Filesystem> {
    let mut out = Vec::new();
    let mut lines = stdout.lines();

    // Skip the header line. If the first line doesn't start with
    // "Filesystem" the format isn't what we expected; bail.
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
        if tokens.len() < 7 {
            // Wrapped rows (rare, when Filesystem alone doesn't fit
            // and df breaks the line). Skip rather than mis-parse —
            // the operator's UI shows "n filesystems" and a
            // wrapped one would be inconsistent anyway.
            continue;
        }
        // Parse from the right: last token is mountpoint, then Use%,
        // Available, Used, blocks, Type. Source is everything before.
        let n = tokens.len();
        let mountpoint = tokens[n - 1].to_string();
        let pct_str = tokens[n - 2];
        let avail_str = tokens[n - 3];
        let used_str = tokens[n - 4];
        let blocks_str = tokens[n - 5];
        let fstype = tokens[n - 6].to_string();
        let source = tokens[..n - 6].join(" ");

        let percent_used = parse_percent(pct_str);
        let available_bytes = avail_str.parse().unwrap_or(0);
        let used_bytes = used_str.parse().unwrap_or(0);
        let size_bytes = blocks_str.parse().unwrap_or(0);

        out.push(Filesystem {
            source,
            fstype,
            mountpoint,
            size_bytes,
            used_bytes,
            available_bytes,
            percent_used,
        });
    }
    out
}

// parse_percent strips df's "Use%" formatting ("36%", "100%", "-")
// and returns 0..=100 clamped. "-" appears for filesystems where
// usage doesn't apply (e.g. /proc); we report 0.
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

// apply_filters trims the result by the request's filter args. Done
// post-parse rather than via df flags so the parser stays a pure
// fn (testable without a real df).
pub fn apply_filters(input: Vec<Filesystem>, req: &ListRequest) -> Vec<Filesystem> {
    input
        .into_iter()
        .filter(|fs| {
            if req.skip_pseudo && PSEUDO_FSTYPES.contains(&fs.fstype.as_str()) {
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

// ---------- exec helper ----------

#[cfg(target_arch = "wasm32")]
fn run_df(timeout_ms: u32) -> Result<ExecResponse, String> {
    let args: Vec<String> = vec![
        "-B".to_string(),
        "1".to_string(),
        "-T".to_string(),
        "-P".to_string(),
    ];
    for path in &["/usr/bin/df", "/bin/df"] {
        let req = ExecRequest {
            command: path.to_string(),
            args: args.clone(),
            timeout_ms,
        };
        let body = match serde_json::to_string(&req) {
            Ok(b) => b,
            Err(e) => return Err(format!("encode_exec_req: {}", e)),
        };
        let env: Envelope = match unsafe { host_exec(body) } {
            Ok(j) => j.0,
            Err(e) => return Err(format!("host_exec: {}", e)),
        };
        if !env.ok {
            if env.error.contains("capability_denied") {
                return Err(env.error);
            }
            continue;
        }
        let resp: ExecResponse = serde_json::from_value(env.data)
            .map_err(|e| format!("decode_exec_resp: {}", e))?;
        return Ok(resp);
    }
    Err("df_not_found_on_either_path".to_string())
}

#[cfg(not(target_arch = "wasm32"))]
fn run_df(_timeout_ms: u32) -> Result<ExecResponse, String> {
    Err("not available on host build".to_string())
}

// ============================================================
// tests
// ============================================================

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parse_basic_three_rows() {
        let stdout = "\
Filesystem     Type     1B-blocks       Used  Available Use% Mounted on
tmpfs          tmpfs    8430878720         0  8430878720  0% /dev/shm
/dev/vda       ext4   270553174016 14199996928 25780891648 36% /
/dev/vdb1      xfs   1099511627776 549755813888 549755813888 50% /var/lib
";
        let got = parse_df_output(stdout);
        assert_eq!(got.len(), 3);
        assert_eq!(got[0].fstype, "tmpfs");
        assert_eq!(got[0].percent_used, 0);
        assert_eq!(got[1].source, "/dev/vda");
        assert_eq!(got[1].fstype, "ext4");
        assert_eq!(got[1].size_bytes, 270_553_174_016);
        assert_eq!(got[1].used_bytes, 14_199_996_928);
        assert_eq!(got[1].available_bytes, 25_780_891_648);
        assert_eq!(got[1].percent_used, 36);
        assert_eq!(got[1].mountpoint, "/");
        assert_eq!(got[2].mountpoint, "/var/lib");
    }

    #[test]
    fn parse_handles_dash_percent_and_zero_size() {
        // /proc-style row where Use% reports "-".
        let stdout = "\
Filesystem     Type     1B-blocks Used Available Use% Mounted on
proc           proc     0         0    0          -    /proc
";
        let got = parse_df_output(stdout);
        assert_eq!(got.len(), 1);
        assert_eq!(got[0].percent_used, 0);
        assert_eq!(got[0].size_bytes, 0);
    }

    #[test]
    fn parse_skips_short_rows() {
        let stdout = "\
Filesystem     Type     1B-blocks
malformed_row
";
        assert!(parse_df_output(stdout).is_empty());
    }

    #[test]
    fn parse_returns_empty_on_missing_header() {
        let stdout = "tmpfs tmpfs 100 0 100 0% /dev/shm\n";
        assert!(parse_df_output(stdout).is_empty());
    }

    #[test]
    fn parse_handles_source_with_spaces() {
        // NFS mount with embedded space in path. Source is everything
        // up to (n-6).
        let stdout = "\
Filesystem     Type     1B-blocks Used Available Use% Mounted on
nfs server:/data nfs 100 50 50 50% /mnt/nfs
";
        let got = parse_df_output(stdout);
        assert_eq!(got.len(), 1);
        assert_eq!(got[0].source, "nfs server:/data");
        assert_eq!(got[0].mountpoint, "/mnt/nfs");
    }

    #[test]
    fn apply_filters_skip_pseudo() {
        let input = vec![
            Filesystem {
                fstype: "tmpfs".into(),
                size_bytes: 1000,
                ..Default::default()
            },
            Filesystem {
                fstype: "ext4".into(),
                size_bytes: 1000,
                ..Default::default()
            },
        ];
        let req = ListRequest {
            skip_pseudo: true,
            ..Default::default()
        };
        let out = apply_filters(input, &req);
        assert_eq!(out.len(), 1);
        assert_eq!(out[0].fstype, "ext4");
    }

    #[test]
    fn apply_filters_min_size_bytes() {
        let input = vec![
            Filesystem {
                fstype: "ext4".into(),
                size_bytes: 100,
                ..Default::default()
            },
            Filesystem {
                fstype: "ext4".into(),
                size_bytes: 1_000_000,
                ..Default::default()
            },
        ];
        let req = ListRequest {
            skip_pseudo: false,
            min_size_bytes: 1000,
            ..Default::default()
        };
        let out = apply_filters(input, &req);
        assert_eq!(out.len(), 1);
        assert_eq!(out[0].size_bytes, 1_000_000);
    }

    #[test]
    fn apply_filters_only_mountpoints() {
        let input = vec![
            Filesystem {
                mountpoint: "/".into(),
                fstype: "ext4".into(),
                size_bytes: 1000,
                ..Default::default()
            },
            Filesystem {
                mountpoint: "/var".into(),
                fstype: "ext4".into(),
                size_bytes: 1000,
                ..Default::default()
            },
        ];
        let req = ListRequest {
            skip_pseudo: false,
            only_mountpoints: vec!["/var".into()],
            ..Default::default()
        };
        let out = apply_filters(input, &req);
        assert_eq!(out.len(), 1);
        assert_eq!(out[0].mountpoint, "/var");
    }

    #[test]
    fn parse_percent_clamps_and_handles_dash() {
        assert_eq!(parse_percent("0%"), 0);
        assert_eq!(parse_percent("100%"), 100);
        assert_eq!(parse_percent("150%"), 100);
        assert_eq!(parse_percent("-"), 0);
        assert_eq!(parse_percent(""), 0);
    }
}
