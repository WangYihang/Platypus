// sys-security forwards SecurityScan + ListSecurityChecks to the
// agent's hardening-check registry behind host_security_scan /
// host_list_security_checks. Forwarding shape — same as sys-procs /
// sys-info — because the check definitions span hundreds of LOC of
// platform-specific Go that's not worth porting under wasm today.
//
// Capability: sysinfo (observation only).

use extism_pdk::*;
use serde::Deserialize;

#[host_fn("platypus")]
extern "ExtismHost" {
    fn host_security_scan(req: String) -> Json<Envelope>;
    fn host_list_security_checks() -> Json<Envelope>;
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
pub fn security_scan(req: String) -> FnResult<String> {
    // Forward request bytes (already protojson) straight through; the
    // host fn parses + dispatches.
    let env: Envelope = unsafe { host_security_scan(req)?.0 };
    if !env.ok {
        return Ok(format!(r#"{{"error":{}}}"#, serde_json::to_string(&env.error)?));
    }
    Ok(env.data.to_string())
}

#[plugin_fn]
pub fn list_security_checks(_: ()) -> FnResult<String> {
    let env: Envelope = unsafe { host_list_security_checks()?.0 };
    if !env.ok {
        return Ok(format!(r#"{{"error":{}}}"#, serde_json::to_string(&env.error)?));
    }
    Ok(env.data.to_string())
}
