// sys-config-audit forwards ConfigAudit + ListConfigAuditors to the
// agent's gitleaks-backed credential-audit registry behind
// host_config_audit / host_list_config_auditors. Same forwarding
// shape as sys-security.
//
// Capability: sysinfo (observation only — secrets are redacted by
// the host before reaching the plugin).

use extism_pdk::*;
use serde::Deserialize;

#[host_fn("platypus")]
extern "ExtismHost" {
    fn host_config_audit(req: String) -> Json<Envelope>;
    fn host_list_config_auditors() -> Json<Envelope>;
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
pub fn config_audit(req: String) -> FnResult<String> {
    let env: Envelope = unsafe { host_config_audit(req)?.0 };
    if !env.ok {
        return Ok(format!(r#"{{"error":{}}}"#, serde_json::to_string(&env.error)?));
    }
    Ok(env.data.to_string())
}

#[plugin_fn]
pub fn list_config_auditors(_: ()) -> FnResult<String> {
    let env: Envelope = unsafe { host_list_config_auditors()?.0 };
    if !env.ok {
        return Ok(format!(r#"{{"error":{}}}"#, serde_json::to_string(&env.error)?));
    }
    Ok(env.data.to_string())
}
