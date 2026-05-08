// sys-users-windows — Get-LocalUser + Get-LocalGroup +
// Get-LocalGroupMember Administrators.
//
// PowerShell pipeline collapses all three queries into one
// invocation that emits a single JSON object with `users`,
// `groups`, and `admins` keys.

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
    include_system: bool,
}

#[derive(Serialize, Default)]
struct ListResponse {
    users: Vec<User>,
    groups: Vec<Group>,
    sudoers: Vec<SudoEntry>,
    #[serde(skip_serializing_if = "String::is_empty")]
    error: String,
}

#[derive(Serialize, Deserialize, Default, Debug, PartialEq)]
#[serde(default)]
struct User {
    username: String,
    uid: u32,
    gid: u32,
    #[serde(rename = "fullName", skip_serializing_if = "String::is_empty")]
    full_name: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    home: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    shell: String,
    #[serde(rename = "isSystem", skip_serializing_if = "is_false")]
    is_system: bool,
    #[serde(rename = "isLocked", skip_serializing_if = "is_false")]
    is_locked: bool,
    #[serde(skip_serializing_if = "Vec::is_empty")]
    groups: Vec<String>,
}

#[derive(Serialize, Deserialize, Default, Debug, PartialEq)]
#[serde(default)]
struct Group {
    name: String,
    gid: u32,
    #[serde(skip_serializing_if = "Vec::is_empty")]
    members: Vec<String>,
}

#[derive(Serialize, Default, Debug, PartialEq)]
struct SudoEntry {
    source: String,
    who: String,
    how: String,
}

fn is_false(b: &bool) -> bool { !*b }

const SYSTEM_USERNAMES: &[&str] = &[
    "DefaultAccount",
    "WDAGUtilityAccount",
    "Guest",
    "SYSTEM",
];

// ---- entry point ----

const PS_SCRIPT: &str = r#"[Console]::OutputEncoding = [System.Text.Encoding]::UTF8;
$users = @(Get-LocalUser -ErrorAction SilentlyContinue | ForEach-Object {
    [pscustomobject]@{
        username  = [string]$_.Name
        fullName  = [string]$_.FullName
        isLocked  = -not ([bool]$_.Enabled)
        isSystem  = $false
        groups    = @()
    }
})
$groups = @(Get-LocalGroup -ErrorAction SilentlyContinue | ForEach-Object {
    $g = $_
    $members = @()
    try {
        $members = @(Get-LocalGroupMember -Name $g.Name -ErrorAction Stop |
                     ForEach-Object { [string]$_.Name })
    } catch {}
    [pscustomobject]@{
        name    = [string]$g.Name
        gid     = 0
        members = $members
    }
})
$admins = @()
try {
    $admins = @(Get-LocalGroupMember -Name 'Administrators' -ErrorAction Stop |
                ForEach-Object { [string]$_.Name })
} catch {}
$result = [pscustomobject]@{
    users  = $users
    groups = $groups
    admins = $admins
}
$result | ConvertTo-Json -Compress -Depth 5"#;

#[derive(Deserialize, Default)]
struct PsResult {
    #[serde(default)]
    users: Vec<User>,
    #[serde(default)]
    groups: Vec<Group>,
    #[serde(default)]
    admins: Vec<String>,
}

#[cfg(target_arch = "wasm32")]
#[plugin_fn]
pub fn list_users(req: Json<ListRequest>) -> FnResult<String> {
    let r = req.0;
    let exec_resp = match run_powershell(PS_SCRIPT, 30_000) {
        Ok(v) => v,
        Err(e) => {
            return Ok(serde_json::to_string(&ListResponse {
                users: Vec::new(),
                groups: Vec::new(),
                sudoers: Vec::new(),
                error: e,
            })?)
        }
    };
    if exec_resp.exit_code != 0 {
        return Ok(serde_json::to_string(&ListResponse {
            users: Vec::new(),
            groups: Vec::new(),
            sudoers: Vec::new(),
            error: format!(
                "powershell exit {}: {}",
                exec_resp.exit_code,
                exec_resp.stderr.trim()
            ),
        })?);
    }
    let parsed = parse_powershell_output(&exec_resp.stdout);
    let mut users = parsed.users;
    let groups = parsed.groups;
    let admins = parsed.admins;

    // Tag system accounts before filtering.
    for u in users.iter_mut() {
        if SYSTEM_USERNAMES.iter().any(|s| s.eq_ignore_ascii_case(&u.username)) {
            u.is_system = true;
        }
    }
    if !r.include_system {
        users.retain(|u| !u.is_system);
    }

    // Cross-reference: fill each user's `groups` from the
    // Get-LocalGroupMember results.
    let user_to_groups = build_user_to_groups(&groups);
    for u in users.iter_mut() {
        if let Some(gs) = user_to_groups.get(&u.username) {
            for g in gs {
                if !u.groups.iter().any(|x| x == g) {
                    u.groups.push(g.clone());
                }
            }
        }
    }

    // Synthesise SudoEntry rows from Administrators membership.
    let sudoers: Vec<SudoEntry> = admins
        .iter()
        .map(|a| SudoEntry {
            source: "Administrators".to_string(),
            who: a.clone(),
            how: String::new(),
        })
        .collect();

    Ok(serde_json::to_string(&ListResponse {
        users,
        groups,
        sudoers,
        error: String::new(),
    })?)
}

// ---- pure helpers ----

fn parse_powershell_output(stdout: &str) -> PsResult {
    let trimmed = stdout.trim();
    if trimmed.is_empty() || trimmed == "null" {
        return PsResult::default();
    }
    serde_json::from_str(trimmed).unwrap_or_default()
}

fn build_user_to_groups(groups: &[Group]) -> std::collections::HashMap<String, Vec<String>> {
    let mut out: std::collections::HashMap<String, Vec<String>> = std::collections::HashMap::new();
    for g in groups {
        for m in &g.members {
            // Member names from Get-LocalGroupMember come back as
            // "MACHINE\username". Strip the prefix so the user-side
            // join matches the Get-LocalUser output (bare username).
            let bare = m.rsplit('\\').next().unwrap_or(m).to_string();
            out.entry(bare).or_default().push(g.name.clone());
        }
    }
    out
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
    fn parse_basic_json() {
        let json = r#"{
            "users":[{"username":"alice","fullName":"Alice","isLocked":false},
                     {"username":"DefaultAccount","fullName":"","isLocked":true}],
            "groups":[{"name":"Administrators","members":["WIN10\\alice"]}],
            "admins":["WIN10\\alice"]
        }"#;
        let p = parse_powershell_output(json);
        assert_eq!(p.users.len(), 2);
        assert_eq!(p.users[0].username, "alice");
        assert_eq!(p.groups[0].members[0], "WIN10\\alice");
        assert_eq!(p.admins, vec!["WIN10\\alice"]);
    }

    #[test]
    fn parse_empty_yields_default() {
        let p = parse_powershell_output("");
        assert!(p.users.is_empty());
        assert!(p.groups.is_empty());
        assert!(p.admins.is_empty());
    }

    #[test]
    fn build_user_to_groups_strips_machine_prefix() {
        let groups = vec![
            Group {
                name: "Administrators".to_string(),
                gid: 0,
                members: vec!["WIN10\\alice".to_string(), "BUILTIN\\Administrator".to_string()],
            },
        ];
        let by = build_user_to_groups(&groups);
        assert!(by.contains_key("alice"));
        assert!(by.contains_key("Administrator"));
    }
}
