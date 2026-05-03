// sys-info forwards SysInfo requests to the agent's gopsutil-backed
// CollectSysInfo via host_collect_sysinfo. Same shape as sys-procs:
// the data collection is too platform-specific (CPU/mem/disk/GPU/net
// branches) to reimplement under wasm; the plugin owns the dispatch.
//
// Capability: sysinfo.

use extism_pdk::*;
use serde::Deserialize;

#[host_fn("platypus")]
extern "ExtismHost" {
    fn host_collect_sysinfo() -> Json<Envelope>;
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
pub fn sys_info(_: ()) -> FnResult<String> {
    let env: Envelope = unsafe { host_collect_sysinfo()?.0 };
    if !env.ok {
        return Ok(format!(r#"{{"error":{}}}"#, serde_json::to_string(&env.error)?));
    }
    Ok(env.data.to_string())
}
