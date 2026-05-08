// sys-users-darwin — macOS account inventory via dscl + sudoers.
//
// PowerShell-of-the-mac: `dscl` is the read-only Directory Services
// query tool. We use:
//   dscl . -readall /Users RealName UniqueID PrimaryGroupID
//                          NFSHomeDirectory UserShell
//   dscl . -readall /Groups PrimaryGroupID GroupMembership
//
// macOS doesn't ship a fixed UID-threshold for "system" accounts
// the way Linux does; the convention is UID < 500 = reserved
// internals (Apple-managed), UID >= 500 = humans.

use extism_pdk::*;
use serde::{Deserialize, Serialize};

#[cfg(target_arch = "wasm32")]
#[host_fn("platypus")]
extern "ExtismHost" {
    fn host_exec(req: String) -> Json<Envelope>;
    fn host_fs_read(path: String) -> Json<Envelope>;
    fn host_fs_listdir(path: String) -> Json<Envelope>;
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

#[derive(Deserialize, Default)]
struct DirEntryJSON {
    name: String,
    #[serde(default)]
    is_dir: bool,
}

// ---- request / response wire shapes (mirror sys-users-linux) ----

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

#[derive(Serialize, Default, Debug, PartialEq)]
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

#[derive(Serialize, Default, Debug, PartialEq)]
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

const DARWIN_SYSTEM_UID_THRESHOLD: u32 = 500;
const NOLOGIN_SHELLS: &[&str] = &["/usr/bin/false", "/bin/false", "/sbin/nologin", "/usr/sbin/nologin"];

// ---- entry point ----

#[cfg(target_arch = "wasm32")]
#[plugin_fn]
pub fn list_users(req: Json<ListRequest>) -> FnResult<String> {
    let r = req.0;

    // Users via dscl.
    let mut users = match exec_dscl(&[
        ".".to_string(),
        "-readall".to_string(),
        "/Users".to_string(),
        "RealName".to_string(),
        "UniqueID".to_string(),
        "PrimaryGroupID".to_string(),
        "NFSHomeDirectory".to_string(),
        "UserShell".to_string(),
        "RecordName".to_string(),
    ]) {
        Ok(stdout) => parse_dscl_users(&stdout),
        Err(_) => Vec::new(),
    };
    if !r.include_system {
        users.retain(|u| !u.is_system);
    }

    let groups = match exec_dscl(&[
        ".".to_string(),
        "-readall".to_string(),
        "/Groups".to_string(),
        "PrimaryGroupID".to_string(),
        "GroupMembership".to_string(),
        "RecordName".to_string(),
    ]) {
        Ok(stdout) => parse_dscl_groups(&stdout),
        Err(_) => Vec::new(),
    };

    // Cross-reference: fill each user's `groups` with every group
    // they're a member of.
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

    // sudoers.
    let mut sudoers = Vec::new();
    if let Some(body) = read_string("/etc/sudoers") {
        sudoers.extend(parse_sudoers("/etc/sudoers", &body));
    }
    if let Some(entries) = list_dir("/etc/sudoers.d") {
        for e in entries.into_iter().filter(|e| !e.is_dir && !e.name.starts_with('.')) {
            let path = format!("/etc/sudoers.d/{}", e.name);
            if let Some(body) = read_string(&path) {
                sudoers.extend(parse_sudoers(&path, &body));
            }
        }
    }

    Ok(serde_json::to_string(&ListResponse {
        users,
        groups,
        sudoers,
        error: String::new(),
    })?)
}

// ---- pure parsers ----

// parse_dscl_users handles `dscl . -readall /Users <attrs>` output.
// Records are separated by "RecordName: <name>" lines; each block
// has lines like "<Attr>: <value>" or, for multi-value attrs, a
// followed-by-indented-line shape:
//   GroupMembership:
//    user1 user2
fn parse_dscl_users(stdout: &str) -> Vec<User> {
    let mut out = Vec::new();
    let mut cur: Option<User> = None;
    for line in stdout.lines() {
        if let Some(rest) = line.strip_prefix("RecordName: ") {
            if let Some(u) = cur.take() {
                if !u.username.is_empty() {
                    out.push(u);
                }
            }
            cur = Some(User {
                username: rest.trim().to_string(),
                ..Default::default()
            });
            continue;
        }
        let u = match cur.as_mut() {
            Some(u) => u,
            None => continue,
        };
        if let Some(rest) = line.strip_prefix("UniqueID: ") {
            u.uid = rest.trim().parse().unwrap_or(0);
            u.is_system = u.uid < DARWIN_SYSTEM_UID_THRESHOLD;
            continue;
        }
        if let Some(rest) = line.strip_prefix("PrimaryGroupID: ") {
            u.gid = rest.trim().parse().unwrap_or(0);
            continue;
        }
        if let Some(rest) = line.strip_prefix("NFSHomeDirectory: ") {
            u.home = rest.trim().to_string();
            continue;
        }
        if let Some(rest) = line.strip_prefix("UserShell: ") {
            u.shell = rest.trim().to_string();
            u.is_locked = NOLOGIN_SHELLS.iter().any(|s| *s == u.shell);
            continue;
        }
        if let Some(rest) = line.strip_prefix("RealName: ") {
            u.full_name = rest.trim().to_string();
            continue;
        }
    }
    if let Some(u) = cur {
        if !u.username.is_empty() {
            out.push(u);
        }
    }
    out
}

fn parse_dscl_groups(stdout: &str) -> Vec<Group> {
    let mut out = Vec::new();
    let mut cur: Option<Group> = None;
    let mut in_membership = false;
    for line in stdout.lines() {
        if let Some(rest) = line.strip_prefix("RecordName: ") {
            if let Some(g) = cur.take() {
                if !g.name.is_empty() {
                    out.push(g);
                }
            }
            cur = Some(Group {
                name: rest.trim().to_string(),
                ..Default::default()
            });
            in_membership = false;
            continue;
        }
        let g = match cur.as_mut() {
            Some(g) => g,
            None => continue,
        };
        if let Some(rest) = line.strip_prefix("PrimaryGroupID: ") {
            g.gid = rest.trim().parse().unwrap_or(0);
            in_membership = false;
            continue;
        }
        // GroupMembership can appear inline ("GroupMembership: a b c")
        // or as a header followed by an indented continuation. Cover
        // both shapes.
        if let Some(rest) = line.strip_prefix("GroupMembership: ") {
            for m in rest.split_whitespace() {
                g.members.push(m.to_string());
            }
            in_membership = true;
            continue;
        }
        if line.trim_end() == "GroupMembership:" {
            in_membership = true;
            continue;
        }
        if in_membership && (line.starts_with(' ') || line.starts_with('\t')) {
            for m in line.split_whitespace() {
                g.members.push(m.to_string());
            }
            continue;
        }
        in_membership = false;
    }
    if let Some(g) = cur {
        if !g.name.is_empty() {
            out.push(g);
        }
    }
    out
}

fn build_user_to_groups(groups: &[Group]) -> std::collections::HashMap<String, Vec<String>> {
    let mut out: std::collections::HashMap<String, Vec<String>> = std::collections::HashMap::new();
    for g in groups {
        for m in &g.members {
            out.entry(m.clone()).or_default().push(g.name.clone());
        }
    }
    out
}

// parse_sudoers — same shape as the linux sibling. Kept duplicated
// rather than shared via a crate so each plugin is a single self-
// contained .wasm.
fn parse_sudoers(source: &str, body: &str) -> Vec<SudoEntry> {
    let mut out = Vec::new();
    for line in body.lines() {
        let trimmed = line.trim();
        if trimmed.is_empty() || trimmed.starts_with('#') {
            continue;
        }
        let lower = trimmed.to_ascii_lowercase();
        if lower.starts_with("defaults")
            || lower.starts_with("cmnd_alias")
            || lower.starts_with("user_alias")
            || lower.starts_with("host_alias")
            || lower.starts_with("runas_alias")
            || lower.starts_with("@include")
            || lower.starts_with("#include")
            || lower.starts_with("include")
        {
            continue;
        }
        let space = match trimmed.find(char::is_whitespace) {
            Some(i) => i,
            None => continue,
        };
        let who = trimmed[..space].trim().to_string();
        let how = trimmed[space..].trim().to_string();
        if how.is_empty() || !how.contains('=') {
            continue;
        }
        out.push(SudoEntry {
            source: source.to_string(),
            who,
            how,
        });
    }
    out
}

// ---- exec helpers ----

#[cfg(target_arch = "wasm32")]
fn exec_dscl(args: &[String]) -> Result<String, String> {
    let req = ExecRequest {
        command: "/usr/bin/dscl".to_string(),
        args: args.to_vec(),
        timeout_ms: 15_000,
    };
    let body = serde_json::to_string(&req).map_err(|e| format!("encode_exec_req: {e}"))?;
    let env: Envelope = unsafe {
        host_exec(body).map_err(|e| format!("host_exec: {e}"))?.0
    };
    if !env.ok {
        return Err(env.error);
    }
    let resp: ExecResponse = serde_json::from_value(env.data).map_err(|e| format!("decode_exec_resp: {e}"))?;
    if resp.exit_code != 0 {
        return Err(format!("dscl exit {}: {}", resp.exit_code, resp.stderr.trim()));
    }
    Ok(resp.stdout)
}

#[cfg(target_arch = "wasm32")]
fn read_string(path: &str) -> Option<String> {
    let env: Envelope = unsafe { host_fs_read(path.to_string()).ok()?.0 };
    if !env.ok { return None; }
    env.data.as_str().map(|s| s.to_string())
}

#[cfg(target_arch = "wasm32")]
fn list_dir(path: &str) -> Option<Vec<DirEntryJSON>> {
    let env: Envelope = unsafe { host_fs_listdir(path.to_string()).ok()?.0 };
    if !env.ok { return None; }
    serde_json::from_value(env.data).ok()
}

// ============================================================
// Pure-function unit tests
// ============================================================
#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parse_dscl_users_basic() {
        let stdout = "\
RecordName: alice
RealName: Alice Example
UniqueID: 501
PrimaryGroupID: 20
NFSHomeDirectory: /Users/alice
UserShell: /bin/zsh
RecordName: _ftp
RealName: FTP Daemon
UniqueID: 98
PrimaryGroupID: 98
NFSHomeDirectory: /var/empty
UserShell: /usr/bin/false
";
        let users = parse_dscl_users(stdout);
        assert_eq!(users.len(), 2);
        assert_eq!(users[0].username, "alice");
        assert_eq!(users[0].uid, 501);
        assert_eq!(users[0].gid, 20);
        assert_eq!(users[0].full_name, "Alice Example");
        assert!(!users[0].is_system);
        assert!(!users[0].is_locked);

        assert_eq!(users[1].username, "_ftp");
        assert_eq!(users[1].uid, 98);
        assert!(users[1].is_system); // <500
        assert!(users[1].is_locked); // /usr/bin/false
    }

    #[test]
    fn parse_dscl_groups_inline_membership() {
        let stdout = "\
RecordName: admin
PrimaryGroupID: 80
GroupMembership: root alice bob
RecordName: staff
PrimaryGroupID: 20
GroupMembership: alice
";
        let groups = parse_dscl_groups(stdout);
        assert_eq!(groups.len(), 2);
        assert_eq!(groups[0].name, "admin");
        assert_eq!(groups[0].gid, 80);
        assert_eq!(groups[0].members, vec!["root", "alice", "bob"]);
        assert_eq!(groups[1].members, vec!["alice"]);
    }

    #[test]
    fn parse_dscl_groups_indented_membership() {
        let stdout = "\
RecordName: admin
PrimaryGroupID: 80
GroupMembership:
 alice bob
 carol
";
        let groups = parse_dscl_groups(stdout);
        assert_eq!(groups.len(), 1);
        assert_eq!(groups[0].members.len(), 3);
        assert!(groups[0].members.contains(&"alice".to_string()));
        assert!(groups[0].members.contains(&"carol".to_string()));
    }

    #[test]
    fn parse_sudoers_smoke() {
        let body = "\
Defaults env_reset
%admin ALL=(ALL) NOPASSWD: ALL
alice ALL=(root) NOPASSWD: /usr/sbin/nginx
";
        let entries = parse_sudoers("/etc/sudoers", body);
        assert_eq!(entries.len(), 2);
        assert_eq!(entries[0].who, "%admin");
        assert_eq!(entries[1].who, "alice");
    }

    #[test]
    fn build_user_to_groups_works() {
        let groups = vec![
            Group { name: "admin".to_string(), gid: 80, members: vec!["alice".to_string(), "bob".to_string()] },
            Group { name: "wheel".to_string(), gid: 1, members: vec!["alice".to_string()] },
        ];
        let by = build_user_to_groups(&groups);
        assert_eq!(by["alice"].len(), 2);
        assert_eq!(by["bob"], vec!["admin"]);
    }
}
