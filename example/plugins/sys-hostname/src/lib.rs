// First Platypus system plugin — proves the bundled bootstrap path
// end-to-end with a real Rust extism artefact.
//
// What it does: one method `hostname` that reads the host's
// hostname via the agent's `host_sysinfo` host function and returns
// it. Tiny on purpose; the migration of the larger SysInfo /
// ProcessList / etc. handlers builds on the same shape but at
// significantly more complexity (parsing /proc files, etc.).
//
// Sandbox posture: declares only the implicit `log` capability +
// `sysinfo` (granted automatically because system plugins run with
// every capability they declare). No fs.read / exec / net.http.

use extism_pdk::*;
use serde::{Deserialize, Serialize};

/// HostnameResponse is the schema operators see over the wire. Mirrors
/// the shape we'll eventually return from the migrated SysInfo
/// handler; today only `hostname` is populated.
#[derive(Serialize, Deserialize)]
pub struct HostnameResponse {
    pub hostname: String,
    /// Source records which path produced the value. "host_sysinfo"
    /// today; once we add more host functions (e.g. host_fs_read of
    /// /etc/hostname), this field lets the operator audit which
    /// primitive was used.
    pub source: String,
}

/// host_sysinfo lives in the `platypus` namespace (see
/// internal/agent/plugin/host_funcs.go's `hostNamespace` constant).
/// The default extism PDK namespace is `extism:host/user`, so we
/// explicitly retarget here. Mismatched namespaces show up as
/// "module not instantiated" at extism.NewPlugin time.
#[host_fn("platypus")]
extern "ExtismHost" {
    fn host_sysinfo() -> Json<Envelope>;
}

#[derive(Deserialize)]
struct Envelope {
    ok: bool,
    #[serde(default)]
    data: serde_json::Value,
    #[serde(default)]
    error: String,
}

#[plugin_fn]
pub fn hostname(_: ()) -> FnResult<Json<HostnameResponse>> {
    let env: Envelope = unsafe { host_sysinfo()?.0 };
    if !env.ok {
        return Err(WithReturnCode::new(
            Error::msg(format!("host_sysinfo: {}", env.error)),
            1,
        ));
    }
    let hostname = env
        .data
        .get("hostname")
        .and_then(|v| v.as_str())
        .unwrap_or("")
        .to_string();
    Ok(Json(HostnameResponse {
        hostname,
        source: "host_sysinfo".to_string(),
    }))
}
