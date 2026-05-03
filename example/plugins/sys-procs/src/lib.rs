// sys-procs v2 — real implementation. Enumerates /proc/<pid>
// entries and reads /proc/<pid>/{stat,status,cmdline} directly via
// host_fs_read + host_fs_listdir. The agent-side host_process_list
// + the gopsutil-backed CollectProcessList are no longer in the
// path; this plugin owns the data collection.
//
// Capability: fs.read of /proc.
//
// Coverage vs gopsutil baseline:
//   - pid, ppid, name, cmdline, status (single char from
//     /proc/<pid>/stat field 3)
//   - rss_bytes (from /proc/<pid>/statm field 2 × pagesize=4096)
//   - num_threads (from /proc/<pid>/status: "Threads:")
//   - user (from /proc/<pid>/status: "Uid:" → numeric uid mapped via
//     /etc/passwd; left empty if the lookup fails)
//   - created_at_unix (left at 0 — requires boot_time + jiffies math
//     which gopsutil also approximates; v3 work)
//   - cpu_percent / mem_percent (left at 0 — requires sampling
//     /proc/stat across two readings; per-call, single-shot can't do
//     it)
//
// Sort order honours request.sort_by ("cpu" default, "mem", "rss",
// "pid"). top_n=0 means "all" (capped at 500 to match the legacy
// handler's wire-bound).

use extism_pdk::*;
use serde::{Deserialize, Serialize};
use std::collections::HashMap;

const PROC_LIST_CAP: u32 = 500;
const PAGE_SIZE: u64 = 4096; // x86_64 / arm64 default; 16KiB hosts will under-report

#[host_fn("platypus")]
extern "ExtismHost" {
    fn host_fs_read(path: String) -> Json<Envelope>;
    fn host_fs_listdir(path: String) -> Json<Envelope>;
}

#[derive(Deserialize, Default)]
struct Envelope {
    ok: bool,
    #[serde(default)]
    data: serde_json::Value,
    #[serde(default)]
    error: String,
}

#[derive(Deserialize)]
struct DirEntryJSON {
    name: String,
    is_dir: bool,
    #[serde(default)]
    size: i64,
    #[serde(default)]
    mtime_unix: i64,
}

#[derive(Deserialize)]
struct ProcessListRequest {
    #[serde(default)]
    top_n: u32,
    #[serde(default)]
    sort_by: String,
}

// ProcessInfo mirrors v2pb.ProcessInfo's protojson encoding.
#[derive(Serialize)]
struct ProcessInfo {
    pid: u32,
    ppid: u32,
    user: String,
    name: String,
    cmdline: String,
    status: String,
    #[serde(rename = "cpuPercent")]
    cpu_percent: f64,
    #[serde(rename = "memPercent")]
    mem_percent: f64,
    #[serde(rename = "rssBytes")]
    rss_bytes: u64,
    #[serde(rename = "numThreads")]
    num_threads: u32,
    #[serde(rename = "createdAtUnix")]
    created_at_unix: i64,
}

#[derive(Serialize)]
struct ProcessListResponse {
    processes: Vec<ProcessInfo>,
    #[serde(rename = "totalCount")]
    total_count: u32,
    #[serde(skip_serializing_if = "String::is_empty")]
    error: String,
}

#[plugin_fn]
pub fn process_list(req: Json<ProcessListRequest>) -> FnResult<String> {
    let pids = list_pids();
    let total = pids.len() as u32;
    let user_map = read_passwd_map();

    let mut procs: Vec<ProcessInfo> = pids
        .into_iter()
        .filter_map(|pid| read_one_process(pid, &user_map))
        .collect();

    let sort_by = req.0.sort_by.as_str();
    procs.sort_by(|a, b| match sort_by {
        "mem" | "rss" => b.rss_bytes.cmp(&a.rss_bytes),
        "pid" => a.pid.cmp(&b.pid),
        // Default ("cpu" or empty): we can't compute cpu_percent
        // single-shot, so fall back to RSS so the response is
        // deterministic + the operator still sees the heaviest
        // processes first.
        _ => b.rss_bytes.cmp(&a.rss_bytes),
    });

    let mut top_n = req.0.top_n;
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

// list_pids walks /proc and returns each numeric subdir name as u32.
// Sorted ascending so the post-sort step has a stable input order.
fn list_pids() -> Vec<u32> {
    let env: Envelope = match unsafe { host_fs_listdir("/proc".to_string()) } {
        Ok(j) => j.0,
        Err(_) => return Vec::new(),
    };
    if !env.ok {
        return Vec::new();
    }
    let entries: Vec<DirEntryJSON> = serde_json::from_value(env.data).unwrap_or_default();
    let mut out: Vec<u32> = entries
        .into_iter()
        .filter(|e| e.is_dir)
        .filter_map(|e| e.name.parse::<u32>().ok())
        .collect();
    out.sort_unstable();
    out
}

// read_one_process pulls /proc/<pid>/stat + /status + /cmdline in
// one shot. None when the process disappeared between listdir and
// the per-pid reads (the common race that all /proc walkers hit).
fn read_one_process(pid: u32, user_map: &HashMap<u32, String>) -> Option<ProcessInfo> {
    // /proc/<pid>/stat: space-delimited fields, but field 2 (the
    // command name) is wrapped in parens and can itself contain
    // spaces. Parse: prefix-up-to-first-'('; then matched-paren
    // command; then space-delimited rest.
    let stat = read_string(&format!("/proc/{}/stat", pid))?;
    let lparen = stat.find('(')?;
    let rparen = stat.rfind(')')?;
    if rparen < lparen {
        return None;
    }
    let pid_field = stat[..lparen].trim().parse::<u32>().ok()?;
    let comm = stat[lparen + 1..rparen].to_string();
    let after = stat[rparen + 1..].trim();
    let rest: Vec<&str> = after.split_whitespace().collect();
    // After the command, fields are 1-indexed in proc(5) starting at
    // "state". So rest[0] = state, rest[1] = ppid, ...
    let status_char = rest.get(0).copied().unwrap_or("?").to_string();
    let ppid: u32 = rest.get(1).and_then(|s| s.parse().ok()).unwrap_or(0);

    // /proc/<pid>/cmdline: NUL-separated argv; gopsutil truncates at
    // 512 bytes for the wire and we mirror that.
    let cmdline_raw = read_string(&format!("/proc/{}/cmdline", pid)).unwrap_or_default();
    let cmdline_joined: String = cmdline_raw
        .replace('\0', " ")
        .trim()
        .chars()
        .take(512)
        .collect();

    // /proc/<pid>/statm: "size resident shared text lib data dt"
    // Pages → bytes via PAGE_SIZE. The legacy handler used the
    // host's actual page size; we hardcode the common 4 KiB page —
    // hosts on 16 KiB pages will under-report by 4×.
    let rss_bytes = read_string(&format!("/proc/{}/statm", pid))
        .and_then(|s| s.split_whitespace().nth(1).map(|v| v.to_string()))
        .and_then(|v| v.parse::<u64>().ok())
        .map(|n| n.saturating_mul(PAGE_SIZE))
        .unwrap_or(0);

    // /proc/<pid>/status: key/value lines. We need Threads + Uid.
    let status = read_string(&format!("/proc/{}/status", pid)).unwrap_or_default();
    let mut num_threads: u32 = 0;
    let mut user = String::new();
    for line in status.lines() {
        if let Some(rest) = line.strip_prefix("Threads:") {
            num_threads = rest.trim().parse().unwrap_or(0);
        } else if let Some(rest) = line.strip_prefix("Uid:") {
            // "Uid: <real> <eff> <saved> <fs>" — use real.
            if let Some(uid_str) = rest.split_whitespace().next() {
                if let Ok(uid) = uid_str.parse::<u32>() {
                    user = user_map.get(&uid).cloned().unwrap_or_else(|| uid.to_string());
                }
            }
        }
    }

    Some(ProcessInfo {
        pid: pid_field,
        ppid,
        user,
        name: comm,
        cmdline: cmdline_joined,
        status: status_char,
        cpu_percent: 0.0,
        mem_percent: 0.0,
        rss_bytes,
        num_threads,
        created_at_unix: 0,
    })
}

// read_passwd_map parses /etc/passwd into a {uid -> username} map.
// One read per process_list call (the map is held only across the
// inner read_one_process loop). Best-effort — a missing /etc/passwd
// (containers without a real userdb) leaves user fields as numeric
// uid strings.
fn read_passwd_map() -> HashMap<u32, String> {
    let mut out = HashMap::new();
    let s = match read_string("/etc/passwd") {
        Some(v) => v,
        None => return out,
    };
    for line in s.lines() {
        let mut parts = line.split(':');
        let name = parts.next().unwrap_or("");
        let _x = parts.next();
        let uid_str = parts.next().unwrap_or("");
        if let Ok(uid) = uid_str.parse::<u32>() {
            out.insert(uid, name.to_string());
        }
    }
    out
}

fn read_string(path: &str) -> Option<String> {
    let env: Envelope = unsafe { host_fs_read(path.to_string()).ok()?.0 };
    if !env.ok {
        return None;
    }
    env.data.as_str().map(|s| s.to_string())
}
