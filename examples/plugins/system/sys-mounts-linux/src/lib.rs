// sys-mounts-linux — read /proc/mounts + /etc/fstab, return a
// unified MountListResponse with both the live and the configured
// state cross-referenced.
//
// /proc/mounts format (one record per line):
//   <source> <mountpoint> <fstype> <options> <dump> <pass>
//   /dev/sda1 /          ext4 rw,relatime,errors=remount-ro 0 1
//   tmpfs    /run/lock   tmpfs rw,nosuid,nodev,noexec,size=5120k 0 0
//
// /etc/fstab format: same six fields, comments start with '#'.

use extism_pdk::*;
use serde::{Deserialize, Serialize};

#[cfg(target_arch = "wasm32")]
#[host_fn("platypus")]
extern "ExtismHost" {
    fn host_fs_read(path: String) -> Json<Envelope>;
}

#[derive(Deserialize, Default)]
struct Envelope {
    ok: bool,
    #[serde(default)]
    data: serde_json::Value,
    #[serde(default)]
    error: String,
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
    fstab: Vec<FstabEntry>,
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

#[derive(Serialize, Default, Debug, PartialEq)]
struct FstabEntry {
    source: String,
    mountpoint: String,
    fstype: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    options: String,
    #[serde(skip_serializing_if = "is_zero_u32")]
    dump: u32,
    #[serde(skip_serializing_if = "is_zero_u32")]
    pass: u32,
    mounted: bool,
}

fn is_false(b: &bool) -> bool { !*b }
fn is_zero_u32(n: &u32) -> bool { *n == 0 }

// Pseudo / virtual filesystem types — never backed by a block device.
const PSEUDO_FS: &[&str] = &[
    "tmpfs", "devtmpfs", "devpts", "proc", "sysfs", "cgroup", "cgroup2",
    "pstore", "bpf", "tracefs", "debugfs", "fusectl", "configfs",
    "securityfs", "hugetlbfs", "mqueue", "rpc_pipefs", "binfmt_misc",
    "autofs", "nsfs", "overlay", "squashfs", "ramfs", "fuse.gvfsd-fuse",
    "fuse.portal", "efivarfs", "selinuxfs",
];

// ---- entry point ----

#[cfg(target_arch = "wasm32")]
#[plugin_fn]
pub fn list_mounts(req: Json<ListRequest>) -> FnResult<String> {
    let r = req.0;

    // /proc/self/mounts is a more reliable read than /proc/mounts
    // when the agent itself is in a mount namespace; fall back to
    // /proc/mounts on older kernels.
    let mounts_raw = read_string("/proc/self/mounts")
        .or_else(|| read_string("/proc/mounts"))
        .unwrap_or_default();
    let mut mounts = parse_proc_mounts(&mounts_raw);
    if !r.include_pseudo {
        mounts.retain(|m| !m.pseudo);
    }

    // Build a mountpoint → exists map for fstab cross-reference.
    let mounted_set: std::collections::HashSet<String> =
        parse_proc_mounts(&mounts_raw)
            .into_iter()
            .map(|m| m.mountpoint)
            .collect();

    let fstab_raw = read_string("/etc/fstab").unwrap_or_default();
    let mut fstab = parse_fstab(&fstab_raw);
    for f in &mut fstab {
        f.mounted = mounted_set.contains(&f.mountpoint);
    }
    if !r.include_active_fstab {
        fstab.retain(|f| !f.mounted);
    }

    Ok(serde_json::to_string(&ListResponse {
        mounts,
        fstab,
        error: String::new(),
    })?)
}

// ---- pure parsers ----

// parse_proc_mounts walks /proc/mounts (or /proc/self/mounts) line by
// line. Six whitespace-separated fields per record; the kernel
// escapes spaces in paths as "\\040" — we leave those alone, the
// operator-facing UI decodes them client-side if needed.
fn parse_proc_mounts(body: &str) -> Vec<Mount> {
    let mut out = Vec::new();
    for line in body.lines() {
        let trimmed = line.trim();
        if trimmed.is_empty() || trimmed.starts_with('#') {
            continue;
        }
        let parts: Vec<&str> = trimmed.split_whitespace().collect();
        if parts.len() < 4 {
            continue;
        }
        let mut m = Mount {
            source: parts[0].to_string(),
            mountpoint: parts[1].to_string(),
            fstype: parts[2].to_string(),
            options: parts[3].to_string(),
            ..Default::default()
        };
        apply_option_flags(&mut m);
        m.pseudo = is_pseudo_fs(&m.fstype);
        out.push(m);
    }
    out
}

fn parse_fstab(body: &str) -> Vec<FstabEntry> {
    let mut out = Vec::new();
    for line in body.lines() {
        let no_comment = match line.find('#') {
            Some(idx) => &line[..idx],
            None => line,
        };
        let trimmed = no_comment.trim();
        if trimmed.is_empty() {
            continue;
        }
        let parts: Vec<&str> = trimmed.split_whitespace().collect();
        if parts.len() < 4 {
            continue;
        }
        let dump = if parts.len() > 4 { parts[4].parse().unwrap_or(0) } else { 0 };
        let pass = if parts.len() > 5 { parts[5].parse().unwrap_or(0) } else { 0 };
        out.push(FstabEntry {
            source: parts[0].to_string(),
            mountpoint: parts[1].to_string(),
            fstype: parts[2].to_string(),
            options: parts[3].to_string(),
            dump,
            pass,
            mounted: false,
        });
    }
    out
}

fn apply_option_flags(m: &mut Mount) {
    for opt in m.options.split(',') {
        match opt.trim() {
            "ro" => m.read_only = true,
            "rw" => m.read_only = false,
            "nosuid" => m.nosuid = true,
            "nodev" => m.nodev = true,
            "noexec" => m.noexec = true,
            _ => {}
        }
    }
}

fn is_pseudo_fs(fstype: &str) -> bool {
    PSEUDO_FS.iter().any(|p| *p == fstype) || fstype.starts_with("fuse.")
}

#[cfg(target_arch = "wasm32")]
fn read_string(path: &str) -> Option<String> {
    let env: Envelope = unsafe { host_fs_read(path.to_string()).ok()?.0 };
    if !env.ok {
        return None;
    }
    env.data.as_str().map(|s| s.to_string())
}

// ============================================================
// Pure-function unit tests
// ============================================================
#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parse_basic_proc_mounts() {
        let body = "\
/dev/sda1 / ext4 rw,relatime,errors=remount-ro 0 0
tmpfs /run/lock tmpfs rw,nosuid,nodev,noexec,size=5120k 0 0
proc /proc proc rw,nosuid,nodev,noexec,relatime 0 0
";
        let mounts = parse_proc_mounts(body);
        assert_eq!(mounts.len(), 3);
        assert_eq!(mounts[0].source, "/dev/sda1");
        assert_eq!(mounts[0].mountpoint, "/");
        assert!(!mounts[0].read_only);
        assert!(!mounts[0].nosuid);
        assert!(!mounts[0].pseudo);

        assert_eq!(mounts[1].fstype, "tmpfs");
        assert!(mounts[1].nosuid);
        assert!(mounts[1].nodev);
        assert!(mounts[1].noexec);
        assert!(mounts[1].pseudo);

        assert!(mounts[2].pseudo); // proc fs
    }

    #[test]
    fn parse_proc_mounts_ro_flag() {
        let body = "/dev/sda1 / ext4 ro,relatime 0 0\n";
        let mounts = parse_proc_mounts(body);
        assert_eq!(mounts.len(), 1);
        assert!(mounts[0].read_only);
    }

    #[test]
    fn parse_proc_mounts_skips_blank_and_short() {
        let body = "\

/dev/sda1 /
short
/dev/sda1 / ext4 rw 0 0
";
        // First two are too short, third is valid.
        let mounts = parse_proc_mounts(body);
        assert_eq!(mounts.len(), 1);
    }

    #[test]
    fn parse_basic_fstab() {
        let body = "\
# /etc/fstab — managed by ansible
UUID=abc / ext4 errors=remount-ro 0 1
/dev/sdb1 /data xfs defaults,nofail 0 2
tmpfs /tmp tmpfs defaults 0 0
";
        let fstab = parse_fstab(body);
        assert_eq!(fstab.len(), 3);
        assert_eq!(fstab[0].source, "UUID=abc");
        assert_eq!(fstab[0].pass, 1);
        assert_eq!(fstab[1].pass, 2);
        assert_eq!(fstab[2].fstype, "tmpfs");
    }

    #[test]
    fn parse_fstab_skips_comments_and_blanks() {
        let body = "\
# this is the root mount
/dev/sda1 / ext4 defaults 0 1

# disabled — moved to NFS
# 192.168.1.1:/data /data nfs defaults 0 0
";
        let fstab = parse_fstab(body);
        assert_eq!(fstab.len(), 1);
        assert_eq!(fstab[0].source, "/dev/sda1");
    }

    #[test]
    fn parse_fstab_inline_comment() {
        let body = "/dev/sda1 / ext4 defaults 0 1  # root\n";
        let fstab = parse_fstab(body);
        assert_eq!(fstab.len(), 1);
        assert_eq!(fstab[0].pass, 1);
    }

    #[test]
    fn pseudo_fs_classification() {
        assert!(is_pseudo_fs("tmpfs"));
        assert!(is_pseudo_fs("proc"));
        assert!(is_pseudo_fs("sysfs"));
        assert!(is_pseudo_fs("cgroup2"));
        assert!(is_pseudo_fs("overlay"));
        assert!(is_pseudo_fs("fuse.gvfsd-fuse"));
        assert!(!is_pseudo_fs("ext4"));
        assert!(!is_pseudo_fs("xfs"));
        assert!(!is_pseudo_fs("nfs4"));
    }

    #[test]
    fn options_parser_handles_compound() {
        let body = "/dev/foo /mnt ext4 rw,nosuid,nodev,noexec,errors=remount-ro 0 0\n";
        let mounts = parse_proc_mounts(body);
        let m = &mounts[0];
        assert!(!m.read_only);
        assert!(m.nosuid);
        assert!(m.nodev);
        assert!(m.noexec);
    }
}
