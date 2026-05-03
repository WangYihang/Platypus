// sys-exec is the plugin-ified replacement for the agent's built-in
// Exec RPC. Receives a JSON ExecRequest, calls host_exec with the
// same JSON, and returns the JSON ExecResponse the host wraps.
//
// Capability: exec with commands=["*"] — the system plugin gets the
// unrestricted-exec posture matching legacy HandleExec. Third-party
// plugins should declare a narrow command list instead; the agent's
// install dialog calls out a "*" entry prominently so it doesn't
// get rubber-stamped.

use extism_pdk::*;
use serde::{Deserialize, Serialize};
use std::collections::HashMap;

#[derive(Deserialize, Serialize, Default)]
pub struct ExecRequest {
    pub command: String,
    #[serde(default)]
    pub args: Vec<String>,
    #[serde(default)]
    pub env: HashMap<String, String>,
    #[serde(default)]
    pub cwd: String,
    #[serde(default)]
    pub timeout_ms: u32,
}

#[derive(Serialize, Deserialize, Default)]
pub struct ExecResponse {
    #[serde(default)]
    pub exit_code: i32,
    #[serde(default)]
    pub stdout: String,
    #[serde(default)]
    pub stderr: String,
    #[serde(default)]
    pub error: String,
}

#[host_fn("platypus")]
extern "ExtismHost" {
    fn host_exec(req: String) -> Json<Envelope>;
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
pub fn exec(req: Json<ExecRequest>) -> FnResult<Json<ExecResponse>> {
    let body = serde_json::to_string(&req.0)?;
    let env: Envelope = unsafe { host_exec(body)?.0 };
    if !env.ok {
        return Ok(Json(ExecResponse {
            error: env.error,
            ..Default::default()
        }));
    }
    let resp: ExecResponse = serde_json::from_value(env.data)?;
    Ok(Json(resp))
}
