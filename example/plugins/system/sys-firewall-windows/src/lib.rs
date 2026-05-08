// sys-firewall-windows — Windows firewall-rule inventory via
// Get-NetFirewallRule + the per-rule filter cmdlets.
//
// PowerShell pipeline: for each NetFirewallRule object, also fetch
// its associated PortFilter (proto + LocalPort + RemotePort) and
// AddressFilter (LocalAddress + RemoteAddress). Combine into a flat
// FirewallRule shape.

use extism_pdk::*;
use serde::{Deserialize, Serialize};

#[cfg(target_arch = "wasm32")]
#[host_fn("platypus")]
extern "ExtismHost" {
    fn host_exec(req: String) -> Json<Envelope>;
}

#[derive(Deserialize, Default)]
struct Envelope {
    ok: bool,
    #[serde(default)]
    data: serde_json::Value,
    #[serde(default)]
    error: String,
}

#[derive(Serialize)]
struct ExecRequest {
    command: String,
    args: Vec<String>,
    #[serde(rename = "timeout_ms")]
    timeout_ms: u32,
}

#[derive(Deserialize, Default)]
struct ExecResponse {
    #[serde(default, rename = "exit_code")]
    exit_code: i32,
    #[serde(default)]
    stdout: String,
    #[serde(default)]
    stderr: String,
}

// ---- request / response ----

#[derive(Deserialize, Default)]
struct ListRequest {
    #[serde(default)]
    include_disabled: bool,
    #[serde(default)]
    filter: String,
}

#[derive(Serialize, Default)]
struct ListResponse {
    rules: Vec<FirewallRule>,
    backend: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    error: String,
}

#[derive(Serialize, Deserialize, Default, Debug, PartialEq)]
#[serde(default)]
struct FirewallRule {
    #[serde(skip_serializing_if = "String::is_empty")]
    id: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    name: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    direction: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    action: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    protocol: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    src: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    dst: String,
    #[serde(rename = "srcPort", skip_serializing_if = "String::is_empty")]
    src_port: String,
    #[serde(rename = "dstPort", skip_serializing_if = "String::is_empty")]
    dst_port: String,
    enabled: bool,
    #[serde(rename = "interface", skip_serializing_if = "String::is_empty")]
    interface_: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    profile: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    raw: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    chain: String,
}

// ---- entry point ----

const PS_SCRIPT: &str = r#"[Console]::OutputEncoding = [System.Text.Encoding]::UTF8;
$rules = Get-NetFirewallRule -ErrorAction SilentlyContinue | ForEach-Object {
    $r = $_
    $portFilter = $null
    $addrFilter = $null
    try { $portFilter = $r | Get-NetFirewallPortFilter -ErrorAction SilentlyContinue } catch {}
    try { $addrFilter = $r | Get-NetFirewallAddressFilter -ErrorAction SilentlyContinue } catch {}
    [pscustomobject]@{
        id          = [string]$r.InstanceID
        name        = [string]$r.DisplayName
        direction   = if ($r.Direction -eq 'Inbound') { 'in' } elseif ($r.Direction -eq 'Outbound') { 'out' } else { '' }
        action      = if ($r.Action -eq 'Allow') { 'allow' } elseif ($r.Action -eq 'Block') { 'deny' } else { [string]$r.Action }
        protocol    = if ($portFilter -ne $null) { ([string]$portFilter.Protocol).ToLower() } else { '' }
        src         = if ($addrFilter -ne $null) { ($addrFilter.RemoteAddress -join ',') } else { '' }
        dst         = if ($addrFilter -ne $null) { ($addrFilter.LocalAddress -join ',') } else { '' }
        srcPort     = if ($portFilter -ne $null) { ($portFilter.RemotePort -join ',') } else { '' }
        dstPort     = if ($portFilter -ne $null) { ($portFilter.LocalPort -join ',') } else { '' }
        enabled     = ($r.Enabled -eq 'True' -or $r.Enabled -eq $true)
        profile     = [string]$r.Profile
        raw         = '<NetFirewallRule>'
        chain       = ''
    }
}
$rules | ConvertTo-Json -Compress -Depth 4"#;

#[cfg(target_arch = "wasm32")]
#[plugin_fn]
pub fn list_firewall_rules(req: Json<ListRequest>) -> FnResult<String> {
    let r = req.0;
    let exec_resp = match run_powershell(PS_SCRIPT, 60_000) {
        Ok(v) => v,
        Err(e) => {
            return Ok(serde_json::to_string(&ListResponse {
                rules: Vec::new(),
                backend: String::new(),
                error: e,
            })?)
        }
    };
    if exec_resp.exit_code != 0 {
        return Ok(serde_json::to_string(&ListResponse {
            rules: Vec::new(),
            backend: String::new(),
            error: format!(
                "powershell exit {}: {}",
                exec_resp.exit_code,
                exec_resp.stderr.trim()
            ),
        })?);
    }
    let parsed = parse_powershell_output(&exec_resp.stdout);
    let rules = filter_rules(parsed, &r);
    Ok(serde_json::to_string(&ListResponse {
        rules,
        backend: "windows-firewall".to_string(),
        error: String::new(),
    })?)
}

// ---- pure helpers ----

fn parse_powershell_output(stdout: &str) -> Vec<FirewallRule> {
    let trimmed = stdout.trim();
    if trimmed.is_empty() || trimmed == "null" {
        return Vec::new();
    }
    if let Ok(v) = serde_json::from_str::<Vec<FirewallRule>>(trimmed) {
        return v;
    }
    if let Ok(v) = serde_json::from_str::<FirewallRule>(trimmed) {
        return vec![v];
    }
    Vec::new()
}

fn filter_rules(rules: Vec<FirewallRule>, r: &ListRequest) -> Vec<FirewallRule> {
    let needle = r.filter.to_ascii_lowercase();
    rules
        .into_iter()
        .filter(|rule| {
            if !r.include_disabled && !rule.enabled {
                return false;
            }
            if !needle.is_empty()
                && !rule.name.to_ascii_lowercase().contains(&needle)
                && !rule.id.to_ascii_lowercase().contains(&needle)
            {
                return false;
            }
            true
        })
        .collect()
}

// ---- exec helper ----

#[cfg(target_arch = "wasm32")]
fn run_powershell(script: &str, timeout_ms: u32) -> Result<ExecResponse, String> {
    let req = ExecRequest {
        command: "C:\\Windows\\System32\\WindowsPowerShell\\v1.0\\powershell.exe"
            .to_string(),
        args: vec![
            "-NoProfile".to_string(),
            "-NonInteractive".to_string(),
            "-OutputFormat".to_string(),
            "Text".to_string(),
            "-Command".to_string(),
            script.to_string(),
        ],
        timeout_ms,
    };
    let body = serde_json::to_string(&req).map_err(|e| format!("encode_exec_req: {e}"))?;
    let env: Envelope = unsafe {
        host_exec(body).map_err(|e| format!("host_exec: {e}"))?.0
    };
    if !env.ok {
        return Err(env.error);
    }
    serde_json::from_value(env.data).map_err(|e| format!("decode_exec_resp: {e}"))
}

#[cfg(not(target_arch = "wasm32"))]
#[allow(dead_code)]
fn run_powershell(_script: &str, _timeout_ms: u32) -> Result<ExecResponse, String> {
    Err("not available on host build".to_string())
}

// ============================================================
// Pure-function unit tests
// ============================================================
#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parse_array_two_rules() {
        let json = r#"[
            {"id":"AllowSSH","name":"Allow SSH","direction":"in","action":"allow",
             "protocol":"tcp","dst":"any","dstPort":"22","enabled":true,"profile":"Domain,Private"},
            {"id":"BlockTelnet","name":"Block Telnet","direction":"in","action":"deny",
             "protocol":"tcp","dstPort":"23","enabled":true,"profile":"Public"}
        ]"#;
        let rules = parse_powershell_output(json);
        assert_eq!(rules.len(), 2);
        assert_eq!(rules[0].name, "Allow SSH");
        assert_eq!(rules[0].dst_port, "22");
        assert_eq!(rules[1].action, "deny");
    }

    #[test]
    fn parse_single_object() {
        let json = r#"{"id":"X","name":"Solo","direction":"out","action":"allow","enabled":true}"#;
        let rules = parse_powershell_output(json);
        assert_eq!(rules.len(), 1);
        assert_eq!(rules[0].direction, "out");
    }

    #[test]
    fn parse_empty() {
        assert!(parse_powershell_output("").is_empty());
        assert!(parse_powershell_output("null").is_empty());
    }

    #[test]
    fn filter_skips_disabled_by_default() {
        let rules = vec![
            FirewallRule { name: "On".to_string(), enabled: true, ..Default::default() },
            FirewallRule { name: "Off".to_string(), enabled: false, ..Default::default() },
        ];
        let req = ListRequest::default();
        let out = filter_rules(rules, &req);
        assert_eq!(out.len(), 1);
        assert_eq!(out[0].name, "On");
    }

    #[test]
    fn filter_by_name_substring() {
        let rules = vec![
            FirewallRule { name: "Skype".to_string(), enabled: true, ..Default::default() },
            FirewallRule { name: "Docker Desktop".to_string(), enabled: true, ..Default::default() },
            FirewallRule { id: "DOCKER-USER".to_string(), name: "".to_string(), enabled: true, ..Default::default() },
        ];
        let req = ListRequest { filter: "docker".to_string(), include_disabled: false };
        let out = filter_rules(rules, &req);
        assert_eq!(out.len(), 2);
    }
}
