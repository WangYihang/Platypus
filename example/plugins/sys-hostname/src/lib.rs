// sys-hostname v2 — reads /etc/hostname (with /proc fallback) via
// host_fs_read directly. The previous version went through the
// agent's host_sysinfo helper, which has been deleted now that the
// fully-self-contained sys-info plugin makes the small "just give
// me hostname" host fn redundant.
//
// Capability: fs.read of /etc + /proc/sys/kernel.

use extism_pdk::*;
use serde::{Deserialize, Serialize};

#[host_fn("platypus")]
extern "ExtismHost" {
    fn host_fs_read(path: String) -> Json<Envelope>;
}

#[derive(Deserialize)]
struct Envelope {
    ok: bool,
    #[serde(default)]
    data: serde_json::Value,
    #[serde(default)]
    error: String,
}

#[derive(Serialize, Deserialize)]
pub struct HostnameResponse {
    pub hostname: String,
    /// Path that produced the value — useful in audit logs to tell
    /// "/etc/hostname is set" apart from "had to fall back to
    /// /proc/sys/kernel/hostname". The legacy v1 plugin set this to
    /// "host_sysinfo"; v2 records the actual filesystem path read.
    pub source: String,
}

#[plugin_fn]
pub fn hostname(_: ()) -> FnResult<Json<HostnameResponse>> {
    // Order: /etc/hostname is the canonical source on every modern
    // distro; fall back to /proc/sys/kernel/hostname when the file
    // is missing (rare but legitimate on minimal containers).
    for path in &["/etc/hostname", "/proc/sys/kernel/hostname"] {
        if let Some(v) = read_trim(path) {
            return Ok(Json(HostnameResponse {
                hostname: v,
                source: path.to_string(),
            }));
        }
    }
    // Both reads failed; surface the failure as an empty hostname +
    // a "source: none" marker so the bridge sees a valid response
    // rather than a wasm trap.
    Ok(Json(HostnameResponse {
        hostname: String::new(),
        source: "none".to_string(),
    }))
}

fn read_trim(path: &str) -> Option<String> {
    let env: Envelope = unsafe { host_fs_read(path.to_string()).ok()?.0 };
    if !env.ok {
        return None;
    }
    let s = env.data.as_str()?;
    let trimmed = s.trim();
    if trimmed.is_empty() {
        None
    } else {
        Some(trimmed.to_string())
    }
}
