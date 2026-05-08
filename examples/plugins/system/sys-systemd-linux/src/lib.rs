// sys-systemd-linux — three RPCs over `systemctl(1)`:
//
//   list_units(filter)  -> { units: [{name, load, active, sub, description}] }
//   show_unit({name})   -> { properties: { Key: Value, … } }
//   unit_action({...})  -> { ok, exit_code, output, error }
//
// All three shell out via host_exec — `systemctl` is the canonical
// Linux interface for systemd, its output is well-trodden ground, and
// hand-rolling a D-Bus client inside wasm32 is needlessly heavy. The
// CapExec allowlist is restricted to the binary's two canonical paths.
//
// Output parsing is intentionally permissive: distros tweak the
// human-formatting of `systemctl list-units` ("●" decorations, footer
// summary lines, paginator escape sequences when the env doesn't
// disable the pager). The parser drops anything that doesn't have
// five whitespace-tab-separated columns rather than failing the whole
// call. `--no-pager --no-legend --plain` minimises the surface but a
// belt-and-suspenders parser is cheap.

use extism_pdk::*;
use serde::{Deserialize, Serialize};
use std::collections::BTreeMap;

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

// ---------- exec request/response (host-side schema) ----------

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

// ---------- list_units ----------

#[derive(Deserialize, Default)]
struct ListUnitsRequest {
    /// State filter, e.g. "active", "failed", "loaded". Empty = all.
    /// Maps to `systemctl list-units --state=<x>`. Multi-state via
    /// comma is accepted by systemctl natively.
    #[serde(default)]
    state: String,
    /// Type filter, e.g. "service", "socket", "timer". Defaults to
    /// "service" (the by-far most common operator query); pass "*"
    /// or "all" to disable.
    #[serde(default)]
    unit_type: String,
    /// Glob pattern passed straight to systemctl (positional arg).
    /// Empty = no pattern restriction.
    #[serde(default)]
    pattern: String,
    #[serde(default)]
    offset: u32,
    #[serde(default)]
    limit: u32,
}

#[derive(Serialize, Default)]
struct ListUnitsResponse {
    units: Vec<UnitListEntry>,
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

#[derive(Serialize, Default)]
pub struct UnitListEntry {
    name: String,
    load: String,
    active: String,
    sub: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    description: String,
}

#[cfg(target_arch = "wasm32")]
#[plugin_fn]
pub fn list_units(req: Json<ListUnitsRequest>) -> FnResult<String> {
    let r = req.0;
    let mut args: Vec<String> = vec![
        "list-units".to_string(),
        "--all".to_string(),
        "--no-legend".to_string(),
        "--no-pager".to_string(),
        "--plain".to_string(),
    ];
    let unit_type = if r.unit_type.is_empty() {
        "service"
    } else {
        r.unit_type.as_str()
    };
    if unit_type != "*" && unit_type != "all" {
        args.push(format!("--type={}", unit_type));
    }
    if !r.state.is_empty() {
        args.push(format!("--state={}", r.state));
    }
    if !r.pattern.is_empty() {
        args.push(r.pattern);
    }

    let exec_resp = match run_systemctl(args, 10_000) {
        Ok(v) => v,
        Err(e) => {
            return Ok(serde_json::to_string(&ListUnitsResponse {
                units: Vec::new(),
                error: e,
                ..Default::default()
            })?)
        }
    };
    if exec_resp.exit_code != 0 {
        // systemctl returns 1 when no units match the filter — that's
        // an empty list, not an error. Distinguish by checking if
        // stderr is empty.
        if exec_resp.stderr.trim().is_empty() {
            return Ok(serde_json::to_string(&ListUnitsResponse::default())?);
        }
        return Ok(serde_json::to_string(&ListUnitsResponse {
            units: Vec::new(),
            error: format!(
                "systemctl exit {}: {}",
                exec_resp.exit_code,
                exec_resp.stderr.trim()
            ),
            ..Default::default()
        })?);
    }
    let units = parse_list_units(&exec_resp.stdout);
    let (sliced, total, has_more) = paginate(units, r.offset, r.limit);
    Ok(serde_json::to_string(&ListUnitsResponse {
        units: sliced,
        error: String::new(),
        total_count: total,
        has_more,
    })?)
}

// parse_list_units takes the stdout of `systemctl list-units --plain
// --no-legend` and returns one UnitListEntry per parseable row. Format
// (whitespace-padded columns):
//
//   <unit>  <load>  <active>  <sub>  <description...>
//
// The first 4 columns are single tokens; description is everything
// after. Rows with fewer than 4 tokens are dropped.
pub fn parse_list_units(stdout: &str) -> Vec<UnitListEntry> {
    stdout
        .lines()
        .filter_map(|line| {
            let trimmed = line.trim();
            if trimmed.is_empty() {
                return None;
            }
            // Skip footer summary lines: "N loaded units listed."
            // and the trailing instructional "To show all installed…"
            if trimmed.starts_with(|c: char| c.is_ascii_digit())
                && trimmed.contains("loaded units")
            {
                return None;
            }
            let mut parts = trimmed.split_whitespace();
            let name = parts.next()?.to_string();
            // Real unit names always contain a `.` (the type suffix:
            // .service, .socket, .mount, .timer, …). The instructional
            // footer lines `To show all installed unit files use …`
            // start with a non-unit token; this gate filters them.
            if !name.contains('.') {
                return None;
            }
            let load = parts.next()?.to_string();
            let active = parts.next()?.to_string();
            let sub = parts.next()?.to_string();
            let description: String = parts.collect::<Vec<_>>().join(" ");
            Some(UnitListEntry {
                name,
                load,
                active,
                sub,
                description,
            })
        })
        .collect()
}

// ---------- show_unit ----------

#[derive(Deserialize)]
struct ShowUnitRequest {
    name: String,
}

#[derive(Serialize, Default)]
struct ShowUnitResponse {
    properties: BTreeMap<String, String>,
    #[serde(skip_serializing_if = "String::is_empty")]
    error: String,
}

#[cfg(target_arch = "wasm32")]
#[plugin_fn]
pub fn show_unit(req: Json<ShowUnitRequest>) -> FnResult<String> {
    let r = req.0;
    if r.name.is_empty() {
        return Ok(serde_json::to_string(&ShowUnitResponse {
            properties: BTreeMap::new(),
            error: "name is required".to_string(),
        })?);
    }
    if let Err(e) = validate_unit_name(&r.name) {
        return Ok(serde_json::to_string(&ShowUnitResponse {
            properties: BTreeMap::new(),
            error: e,
        })?);
    }
    let exec_resp = match run_systemctl(
        vec!["show".to_string(), "--no-pager".to_string(), r.name],
        10_000,
    ) {
        Ok(v) => v,
        Err(e) => {
            return Ok(serde_json::to_string(&ShowUnitResponse {
                properties: BTreeMap::new(),
                error: e,
            })?)
        }
    };
    if exec_resp.exit_code != 0 {
        return Ok(serde_json::to_string(&ShowUnitResponse {
            properties: BTreeMap::new(),
            error: format!(
                "systemctl exit {}: {}",
                exec_resp.exit_code,
                exec_resp.stderr.trim()
            ),
        })?);
    }
    Ok(serde_json::to_string(&ShowUnitResponse {
        properties: parse_show_unit(&exec_resp.stdout),
        error: String::new(),
    })?)
}

// parse_show_unit splits `systemctl show <unit>` output into a
// Key=Value map. Multi-line values (rare; e.g. ExecStart's argv
// pretty-printing) are not supported in v1 — the trailing lines for
// such a property are silently dropped. Any production multi-line
// value the operator cares about should pass through `show_unit` then
// be re-fetched via a more targeted command.
pub fn parse_show_unit(stdout: &str) -> BTreeMap<String, String> {
    let mut out = BTreeMap::new();
    for line in stdout.lines() {
        if let Some((k, v)) = line.split_once('=') {
            out.insert(k.to_string(), v.to_string());
        }
    }
    out
}

// ---------- unit_action ----------

#[derive(Deserialize)]
struct UnitActionRequest {
    name: String,
    action: String,
}

#[derive(Serialize, Default)]
struct UnitActionResponse {
    ok: bool,
    #[serde(rename = "exitCode")]
    exit_code: i32,
    #[serde(skip_serializing_if = "String::is_empty")]
    output: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    error: String,
}

// Allowed actions. `is-active` / `is-enabled` / `is-failed` are
// included for completeness — they're query-only, exit code carries
// the answer (0 = yes, non-0 = no), so the response's ok/exit_code
// pair gives the caller exactly that.
const ALLOWED_ACTIONS: &[&str] = &[
    "start",
    "stop",
    "restart",
    "reload",
    "try-restart",
    "reload-or-restart",
    "enable",
    "disable",
    "status",
    "is-active",
    "is-enabled",
    "is-failed",
    "mask",
    "unmask",
];

#[cfg(target_arch = "wasm32")]
#[plugin_fn]
pub fn unit_action(req: Json<UnitActionRequest>) -> FnResult<String> {
    let r = req.0;
    if !ALLOWED_ACTIONS.contains(&r.action.as_str()) {
        return Ok(serde_json::to_string(&UnitActionResponse {
            ok: false,
            error: format!("action_not_allowed: {}", r.action),
            ..Default::default()
        })?);
    }
    if let Err(e) = validate_unit_name(&r.name) {
        return Ok(serde_json::to_string(&UnitActionResponse {
            ok: false,
            error: e,
            ..Default::default()
        })?);
    }
    let exec_resp = match run_systemctl(
        vec![r.action.clone(), "--no-pager".to_string(), r.name],
        10_000,
    ) {
        Ok(v) => v,
        Err(e) => {
            return Ok(serde_json::to_string(&UnitActionResponse {
                ok: false,
                error: e,
                ..Default::default()
            })?)
        }
    };
    let combined = if exec_resp.stderr.is_empty() {
        exec_resp.stdout.clone()
    } else if exec_resp.stdout.is_empty() {
        exec_resp.stderr.clone()
    } else {
        format!("{}\n{}", exec_resp.stdout, exec_resp.stderr)
    };
    Ok(serde_json::to_string(&UnitActionResponse {
        ok: exec_resp.exit_code == 0,
        exit_code: exec_resp.exit_code,
        output: combined,
        error: String::new(),
    })?)
}

// validate_unit_name rejects shell-escaping foot-guns. systemctl
// itself won't dispatch `; rm -rf /` (each arg is a separate argv
// slot, so shell metachar interpretation is impossible), but a unit
// name containing `\0` or a leading `-` could be mis-parsed by
// systemctl as an option. Belt-and-suspenders.
fn validate_unit_name(name: &str) -> Result<(), String> {
    if name.is_empty() {
        return Err("name is required".to_string());
    }
    if name.starts_with('-') {
        return Err("name must not start with '-'".to_string());
    }
    if name.contains('\0') || name.contains('\n') {
        return Err("name contains forbidden characters".to_string());
    }
    Ok(())
}

// ---------- exec helper ----------

#[cfg(target_arch = "wasm32")]
fn run_systemctl(args: Vec<String>, timeout_ms: u32) -> Result<ExecResponse, String> {
    // Try the canonical /usr/bin/systemctl first (Debian/Ubuntu); if
    // host_exec rejects it as not-in-allowlist (shouldn't happen given
    // our manifest, but be defensive), fall back to /bin/systemctl
    // (the RHEL shim path).
    for path in &["/usr/bin/systemctl", "/bin/systemctl"] {
        let req = ExecRequest {
            command: path.to_string(),
            args: args.clone(),
            timeout_ms,
        };
        let body = match serde_json::to_string(&req) {
            Ok(b) => b,
            Err(e) => return Err(format!("encode_exec_req: {}", e)),
        };
        let env: Envelope = match unsafe { host_exec(body) } {
            Ok(j) => j.0,
            Err(e) => return Err(format!("host_exec: {}", e)),
        };
        if !env.ok {
            // The first probe failing with "no such file" is normal on
            // RHEL where /usr/bin/systemctl doesn't exist; loop and try
            // /bin/systemctl. Capability denials are not retryable —
            // they'll fail the same way for both paths.
            if env.error.contains("capability_denied") {
                return Err(env.error);
            }
            continue;
        }
        let resp: ExecResponse = serde_json::from_value(env.data)
            .map_err(|e| format!("decode_exec_resp: {}", e))?;
        return Ok(resp);
    }
    Err("systemctl_not_found_on_either_path".to_string())
}

// ---------- host-side stubs (rlib build only) ----------

#[cfg(not(target_arch = "wasm32"))]
fn run_systemctl(_args: Vec<String>, _timeout_ms: u32) -> Result<ExecResponse, String> {
    Err("not available on host build".to_string())
}

// ============================================================
// tests (host-build only — pure parsers)
// ============================================================

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parse_list_units_basic() {
        let stdout = "\
ssh.service               loaded active running   OpenBSD Secure Shell server
nginx.service             loaded active running   nginx HTTP server
cron.service              loaded active running   Regular background program processing daemon
";
        let units = parse_list_units(stdout);
        assert_eq!(units.len(), 3);
        assert_eq!(units[0].name, "ssh.service");
        assert_eq!(units[0].load, "loaded");
        assert_eq!(units[0].active, "active");
        assert_eq!(units[0].sub, "running");
        assert!(units[0].description.contains("OpenBSD"));
    }

    #[test]
    fn parse_list_units_skips_footer() {
        let stdout = "\
ssh.service     loaded active running   OpenBSD Secure Shell server

3 loaded units listed.
To show all installed unit files use 'systemctl list-unit-files'.
";
        let units = parse_list_units(stdout);
        assert_eq!(units.len(), 1);
        assert_eq!(units[0].name, "ssh.service");
    }

    #[test]
    fn parse_list_units_empty() {
        assert!(parse_list_units("").is_empty());
        assert!(parse_list_units("\n\n   \n").is_empty());
    }

    #[test]
    fn parse_list_units_short_row_dropped() {
        // Only 3 columns — should be filtered, not panic.
        let stdout = "ssh.service loaded active\n";
        assert!(parse_list_units(stdout).is_empty());
    }

    #[test]
    fn parse_show_unit_kv() {
        let stdout = "\
Id=ssh.service
LoadState=loaded
ActiveState=active
SubState=running
Description=OpenBSD Secure Shell server
ExecMainPID=1234
";
        let map = parse_show_unit(stdout);
        assert_eq!(map.get("Id").map(String::as_str), Some("ssh.service"));
        assert_eq!(map.get("LoadState").map(String::as_str), Some("loaded"));
        assert_eq!(map.get("ExecMainPID").map(String::as_str), Some("1234"));
    }

    #[test]
    fn parse_show_unit_handles_equals_in_value() {
        // Property values may contain `=` (e.g. environment dumps).
        // split_once must take the first delimiter only.
        let stdout = "Environment=PATH=/usr/local/bin:/usr/bin\n";
        let map = parse_show_unit(stdout);
        assert_eq!(
            map.get("Environment").map(String::as_str),
            Some("PATH=/usr/local/bin:/usr/bin")
        );
    }

    #[test]
    fn validate_unit_name_rejects_dash_prefix() {
        assert!(validate_unit_name("-evil").is_err());
        assert!(validate_unit_name("").is_err());
        assert!(validate_unit_name("ok\nbad").is_err());
        assert!(validate_unit_name("ssh.service").is_ok());
        assert!(validate_unit_name("getty@tty1.service").is_ok());
    }

    #[test]
    fn allowed_actions_covers_common_ops() {
        for a in &[
            "start",
            "stop",
            "restart",
            "reload",
            "enable",
            "disable",
            "status",
        ] {
            assert!(ALLOWED_ACTIONS.contains(a));
        }
        // Sanity: a clearly-bad string is rejected.
        assert!(!ALLOWED_ACTIONS.contains(&"daemon-reexec"));
    }
}
