// sys-firewall-darwin — macOS pf rule inventory via `pfctl -sr`.
//
// pf rule grammar (excerpts; full spec is `man 5 pf.conf`):
//   pass in proto tcp from any to any port = 22
//   block return-rst in proto tcp from any to any port = 23 keep state
//   pass on lo0 all
//   block drop in quick on ! lo0 inet6 proto tcp from any to ::1
//
// We parse the most common shape (action + direction + proto + port
// + endpoints) per rule and stash the original line in `raw` for
// fidelity. pf's quick / log / tag / state / scrub flags surface in
// raw only.

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
    #[serde(default)]
    offset: u32,
    #[serde(default)]
    limit: u32,
}

#[derive(Serialize, Default)]
struct ListResponse {
    rules: Vec<FirewallRule>,
    backend: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    error: String,
    #[serde(rename = "totalCount", skip_serializing_if = "is_zero_u32")]
    total_count: u32,
    #[serde(rename = "hasMore", skip_serializing_if = "is_false")]
    has_more: bool,
}

fn is_zero_u32(n: &u32) -> bool { *n == 0 }
fn is_false(b: &bool) -> bool { !*b }

const DEFAULT_LIMIT: u32 = 200;
const HARD_LIMIT: u32 = 5_000;

fn effective_limit(requested: u32) -> u32 {
    let n = if requested == 0 { DEFAULT_LIMIT } else { requested };
    n.min(HARD_LIMIT)
}

fn paginate<T>(items: Vec<T>, offset: u32, limit: u32) -> (Vec<T>, u32, bool) {
    let total = items.len() as u32;
    let off = (offset as usize).min(items.len());
    let lim = effective_limit(limit) as usize;
    let end = (off + lim).min(items.len());
    let slice: Vec<T> = items.into_iter().skip(off).take(end - off).collect();
    let has_more = (off + slice.len()) < total as usize;
    (slice, total, has_more)
}

#[derive(Serialize, Default, Debug, PartialEq)]
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

#[cfg(target_arch = "wasm32")]
#[plugin_fn]
pub fn list_firewall_rules(req: Json<ListRequest>) -> FnResult<String> {
    let r = req.0;
    let exec_resp = match run_pfctl(15_000) {
        Ok(v) => v,
        Err(e) => {
            return Ok(serde_json::to_string(&ListResponse {
                rules: Vec::new(),
                backend: String::new(),
                error: e,
                ..Default::default()
            })?)
        }
    };
    if exec_resp.exit_code != 0 {
        let lower = exec_resp.stderr.to_ascii_lowercase();
        let err = if lower.contains("operation not permitted")
            || lower.contains("permission denied")
        {
            "permission_denied".to_string()
        } else {
            format!(
                "pfctl exit {}: {}",
                exec_resp.exit_code,
                exec_resp.stderr.trim()
            )
        };
        return Ok(serde_json::to_string(&ListResponse {
            rules: Vec::new(),
            backend: String::new(),
            error: err,
            ..Default::default()
        })?);
    }
    let rules = parse_pfctl_output(&exec_resp.stdout);
    let needle = r.filter.to_ascii_lowercase();
    let filtered: Vec<FirewallRule> = rules
        .into_iter()
        .filter(|rule| needle.is_empty() || rule.raw.to_ascii_lowercase().contains(&needle))
        .collect();
    let (sliced, total, has_more) = paginate(filtered, r.offset, r.limit);
    Ok(serde_json::to_string(&ListResponse {
        rules: sliced,
        backend: "pf".to_string(),
        error: String::new(),
        total_count: total,
        has_more,
    })?)
}

// ---- pure parser ----

fn parse_pfctl_output(stdout: &str) -> Vec<FirewallRule> {
    let mut out = Vec::new();
    let mut idx = 0u32;
    for line in stdout.lines() {
        let trimmed = line.trim();
        if trimmed.is_empty() || trimmed.starts_with('#') {
            continue;
        }
        idx += 1;
        let mut rule = FirewallRule {
            id: format!("pf:{idx}"),
            enabled: true,
            raw: trimmed.to_string(),
            ..Default::default()
        };
        parse_pf_rule(trimmed, &mut rule);
        out.push(rule);
    }
    out
}

// parse_pf_rule walks the tokens of a single pf rule line.
// Recognised shapes:
//   <action> [direction] [quick] [on <iface>] [proto <proto>]
//     [from <src> [port <port>]] [to <dst> [port <port>]]
//     [keep state] [...]
fn parse_pf_rule(line: &str, rule: &mut FirewallRule) {
    let tokens: Vec<&str> = line.split_whitespace().collect();
    let mut i = 0;
    while i < tokens.len() {
        let t = tokens[i];
        match t {
            "pass" => rule.action = "allow".to_string(),
            "block" | "drop" => {
                if rule.action.is_empty() {
                    rule.action = "deny".to_string();
                }
            }
            "match" => {
                if rule.action.is_empty() {
                    rule.action = "match".to_string();
                }
            }
            "in" => rule.direction = "in".to_string(),
            "out" => rule.direction = "out".to_string(),
            "on" => {
                if i + 1 < tokens.len() {
                    rule.interface_ = tokens[i + 1].to_string();
                    i += 2; continue;
                }
            }
            "proto" => {
                if i + 1 < tokens.len() {
                    rule.protocol = tokens[i + 1].to_string();
                    i += 2; continue;
                }
            }
            "from" => {
                if i + 1 < tokens.len() {
                    rule.src = tokens[i + 1].to_string();
                    // Optional "port <p>" right after.
                    if i + 3 < tokens.len() && tokens[i + 2] == "port" {
                        // Skip optional "=" between "port" and the value.
                        let p_idx = if tokens[i + 3] == "=" { i + 4 } else { i + 3 };
                        if p_idx < tokens.len() {
                            rule.src_port = tokens[p_idx].to_string();
                            i = p_idx + 1; continue;
                        }
                    }
                    i += 2; continue;
                }
            }
            "to" => {
                if i + 1 < tokens.len() {
                    rule.dst = tokens[i + 1].to_string();
                    if i + 3 < tokens.len() && tokens[i + 2] == "port" {
                        let p_idx = if tokens[i + 3] == "=" { i + 4 } else { i + 3 };
                        if p_idx < tokens.len() {
                            rule.dst_port = tokens[p_idx].to_string();
                            i = p_idx + 1; continue;
                        }
                    }
                    i += 2; continue;
                }
            }
            _ => {}
        }
        i += 1;
    }
}

// ---- exec helper ----

#[cfg(target_arch = "wasm32")]
fn run_pfctl(timeout_ms: u32) -> Result<ExecResponse, String> {
    let req = ExecRequest {
        command: "/sbin/pfctl".to_string(),
        args: vec!["-sr".to_string()],
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
fn run_pfctl(_timeout_ms: u32) -> Result<ExecResponse, String> {
    Err("not available on host build".to_string())
}

// ============================================================
// Pure-function unit tests
// ============================================================
#[cfg(test)]
mod tests {
    use super::*;

    fn parse_one(line: &str) -> FirewallRule {
        let mut r = FirewallRule { enabled: true, raw: line.to_string(), ..Default::default() };
        parse_pf_rule(line, &mut r);
        r
    }

    #[test]
    fn parse_pass_in_tcp_dport_22() {
        let r = parse_one("pass in proto tcp from any to any port = 22");
        assert_eq!(r.action, "allow");
        assert_eq!(r.direction, "in");
        assert_eq!(r.protocol, "tcp");
        assert_eq!(r.src, "any");
        assert_eq!(r.dst, "any");
        assert_eq!(r.dst_port, "22");
    }

    #[test]
    fn parse_block_return_rst_with_keep_state() {
        let r = parse_one("block return-rst in proto tcp from any to any port = 23 keep state");
        assert_eq!(r.action, "deny");
        assert_eq!(r.direction, "in");
        assert_eq!(r.dst_port, "23");
    }

    #[test]
    fn parse_pass_on_lo0_all() {
        let r = parse_one("pass on lo0 all");
        assert_eq!(r.action, "allow");
        assert_eq!(r.interface_, "lo0");
    }

    #[test]
    fn parse_pass_out_with_src_port() {
        let r = parse_one("pass out proto udp from any port = 53 to any");
        assert_eq!(r.action, "allow");
        assert_eq!(r.direction, "out");
        assert_eq!(r.protocol, "udp");
        assert_eq!(r.src_port, "53");
    }

    #[test]
    fn parse_full_output_three_rules() {
        let stdout = "\
# generated
pass in proto tcp from any to any port = 22
pass in proto tcp from any to any port = 80
block in proto tcp from any to any port = 23
";
        let rules = parse_pfctl_output(stdout);
        assert_eq!(rules.len(), 3);
        assert_eq!(rules[0].id, "pf:1");
        assert_eq!(rules[0].dst_port, "22");
        assert_eq!(rules[2].action, "deny");
        assert_eq!(rules[2].dst_port, "23");
    }

    #[test]
    fn parse_skips_comments_and_blanks() {
        let stdout = "\

# comment
# another
pass in proto tcp from any to any port = 22
";
        let rules = parse_pfctl_output(stdout);
        assert_eq!(rules.len(), 1);
    }
}
