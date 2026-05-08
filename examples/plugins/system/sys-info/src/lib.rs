// sys-info v2 — real implementation. Reads /proc + /etc + /sys
// directly via host_fs_read and assembles a SysInfoResponse-shaped
// JSON. The agent-side host_collect_sysinfo + the gopsutil-backed
// CollectSysInfo are no longer in the path; this plugin owns the
// data collection.
//
// Capability: sysinfo (for host_uname) + fs.read of /proc, /etc,
// /sys (for the file reads). Operator implicitly trusts the system
// plugin's allowlist.
//
// Coverage: covers the basics every fleet inventory needs (os, arch,
// hostname, kernel, mem, cpu count, cpu model, uptime, load,
// process count, platform/distro, machine id, timezone). Advanced
// fields that gopsutil computed (cpu_percent, per-disk usage, GPU,
// virtualization detection, network MACs, public IP) are left at
// zero/empty for v2 — the wire schema documents them as optional and
// the UI renders missing as "—". Future plugin versions can fill
// each one as Rust /proc parsing is added.

use extism_pdk::*;
use serde::{Deserialize, Serialize};

// ---------- host fn bindings ----------

#[cfg(target_arch = "wasm32")]
#[host_fn("platypus")]
extern "ExtismHost" {
    fn host_uname() -> Json<Envelope>;
    fn host_fs_read(path: String) -> Json<Envelope>;
    fn host_fs_listdir(path: String) -> Json<Envelope>;
}

#[derive(Deserialize, Default)]
struct UnameJSON {
    #[serde(default)]
    os: String,
    #[serde(default)]
    arch: String,
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

// ---------- response shape ----------

// SysInfoResponse mirrors the protojson encoding of v2pb.SysInfoResponse.
// Only fields v2 fills are listed — protojson tolerates missing
// optional fields and the bridge unmarshalls leniently. Camel-case
// names match protojson's default convention.
//
// SAFE TO ADD: new field with #[serde(skip_serializing_if=…)] when
// v3 fills another /proc-derived value. Existing fields stay stable.
#[derive(Serialize, Default)]
struct SysInfoResponse {
    #[serde(skip_serializing_if = "String::is_empty")]
    os: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    arch: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    hostname: String,
    #[serde(rename = "kernelVersion", skip_serializing_if = "String::is_empty")]
    kernel_version: String,

    #[serde(rename = "memTotal", skip_serializing_if = "is_zero_u64")]
    mem_total: u64,
    #[serde(rename = "memUsed", skip_serializing_if = "is_zero_u64")]
    mem_used: u64,
    #[serde(rename = "memAvailable", skip_serializing_if = "is_zero_u64")]
    mem_available: u64,
    #[serde(rename = "memFree", skip_serializing_if = "is_zero_u64")]
    mem_free: u64,
    #[serde(rename = "swapTotal", skip_serializing_if = "is_zero_u64")]
    swap_total: u64,
    #[serde(rename = "swapUsed", skip_serializing_if = "is_zero_u64")]
    swap_used: u64,

    #[serde(skip_serializing_if = "String::is_empty")]
    platform: String,
    #[serde(rename = "platformFamily", skip_serializing_if = "String::is_empty")]
    platform_family: String,
    #[serde(rename = "platformVersion", skip_serializing_if = "String::is_empty")]
    platform_version: String,

    #[serde(rename = "numCpu", skip_serializing_if = "is_zero_u32")]
    num_cpu: u32,
    #[serde(rename = "cpuModel", skip_serializing_if = "String::is_empty")]
    cpu_model: String,
    #[serde(rename = "cpuMhz", skip_serializing_if = "is_zero_f64")]
    cpu_mhz: f64,

    #[serde(rename = "bootTimeUnix", skip_serializing_if = "is_zero_u64")]
    boot_time_unix: u64,
    #[serde(rename = "uptimeSeconds", skip_serializing_if = "is_zero_u64")]
    uptime_seconds: u64,

    #[serde(skip_serializing_if = "is_zero_f64")]
    load1: f64,
    #[serde(skip_serializing_if = "is_zero_f64")]
    load5: f64,
    #[serde(skip_serializing_if = "is_zero_f64")]
    load15: f64,

    #[serde(rename = "processCount", skip_serializing_if = "is_zero_u32")]
    process_count: u32,

    #[serde(rename = "machineId", skip_serializing_if = "String::is_empty")]
    machine_id: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    timezone: String,

    #[serde(rename = "sampledAtUnix", skip_serializing_if = "is_zero_i64")]
    sampled_at_unix: i64,

    #[serde(skip_serializing_if = "String::is_empty")]
    error: String,
}

fn is_zero_u32(x: &u32) -> bool { *x == 0 }
fn is_zero_u64(x: &u64) -> bool { *x == 0 }
fn is_zero_i64(x: &i64) -> bool { *x == 0 }
fn is_zero_f64(x: &f64) -> bool { *x == 0.0 }

// ---------- entry point ----------

#[cfg(target_arch = "wasm32")]
#[plugin_fn]
pub fn sys_info(_: ()) -> FnResult<String> {
    let mut resp = SysInfoResponse::default();

    // Cheap primitives via host_uname. Comes back as the standard
    // {ok,data,error} envelope; data is a JSON object with os/arch
    // fields.
    if let Ok(env_json) = unsafe { host_uname() } {
        let env: Envelope = env_json.0;
        if env.ok {
            if let Ok(u) = serde_json::from_value::<UnameJSON>(env.data) {
                resp.os = u.os;
                resp.arch = u.arch;
            }
        }
    }

    // Trivially-readable single-value files. read_trim returns None
    // on missing/denied so each is independent — a denied path
    // doesn't blank out the whole response.
    if let Some(s) = read_trim("/etc/hostname") {
        resp.hostname = s;
    } else if let Some(s) = read_trim("/proc/sys/kernel/hostname") {
        resp.hostname = s;
    }

    if let Some(s) = read_trim("/proc/sys/kernel/osrelease") {
        resp.kernel_version = s;
    } else if let Some(s) = read_trim("/proc/version") {
        // Fallback: extract second whitespace-delimited word.
        resp.kernel_version = s.split_whitespace().nth(2).unwrap_or("").to_string();
    }

    if let Some(s) = read_trim("/etc/machine-id") {
        resp.machine_id = s;
    }

    if let Some(s) = read_trim("/etc/timezone") {
        resp.timezone = s;
    }

    // /etc/os-release: KEY=value lines, possibly quoted.
    if let Some(s) = read_string("/etc/os-release") {
        let (id, id_like, version_id) = parse_os_release(&s);
        resp.platform = id;
        resp.platform_family = id_like;
        resp.platform_version = version_id;
    }

    // /proc/meminfo: "Key: 12345 kB" lines. Values in KiB → multiply
    // by 1024 for bytes (matches the gopsutil-emitted shape).
    if let Some(s) = read_string("/proc/meminfo") {
        for line in s.lines() {
            let mut parts = line.split_whitespace();
            let key = parts.next().unwrap_or("");
            let value: u64 = parts.next().and_then(|v| v.parse().ok()).unwrap_or(0);
            let bytes = value.saturating_mul(1024);
            match key {
                "MemTotal:" => resp.mem_total = bytes,
                "MemFree:" => resp.mem_free = bytes,
                "MemAvailable:" => resp.mem_available = bytes,
                "SwapTotal:" => resp.swap_total = bytes,
                "SwapFree:" => {
                    // SwapUsed = SwapTotal - SwapFree, computed below
                    // once both are read. Park SwapFree in mem_used
                    // temporarily — replaced before serialisation.
                    resp.mem_used = bytes; // reusing slot; replaced below
                }
                _ => {}
            }
        }
        // Compute used from total - available (matches gopsutil's
        // "used" definition: total - available, NOT total - free).
        if resp.mem_total > 0 && resp.mem_available > 0 {
            resp.mem_used = resp.mem_total.saturating_sub(resp.mem_available);
        } else if resp.mem_total > 0 && resp.mem_free > 0 {
            resp.mem_used = resp.mem_total.saturating_sub(resp.mem_free);
        } else {
            resp.mem_used = 0; // unset
        }
        // SwapUsed = SwapTotal - SwapFree. We re-read SwapFree because
        // we stored it incorrectly above.
        if let Some(swap_free) = scan_kib(&s, "SwapFree:") {
            resp.swap_used = resp.swap_total.saturating_sub(swap_free);
        }
    }

    // /proc/uptime: "<uptime> <idle>" — both as float seconds.
    if let Some(s) = read_string("/proc/uptime") {
        resp.uptime_seconds = parse_uptime(&s);
    }

    // /proc/loadavg: "1.0 1.0 1.0 ..."
    if let Some(s) = read_string("/proc/loadavg") {
        let (l1, l5, l15) = parse_loadavg(&s);
        resp.load1 = l1;
        resp.load5 = l5;
        resp.load15 = l15;
    }

    // /proc/cpuinfo: count "processor" occurrences (logical cores)
    // + capture the first "model name".
    if let Some(s) = read_string("/proc/cpuinfo") {
        let mut count: u32 = 0;
        for line in s.lines() {
            if line.starts_with("processor") {
                count += 1;
            } else if resp.cpu_model.is_empty() && line.starts_with("model name") {
                if let Some((_, v)) = line.split_once(':') {
                    resp.cpu_model = v.trim().to_string();
                }
            } else if resp.cpu_mhz == 0.0 && line.starts_with("cpu MHz") {
                if let Some((_, v)) = line.split_once(':') {
                    resp.cpu_mhz = v.trim().parse().unwrap_or(0.0);
                }
            }
        }
        resp.num_cpu = count;
    }

    // /proc/<pid> entries → process count.
    if let Some(entries) = list_dir("/proc") {
        let mut count: u32 = 0;
        for e in entries {
            if e.is_dir && e.name.chars().all(|c| c.is_ascii_digit()) {
                count += 1;
            }
        }
        resp.process_count = count;
    }

    // protojson serialiser. The bridge wrapper unmarshals via
    // protojson.Unmarshal so we just need camelCase keys + the
    // SysInfoResponse field set; missing fields stay default.
    Ok(serde_json::to_string(&resp)?)
}

// ---------- /proc helpers ----------

#[cfg(target_arch = "wasm32")]
fn read_string(path: &str) -> Option<String> {
    let env: Envelope = unsafe { host_fs_read(path.to_string()).ok()?.0 };
    if !env.ok {
        return None;
    }
    // host_fs_read returns Data as a JSON string (the file's
    // contents), so the JSON value is a String node.
    env.data.as_str().map(|s| s.to_string())
}

#[cfg(target_arch = "wasm32")]
fn read_trim(path: &str) -> Option<String> {
    read_string(path).map(|s| s.trim().to_string()).filter(|s| !s.is_empty())
}

#[cfg(target_arch = "wasm32")]
fn list_dir(path: &str) -> Option<Vec<DirEntryJSON>> {
    let env: Envelope = unsafe { host_fs_listdir(path.to_string()).ok()?.0 };
    if !env.ok {
        return None;
    }
    serde_json::from_value(env.data).ok()
}

// scan_kib finds a "<key>: <number> kB" line in /proc/meminfo-style
// content and returns the value in bytes. Used for the SwapFree
// re-read after we accidentally clobbered mem_used.
fn scan_kib(content: &str, key: &str) -> Option<u64> {
    for line in content.lines() {
        let mut parts = line.split_whitespace();
        if parts.next() == Some(key) {
            if let Some(v) = parts.next() {
                return v.parse::<u64>().ok().map(|n| n.saturating_mul(1024));
            }
        }
    }
    None
}

// parse_os_release extracts (ID, ID_LIKE, VERSION_ID) from /etc/os-release
// content. Lines look like `KEY=value` or `KEY="value"`. Missing keys
// come back as empty strings — every distro uses a different subset.
fn parse_os_release(content: &str) -> (String, String, String) {
    let mut id = String::new();
    let mut id_like = String::new();
    let mut version_id = String::new();
    for line in content.lines() {
        if let Some((k, v)) = line.split_once('=') {
            let v = v.trim_matches('"').to_string();
            match k {
                "ID" => id = v,
                "ID_LIKE" => id_like = v,
                "VERSION_ID" => version_id = v,
                _ => {}
            }
        }
    }
    (id, id_like, version_id)
}

// parse_uptime returns the integer-second count from /proc/uptime
// content ("<float-seconds> <idle-float-seconds>\n"). Malformed
// input or empty file yields 0.
fn parse_uptime(content: &str) -> u64 {
    let secs: f64 = content
        .split_whitespace()
        .next()
        .and_then(|v| v.parse().ok())
        .unwrap_or(0.0);
    if secs > 0.0 { secs as u64 } else { 0 }
}

// parse_loadavg returns (load1, load5, load15) from /proc/loadavg
// content ("1.00 1.50 2.00 ...\n"). Missing or unparsable fields
// fall through to 0.0.
fn parse_loadavg(content: &str) -> (f64, f64, f64) {
    let mut parts = content.split_whitespace();
    let l1 = parts.next().and_then(|v| v.parse().ok()).unwrap_or(0.0);
    let l5 = parts.next().and_then(|v| v.parse().ok()).unwrap_or(0.0);
    let l15 = parts.next().and_then(|v| v.parse().ok()).unwrap_or(0.0);
    (l1, l5, l15)
}

// ============================================================
// Pure-function unit tests (host build, not wasm)
// ============================================================
//
// /proc parsers can run on host fixtures — no host_fn needed. The
// wasm-only top-level sys_info entry is exercised end-to-end by
// internal/agent/plugin/sys_info_integration_test.go.
#[cfg(test)]
mod tests {
    use super::*;

    // ---- scan_kib --------------------------------------------

    #[test]
    fn scan_kib_finds_key_and_converts_to_bytes() {
        let meminfo = "MemTotal:       16336872 kB\nMemFree:         1048576 kB\n";
        assert_eq!(scan_kib(meminfo, "MemTotal:"), Some(16336872 * 1024));
        assert_eq!(scan_kib(meminfo, "MemFree:"), Some(1048576 * 1024));
    }

    #[test]
    fn scan_kib_missing_key_returns_none() {
        assert_eq!(scan_kib("MemTotal: 100 kB\n", "Missing:"), None);
    }

    #[test]
    fn scan_kib_malformed_value_returns_none() {
        assert_eq!(scan_kib("Key: notanumber kB\n", "Key:"), None);
    }

    // ---- parse_os_release ------------------------------------

    #[test]
    fn parse_os_release_ubuntu() {
        let s = r#"NAME="Ubuntu"
VERSION="22.04.3 LTS (Jammy Jellyfish)"
ID=ubuntu
ID_LIKE=debian
PRETTY_NAME="Ubuntu 22.04.3 LTS"
VERSION_ID="22.04"
"#;
        let (id, id_like, version_id) = parse_os_release(s);
        assert_eq!(id, "ubuntu");
        assert_eq!(id_like, "debian");
        assert_eq!(version_id, "22.04");
    }

    #[test]
    fn parse_os_release_alpine_no_id_like() {
        let s = "NAME=\"Alpine Linux\"\nID=alpine\nVERSION_ID=3.18.4\n";
        let (id, id_like, version_id) = parse_os_release(s);
        assert_eq!(id, "alpine");
        assert_eq!(id_like, ""); // Alpine doesn't ship ID_LIKE
        assert_eq!(version_id, "3.18.4");
    }

    #[test]
    fn parse_os_release_handles_unquoted_values() {
        // Some distros emit ID without quotes; trim_matches('"') is a no-op then.
        let s = "ID=arch\n";
        let (id, _, _) = parse_os_release(s);
        assert_eq!(id, "arch");
    }

    #[test]
    fn parse_os_release_empty_input() {
        let (id, id_like, version_id) = parse_os_release("");
        assert_eq!(id, "");
        assert_eq!(id_like, "");
        assert_eq!(version_id, "");
    }

    // ---- parse_uptime ----------------------------------------

    #[test]
    fn parse_uptime_typical_format() {
        assert_eq!(parse_uptime("3600.42 1234.56\n"), 3600);
    }

    #[test]
    fn parse_uptime_empty_or_malformed() {
        assert_eq!(parse_uptime(""), 0);
        assert_eq!(parse_uptime("notanumber\n"), 0);
    }

    // ---- parse_loadavg ---------------------------------------

    #[test]
    fn parse_loadavg_three_values() {
        let (l1, l5, l15) = parse_loadavg("0.20 0.30 0.40 1/123 4567\n");
        assert!((l1 - 0.20).abs() < 0.001);
        assert!((l5 - 0.30).abs() < 0.001);
        assert!((l15 - 0.40).abs() < 0.001);
    }

    #[test]
    fn parse_loadavg_missing_fields_default_zero() {
        let (l1, l5, l15) = parse_loadavg("1.5\n");
        assert!((l1 - 1.5).abs() < 0.001);
        assert_eq!(l5, 0.0);
        assert_eq!(l15, 0.0);
    }

    #[test]
    fn parse_loadavg_empty_input() {
        let (l1, l5, l15) = parse_loadavg("");
        assert_eq!(l1, 0.0);
        assert_eq!(l5, 0.0);
        assert_eq!(l15, 0.0);
    }
}
