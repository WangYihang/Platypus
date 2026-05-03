// sys-procs is a thin forwarding plugin: receives a ProcessList JSON
// request, hands it to host_process_list (which wraps the agent's
// gopsutil-backed CollectProcessList), and returns the JSON-encoded
// proto response untouched.
//
// Why thin: gopsutil's enumerate logic is hundreds of LOC of platform-
// specific syscalls + /proc parsing. Reimplementing in Rust + WASI
// would be a major project of its own; for now the plugin owns the
// dispatch boundary while the agent retains the data-collection
// implementation. Migrating the data collection itself is a separate
// future iteration once we have a Rust process-info crate that works
// in wasm32-unknown-unknown.
//
// Capability: sysinfo. Same posture as sys-hostname.

use extism_pdk::*;
use serde::{Deserialize, Serialize};

#[derive(Deserialize)]
pub struct ProcessListRequest {
    #[serde(default)]
    pub top_n: u32,
    #[serde(default)]
    pub sort_by: String,
}

/// host_process_list returns the JSON envelope with `data` set to the
/// protojson encoding of v2pb.ProcessListResponse. We forward `data`
/// straight back to the bridge; the bridge re-decodes via protojson
/// into the typed proto.
#[host_fn("platypus")]
extern "ExtismHost" {
    fn host_process_list(req: String) -> Json<Envelope>;
}

#[derive(Deserialize)]
struct Envelope {
    ok: bool,
    #[serde(default)]
    data: serde_json::Value,
    #[serde(default)]
    error: String,
}

#[derive(Serialize)]
struct ForwardRequest<'a> {
    top_n: u32,
    sort_by: &'a str,
}

#[plugin_fn]
pub fn process_list(req: Json<ProcessListRequest>) -> FnResult<String> {
    let body = serde_json::to_string(&ForwardRequest {
        top_n: req.0.top_n,
        sort_by: &req.0.sort_by,
    })?;
    let env: Envelope = unsafe { host_process_list(body)?.0 };
    if !env.ok {
        // Pass-through error: the bridge maps the JSON to
        // ProcessListResponse{error: ...}.
        return Ok(format!(r#"{{"error":{}}}"#, serde_json::to_string(&env.error)?));
    }
    // Forward the protojson-encoded ProcessListResponse straight
    // through. The bridge will decode it.
    Ok(env.data.to_string())
}
