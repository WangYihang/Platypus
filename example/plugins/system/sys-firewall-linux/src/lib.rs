// sys-firewall-linux — auto-detect active firewall backend, dump
// rules, normalise to the cross-OS FirewallRule shape.
//
// Backends in detection priority order:
//   1. nft list ruleset       (modern nftables; Debian/Ubuntu default since 11)
//   2. iptables-save          (legacy interface; covers iptables-nft too)
//   3. ufw status numbered    (Debian/Ubuntu user-space wrapper)
//   4. firewall-cmd --list-all (RHEL/SUSE/Fedora user-space wrapper)
//
// We try each in turn; first one that produces a 0-exit response
// wins. The detected backend ships back in the response so the UI
// can show "this host runs nftables".

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

// ---- request / response wire shapes ----

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
    // Try backends in priority order.
    if let Some((rules, backend)) = try_iptables() {
        return Ok(serde_json::to_string(&ListResponse {
            rules: filter_rules(rules, &r),
            backend,
            error: String::new(),
        })?);
    }
    if let Some((rules, backend)) = try_nftables() {
        return Ok(serde_json::to_string(&ListResponse {
            rules: filter_rules(rules, &r),
            backend,
            error: String::new(),
        })?);
    }
    if let Some((rules, backend)) = try_ufw() {
        return Ok(serde_json::to_string(&ListResponse {
            rules: filter_rules(rules, &r),
            backend,
            error: String::new(),
        })?);
    }
    if let Some((rules, backend)) = try_firewalld() {
        return Ok(serde_json::to_string(&ListResponse {
            rules: filter_rules(rules, &r),
            backend,
            error: String::new(),
        })?);
    }
    Ok(serde_json::to_string(&ListResponse {
        rules: Vec::new(),
        backend: String::new(),
        error: "no_supported_firewall_backend".to_string(),
    })?)
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
                && !rule.raw.to_ascii_lowercase().contains(&needle)
            {
                return false;
            }
            true
        })
        .collect()
}

// ---- iptables backend ----

#[cfg(target_arch = "wasm32")]
fn try_iptables() -> Option<(Vec<FirewallRule>, String)> {
    let path = first_existing(&["/sbin/iptables-save", "/usr/sbin/iptables-save"])?;
    let resp = run_one(&path, &[], 10_000).ok()?;
    if resp.exit_code != 0 {
        return None;
    }
    let mut rules = parse_iptables_save(&resp.stdout);
    // Also try ip6tables-save and merge.
    if let Some(p6) = first_existing(&["/sbin/ip6tables-save", "/usr/sbin/ip6tables-save"]) {
        if let Ok(r6) = run_one(&p6, &[], 10_000) {
            if r6.exit_code == 0 {
                let mut v6_rules = parse_iptables_save(&r6.stdout);
                for r in v6_rules.iter_mut() {
                    if !r.id.is_empty() {
                        r.id = format!("v6:{}", r.id);
                    }
                }
                rules.append(&mut v6_rules);
            }
        }
    }
    Some((rules, "iptables".to_string()))
}

// parse_iptables_save handles the iptables-save output:
//   *filter
//   :INPUT ACCEPT [0:0]
//   :FORWARD DROP [0:0]
//   :OUTPUT ACCEPT [0:0]
//   -A INPUT -i lo -j ACCEPT
//   -A INPUT -p tcp --dport 22 -j ACCEPT
//   COMMIT
// The `-A <CHAIN> ...` lines are the rule rows. Other lines
// (`*table`, `:CHAIN POLICY`, `COMMIT`, `#`) are skipped.
fn parse_iptables_save(stdout: &str) -> Vec<FirewallRule> {
    let mut out = Vec::new();
    let mut current_table = String::from("filter");
    let mut chain_index: std::collections::HashMap<String, u32> = std::collections::HashMap::new();
    for line in stdout.lines() {
        let line = line.trim();
        if line.is_empty() || line.starts_with('#') || line == "COMMIT" {
            continue;
        }
        if let Some(t) = line.strip_prefix('*') {
            current_table = t.trim().to_string();
            continue;
        }
        if line.starts_with(':') {
            // Chain default-policy row; we don't surface those as
            // rules but could in a future revision.
            continue;
        }
        if let Some(rest) = line.strip_prefix("-A ") {
            let parts: Vec<&str> = rest.split_whitespace().collect();
            if parts.is_empty() {
                continue;
            }
            let chain = parts[0].to_string();
            let key = format!("{current_table}/{chain}");
            let idx = chain_index.entry(key.clone()).or_insert(0);
            *idx += 1;
            let id = format!("{current_table}:{chain}:{}", idx);
            let mut rule = FirewallRule {
                id,
                chain: format!("{current_table}/{chain}"),
                direction: chain_to_direction(&chain),
                enabled: true,
                raw: line.to_string(),
                ..Default::default()
            };
            apply_iptables_args(&mut rule, &parts[1..]);
            out.push(rule);
        }
    }
    out
}

// chain_to_direction maps the iptables built-in chain name to a
// crude direction. PREROUTING / FORWARD / POSTROUTING land as
// "both" because they cover NAT-style rewrites in either path.
fn chain_to_direction(chain: &str) -> String {
    match chain {
        "INPUT" => "in".to_string(),
        "OUTPUT" => "out".to_string(),
        "FORWARD" | "PREROUTING" | "POSTROUTING" => "both".to_string(),
        _ => String::new(),
    }
}

// apply_iptables_args walks the rest of an iptables -A row and
// fills in the FirewallRule fields. Handles the most common args:
// -p / --protocol, --dport / --sport, -s / -d, -i / -o, -j / --jump.
fn apply_iptables_args(rule: &mut FirewallRule, args: &[&str]) {
    let mut i = 0;
    while i < args.len() {
        let a = args[i];
        let next = || -> Option<&str> {
            if i + 1 < args.len() { Some(args[i + 1]) } else { None }
        };
        match a {
            "-p" | "--protocol" => {
                if let Some(v) = next() {
                    rule.protocol = v.to_string();
                    i += 2; continue;
                }
            }
            "--dport" | "--destination-port" => {
                if let Some(v) = next() {
                    rule.dst_port = v.to_string();
                    i += 2; continue;
                }
            }
            "--sport" | "--source-port" => {
                if let Some(v) = next() {
                    rule.src_port = v.to_string();
                    i += 2; continue;
                }
            }
            "-s" | "--source" => {
                if let Some(v) = next() {
                    rule.src = v.to_string();
                    i += 2; continue;
                }
            }
            "-d" | "--destination" => {
                if let Some(v) = next() {
                    rule.dst = v.to_string();
                    i += 2; continue;
                }
            }
            "-i" | "--in-interface" => {
                if let Some(v) = next() {
                    rule.interface_ = v.to_string();
                    i += 2; continue;
                }
            }
            "-o" | "--out-interface" => {
                if let Some(v) = next() {
                    if rule.interface_.is_empty() {
                        rule.interface_ = v.to_string();
                    }
                    i += 2; continue;
                }
            }
            "-j" | "--jump" => {
                if let Some(v) = next() {
                    rule.action = jump_to_action(v);
                    i += 2; continue;
                }
            }
            _ => {}
        }
        i += 1;
    }
}

fn jump_to_action(jump: &str) -> String {
    match jump {
        "ACCEPT" => "allow".to_string(),
        "DROP" => "drop".to_string(),
        "REJECT" => "reject".to_string(),
        "LOG" => "log".to_string(),
        "MASQUERADE" => "masquerade".to_string(),
        "DNAT" | "SNAT" | "REDIRECT" => "nat".to_string(),
        other => other.to_lowercase(),
    }
}

// ---- nftables backend (raw fallback) ----

#[cfg(target_arch = "wasm32")]
fn try_nftables() -> Option<(Vec<FirewallRule>, String)> {
    let path = first_existing(&["/usr/sbin/nft", "/sbin/nft"])?;
    let resp = run_one(&path, &["list".to_string(), "ruleset".to_string()], 10_000).ok()?;
    if resp.exit_code != 0 {
        return None;
    }
    let rules = parse_nft_ruleset(&resp.stdout);
    if rules.is_empty() {
        return None;
    }
    Some((rules, "nftables".to_string()))
}

// parse_nft_ruleset peels off table { chain { rule; rule; } chain
// { ... } } blocks. Each rule line becomes a FirewallRule with the
// table+chain in `chain` and the rule text in `raw`. Per-field
// extraction (proto / dport / src / dst / verdict) follows the
// nftables grammar.
fn parse_nft_ruleset(stdout: &str) -> Vec<FirewallRule> {
    let mut out = Vec::new();
    let mut current_table = String::new();
    let mut current_chain = String::new();
    for line in stdout.lines() {
        let trimmed = line.trim();
        if trimmed.is_empty() || trimmed.starts_with('#') {
            continue;
        }
        // Strip `{` / `}` lines.
        if trimmed == "{" || trimmed == "}" {
            if trimmed == "}" {
                if !current_chain.is_empty() {
                    current_chain.clear();
                } else if !current_table.is_empty() {
                    current_table.clear();
                }
            }
            continue;
        }
        if let Some(rest) = trimmed.strip_prefix("table ") {
            // "table inet filter {"
            let parts: Vec<&str> = rest.split_whitespace().collect();
            if parts.len() >= 2 {
                current_table = format!("{} {}", parts[0], parts[1]);
            }
            continue;
        }
        if let Some(rest) = trimmed.strip_prefix("chain ") {
            // "chain input {"
            let name = rest.trim_end_matches(" {").trim();
            current_chain = name.to_string();
            continue;
        }
        // Skip the chain header lines: "type filter hook input ..."
        if trimmed.starts_with("type ")
            || trimmed.starts_with("policy ")
            || trimmed.starts_with("hook ")
            || trimmed.starts_with("counter packets")
        {
            continue;
        }
        if current_chain.is_empty() {
            continue;
        }
        let rule = parse_nft_rule_line(trimmed, &current_table, &current_chain);
        out.push(rule);
    }
    out
}

fn parse_nft_rule_line(line: &str, table: &str, chain: &str) -> FirewallRule {
    let mut rule = FirewallRule {
        chain: format!("{table}/{chain}"),
        direction: chain_to_direction(&chain.to_uppercase()),
        enabled: true,
        raw: line.to_string(),
        ..Default::default()
    };
    let tokens: Vec<&str> = line.split_whitespace().collect();
    let mut i = 0;
    while i < tokens.len() {
        let t = tokens[i];
        match t {
            "tcp" | "udp" | "icmp" | "icmpv6" => {
                if rule.protocol.is_empty() {
                    rule.protocol = t.to_string();
                }
            }
            "dport" => {
                if i + 1 < tokens.len() {
                    rule.dst_port = tokens[i + 1].trim_end_matches(',').to_string();
                }
            }
            "sport" => {
                if i + 1 < tokens.len() {
                    rule.src_port = tokens[i + 1].trim_end_matches(',').to_string();
                }
            }
            "saddr" => {
                if i + 1 < tokens.len() {
                    rule.src = tokens[i + 1].trim_end_matches(',').to_string();
                }
            }
            "daddr" => {
                if i + 1 < tokens.len() {
                    rule.dst = tokens[i + 1].trim_end_matches(',').to_string();
                }
            }
            "iifname" | "iif" => {
                if i + 1 < tokens.len() {
                    rule.interface_ = tokens[i + 1].trim_matches('"').to_string();
                }
            }
            "accept" | "drop" | "reject" | "log" => {
                rule.action = match t {
                    "accept" => "allow".to_string(),
                    other => other.to_string(),
                };
            }
            _ => {}
        }
        i += 1;
    }
    rule
}

// ---- ufw backend ----

#[cfg(target_arch = "wasm32")]
fn try_ufw() -> Option<(Vec<FirewallRule>, String)> {
    let path = first_existing(&["/usr/sbin/ufw"])?;
    let resp = run_one(&path, &["status".to_string(), "numbered".to_string()], 10_000).ok()?;
    if resp.exit_code != 0 {
        return None;
    }
    let rules = parse_ufw_status(&resp.stdout);
    if rules.is_empty() && !resp.stdout.contains("Status: active") {
        return None;
    }
    Some((rules, "ufw".to_string()))
}

// parse_ufw_status handles the `ufw status numbered` output:
//   Status: active
//   To                         Action      From
//   --                         ------      ----
//   [ 1] 22/tcp                ALLOW IN    Anywhere
//   [ 2] 80                    ALLOW IN    Anywhere (v6)
fn parse_ufw_status(stdout: &str) -> Vec<FirewallRule> {
    let mut out = Vec::new();
    for line in stdout.lines() {
        let trimmed = line.trim();
        if !trimmed.starts_with('[') {
            continue;
        }
        // Format: "[ N] <to-spec>  <ACTION DIR>  <from-spec>"
        let close = match trimmed.find(']') {
            Some(i) => i,
            None => continue,
        };
        let id = trimmed[1..close].trim().to_string();
        let rest = trimmed[close + 1..].trim();
        // Split into >=3 columns by 2+ spaces.
        let fields = split_double_space(rest);
        if fields.len() < 3 {
            continue;
        }
        let to = fields[0].clone();
        let action_dir = fields[1].clone();
        let from = fields[2].clone();
        let parts: Vec<&str> = action_dir.split_whitespace().collect();
        let action = parts.first().map(|s| s.to_lowercase()).unwrap_or_default();
        let direction = parts
            .get(1)
            .map(|s| match *s {
                "IN" => "in".to_string(),
                "OUT" => "out".to_string(),
                _ => String::new(),
            })
            .unwrap_or_default();
        // Best-effort: "22/tcp" → port=22, proto=tcp.
        let (dst_port, protocol) = split_port_proto(&to);
        out.push(FirewallRule {
            id: format!("ufw:{id}"),
            direction,
            action: match action.as_str() {
                "allow" => "allow".to_string(),
                "deny" => "deny".to_string(),
                "reject" => "reject".to_string(),
                "limit" => "allow".to_string(), // limit is rate-limited allow
                other => other.to_string(),
            },
            protocol,
            src: from,
            dst_port,
            enabled: true,
            raw: trimmed.to_string(),
            ..Default::default()
        });
    }
    out
}

// split_double_space splits on runs of >=2 spaces — preserves
// internal single spaces in field values (e.g. "Anywhere (v6)").
fn split_double_space(s: &str) -> Vec<String> {
    let mut out = Vec::new();
    let mut cur = String::new();
    let mut spaces = 0usize;
    for ch in s.chars() {
        if ch == ' ' {
            spaces += 1;
            if spaces >= 2 && !cur.is_empty() {
                out.push(cur.trim().to_string());
                cur.clear();
            }
        } else {
            if spaces > 0 && !cur.is_empty() && spaces < 2 {
                cur.push(' ');
            }
            spaces = 0;
            cur.push(ch);
        }
    }
    if !cur.is_empty() {
        out.push(cur.trim().to_string());
    }
    out
}

fn split_port_proto(to: &str) -> (String, String) {
    if let Some(idx) = to.find('/') {
        return (to[..idx].to_string(), to[idx + 1..].to_string());
    }
    (to.to_string(), String::new())
}

// ---- firewalld backend (very thin v1 — surface raw rich rules) ----

#[cfg(target_arch = "wasm32")]
fn try_firewalld() -> Option<(Vec<FirewallRule>, String)> {
    let path = first_existing(&["/usr/bin/firewall-cmd"])?;
    let resp = run_one(&path, &["--list-all".to_string()], 10_000).ok()?;
    if resp.exit_code != 0 {
        return None;
    }
    let rules = parse_firewalld_list_all(&resp.stdout);
    Some((rules, "firewalld".to_string()))
}

// parse_firewalld_list_all skims `firewall-cmd --list-all` output.
// We surface the active services + ports as rules with raw=<line>;
// rich rules ("rich rules: ...") become individual rule rows.
fn parse_firewalld_list_all(stdout: &str) -> Vec<FirewallRule> {
    let mut out = Vec::new();
    let mut zone = String::new();
    for line in stdout.lines() {
        let trimmed = line.trim();
        if trimmed.is_empty() {
            continue;
        }
        // Zone line: "public (active)"
        if let Some(rest) = trimmed.strip_suffix("(active)") {
            zone = rest.trim().to_string();
            continue;
        }
        if let Some(rest) = trimmed.strip_prefix("services: ") {
            for s in rest.split_whitespace() {
                out.push(FirewallRule {
                    id: format!("firewalld:{zone}:service:{s}"),
                    name: s.to_string(),
                    chain: zone.clone(),
                    direction: "in".to_string(),
                    action: "allow".to_string(),
                    enabled: true,
                    raw: format!("services: {s}"),
                    ..Default::default()
                });
            }
            continue;
        }
        if let Some(rest) = trimmed.strip_prefix("ports: ") {
            for p in rest.split_whitespace() {
                let (port, proto) = split_port_proto(p);
                out.push(FirewallRule {
                    id: format!("firewalld:{zone}:port:{p}"),
                    chain: zone.clone(),
                    direction: "in".to_string(),
                    action: "allow".to_string(),
                    protocol: proto,
                    dst_port: port,
                    enabled: true,
                    raw: format!("ports: {p}"),
                    ..Default::default()
                });
            }
            continue;
        }
        if let Some(rest) = trimmed.strip_prefix("rich rules: ") {
            for r in rest.split('\n') {
                let r = r.trim();
                if r.is_empty() {
                    continue;
                }
                out.push(FirewallRule {
                    id: format!("firewalld:{zone}:rich"),
                    chain: zone.clone(),
                    enabled: true,
                    raw: r.to_string(),
                    ..Default::default()
                });
            }
        }
    }
    out
}

// ---- exec helper ----

#[cfg(target_arch = "wasm32")]
fn first_existing(candidates: &[&str]) -> Option<String> {
    for c in candidates {
        if probe_exists(c) {
            return Some((*c).to_string());
        }
    }
    None
}

#[cfg(target_arch = "wasm32")]
fn probe_exists(path: &str) -> bool {
    let req = ExecRequest {
        command: "/bin/sh".to_string(),
        args: vec!["-c".to_string(), format!("command -v {path} >/dev/null 2>&1")],
        timeout_ms: 2_000,
    };
    let body = match serde_json::to_string(&req) { Ok(s) => s, Err(_) => return false };
    let env: Envelope = match unsafe { host_exec(body) } { Ok(j) => j.0, Err(_) => return false };
    if !env.ok { return false; }
    let resp: ExecResponse = match serde_json::from_value(env.data) { Ok(r) => r, Err(_) => return false };
    resp.exit_code == 0
}

#[cfg(target_arch = "wasm32")]
fn run_one(cmd: &str, args: &[String], timeout_ms: u32) -> Result<ExecResponse, String> {
    let req = ExecRequest {
        command: cmd.to_string(),
        args: args.to_vec(),
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

// ============================================================
// Pure-function unit tests
// ============================================================
#[cfg(test)]
mod tests {
    use super::*;

    // ---- parse_iptables_save ----

    #[test]
    fn parse_iptables_basic() {
        let stdout = "\
*filter
:INPUT ACCEPT [0:0]
:FORWARD DROP [0:0]
:OUTPUT ACCEPT [0:0]
-A INPUT -i lo -j ACCEPT
-A INPUT -p tcp --dport 22 -j ACCEPT
-A INPUT -j DROP
COMMIT
";
        let rules = parse_iptables_save(stdout);
        assert_eq!(rules.len(), 3);
        assert_eq!(rules[0].chain, "filter/INPUT");
        assert_eq!(rules[0].direction, "in");
        assert_eq!(rules[0].interface_, "lo");
        assert_eq!(rules[0].action, "allow");

        assert_eq!(rules[1].protocol, "tcp");
        assert_eq!(rules[1].dst_port, "22");
        assert_eq!(rules[1].action, "allow");

        assert_eq!(rules[2].action, "drop");
    }

    #[test]
    fn parse_iptables_multiple_tables() {
        let stdout = "\
*nat
-A POSTROUTING -o eth0 -j MASQUERADE
COMMIT
*filter
-A INPUT -p tcp --dport 80 -j ACCEPT
COMMIT
";
        let rules = parse_iptables_save(stdout);
        assert_eq!(rules.len(), 2);
        assert_eq!(rules[0].chain, "nat/POSTROUTING");
        assert_eq!(rules[0].action, "masquerade");
        assert_eq!(rules[1].chain, "filter/INPUT");
    }

    #[test]
    fn parse_iptables_skips_chain_decls_and_comments() {
        let stdout = "\
# Generated by iptables-save
*filter
:INPUT ACCEPT [0:0]
COMMIT
";
        assert!(parse_iptables_save(stdout).is_empty());
    }

    // ---- chain_to_direction ----

    #[test]
    fn chain_direction_mapping() {
        assert_eq!(chain_to_direction("INPUT"), "in");
        assert_eq!(chain_to_direction("OUTPUT"), "out");
        assert_eq!(chain_to_direction("FORWARD"), "both");
        assert_eq!(chain_to_direction("PREROUTING"), "both");
        assert_eq!(chain_to_direction("USER_DEFINED"), "");
    }

    // ---- jump_to_action ----

    #[test]
    fn jump_action_mapping() {
        assert_eq!(jump_to_action("ACCEPT"), "allow");
        assert_eq!(jump_to_action("DROP"), "drop");
        assert_eq!(jump_to_action("REJECT"), "reject");
        assert_eq!(jump_to_action("MASQUERADE"), "masquerade");
        assert_eq!(jump_to_action("CUSTOM_CHAIN"), "custom_chain");
    }

    // ---- parse_nft_ruleset ----

    #[test]
    fn parse_nft_basic() {
        let stdout = "\
table inet filter {
    chain input {
        type filter hook input priority 0; policy accept;
        iifname \"lo\" accept
        tcp dport 22 accept
        tcp dport 80 accept
    }
    chain forward {
        type filter hook forward priority 0; policy drop;
    }
}
";
        let rules = parse_nft_ruleset(stdout);
        // 3 rules in input chain, 0 in forward.
        assert_eq!(rules.len(), 3);
        assert_eq!(rules[0].interface_, "lo");
        assert_eq!(rules[0].action, "allow");
        assert_eq!(rules[0].chain, "inet filter/input");
        assert_eq!(rules[1].dst_port, "22");
        assert_eq!(rules[1].protocol, "tcp");
    }

    // ---- parse_ufw_status ----

    #[test]
    fn parse_ufw_basic() {
        let stdout = "\
Status: active

     To                         Action      From
     --                         ------      ----
[ 1] 22/tcp                     ALLOW IN    Anywhere
[ 2] 80                         ALLOW IN    Anywhere (v6)
[ 3] 443/tcp                    DENY IN     192.168.1.0/24
";
        let rules = parse_ufw_status(stdout);
        assert_eq!(rules.len(), 3);
        assert_eq!(rules[0].id, "ufw:1");
        assert_eq!(rules[0].action, "allow");
        assert_eq!(rules[0].direction, "in");
        assert_eq!(rules[0].protocol, "tcp");
        assert_eq!(rules[0].dst_port, "22");

        assert_eq!(rules[2].action, "deny");
        assert_eq!(rules[2].src, "192.168.1.0/24");
        assert_eq!(rules[2].dst_port, "443");
    }

    // ---- split_port_proto ----

    #[test]
    fn split_port_proto_with_slash() {
        assert_eq!(split_port_proto("22/tcp"), ("22".to_string(), "tcp".to_string()));
        assert_eq!(split_port_proto("8080/udp"), ("8080".to_string(), "udp".to_string()));
    }

    #[test]
    fn split_port_proto_without_slash() {
        assert_eq!(split_port_proto("80"), ("80".to_string(), String::new()));
    }

    // ---- filter_rules ----

    #[test]
    fn filter_includes_disabled_when_requested() {
        let rules = vec![
            FirewallRule { name: "A".to_string(), enabled: true, ..Default::default() },
            FirewallRule { name: "B".to_string(), enabled: false, ..Default::default() },
        ];
        let req = ListRequest { include_disabled: true, filter: String::new() };
        assert_eq!(filter_rules(rules.clone(), &req).len(), 2);
        let req = ListRequest::default();
        assert_eq!(filter_rules(rules, &req).len(), 1);
    }

    #[test]
    fn filter_by_substring_matches_name_or_raw() {
        let rules = vec![
            FirewallRule { name: "Docker".to_string(), enabled: true, ..Default::default() },
            FirewallRule { raw: "DOCKER-USER stuff".to_string(), enabled: true, ..Default::default() },
            FirewallRule { name: "Skype".to_string(), enabled: true, ..Default::default() },
        ];
        let req = ListRequest { filter: "docker".to_string(), include_disabled: false };
        let out = filter_rules(rules, &req);
        assert_eq!(out.len(), 2);
    }
}

// Make FirewallRule cloneable in tests (the prod code never clones
// it). Cheap to add since all fields are owned strings + bools.
#[cfg(test)]
impl Clone for FirewallRule {
    fn clone(&self) -> Self {
        FirewallRule {
            id: self.id.clone(),
            name: self.name.clone(),
            direction: self.direction.clone(),
            action: self.action.clone(),
            protocol: self.protocol.clone(),
            src: self.src.clone(),
            dst: self.dst.clone(),
            src_port: self.src_port.clone(),
            dst_port: self.dst_port.clone(),
            enabled: self.enabled,
            interface_: self.interface_.clone(),
            profile: self.profile.clone(),
            raw: self.raw.clone(),
            chain: self.chain.clone(),
        }
    }
}
