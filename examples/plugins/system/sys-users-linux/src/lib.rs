// sys-users-linux — read /etc/passwd + /etc/group + /etc/sudoers
// (and sudoers.d/*) and return a UserListResponse.
//
// /etc/passwd: name:passwd:uid:gid:gecos:home:shell
// /etc/group:  name:passwd:gid:member1,member2,...
// /etc/sudoers: free-form; we extract `who    target=command` rows.

use extism_pdk::*;
use serde::{Deserialize, Serialize};

#[cfg(target_arch = "wasm32")]
#[host_fn("platypus")]
extern "ExtismHost" {
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

#[derive(Deserialize, Default)]
struct DirEntryJSON {
    name: String,
    #[serde(default)]
    is_dir: bool,
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

const SYSTEM_UID_THRESHOLD: u32 = 1000;
const NOLOGIN_SHELLS: &[&str] = &["/sbin/nologin", "/usr/sbin/nologin", "/bin/false", "/usr/bin/false"];

// ---- entry point ----

#[cfg(target_arch = "wasm32")]
#[plugin_fn]
pub fn list_users(req: Json<ListRequest>) -> FnResult<String> {
    let r = req.0;

    let passwd = read_string("/etc/passwd").unwrap_or_default();
    let group_raw = read_string("/etc/group").unwrap_or_default();

    let groups = parse_group(&group_raw);
    let group_names_by_gid: std::collections::HashMap<u32, String> = groups
        .iter()
        .map(|g| (g.gid, g.name.clone()))
        .collect();
    let groups_by_user: std::collections::HashMap<String, Vec<String>> = build_user_to_groups(&groups);

    let mut users = parse_passwd(&passwd, &group_names_by_gid, &groups_by_user);
    if !r.include_system {
        users.retain(|u| !u.is_system);
    }

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

fn parse_passwd(
    body: &str,
    group_names_by_gid: &std::collections::HashMap<u32, String>,
    groups_by_user: &std::collections::HashMap<String, Vec<String>>,
) -> Vec<User> {
    let mut out = Vec::new();
    for line in body.lines() {
        let trimmed = line.trim();
        if trimmed.is_empty() || trimmed.starts_with('#') {
            continue;
        }
        let parts: Vec<&str> = trimmed.split(':').collect();
        if parts.len() < 7 {
            continue;
        }
        let uid: u32 = parts[2].parse().unwrap_or(0);
        let gid: u32 = parts[3].parse().unwrap_or(0);
        let username = parts[0].to_string();
        let shell = parts[6].to_string();
        let mut groups: Vec<String> = Vec::new();
        if let Some(primary) = group_names_by_gid.get(&gid) {
            groups.push(primary.clone());
        }
        if let Some(supp) = groups_by_user.get(&username) {
            for g in supp {
                if !groups.iter().any(|x| x == g) {
                    groups.push(g.clone());
                }
            }
        }
        let is_system = uid < SYSTEM_UID_THRESHOLD;
        let is_locked = NOLOGIN_SHELLS.iter().any(|s| *s == shell);
        out.push(User {
            username,
            uid,
            gid,
            full_name: parts[4].split(',').next().unwrap_or("").to_string(),
            home: parts[5].to_string(),
            shell,
            is_system,
            is_locked,
            groups,
        });
    }
    out
}

fn parse_group(body: &str) -> Vec<Group> {
    let mut out = Vec::new();
    for line in body.lines() {
        let trimmed = line.trim();
        if trimmed.is_empty() || trimmed.starts_with('#') {
            continue;
        }
        let parts: Vec<&str> = trimmed.split(':').collect();
        if parts.len() < 4 {
            continue;
        }
        let gid: u32 = parts[2].parse().unwrap_or(0);
        let members: Vec<String> = if parts[3].is_empty() {
            Vec::new()
        } else {
            parts[3].split(',').map(|s| s.trim().to_string()).collect()
        };
        out.push(Group {
            name: parts[0].to_string(),
            gid,
            members,
        });
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

// parse_sudoers extracts user/group escalation rows. Skips
// `Defaults` lines, alias defs, includes. Rules look like:
//   alice  ALL=(ALL:ALL) ALL
//   %wheel ALL=(ALL) NOPASSWD: ALL
//   bob ALL=(root) NOPASSWD: /usr/bin/systemctl restart nginx
fn parse_sudoers(source: &str, body: &str) -> Vec<SudoEntry> {
    let mut out = Vec::new();
    for line in body.lines() {
        let trimmed = line.trim();
        if trimmed.is_empty() || trimmed.starts_with('#') {
            continue;
        }
        // Skip Defaults / Cmnd_Alias / User_Alias / Host_Alias /
        // include / includedir / @include directives.
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
        // First whitespace-separated token is the principal; rest is
        // the spec. A valid spec contains "ALL=" or "()=" — use that
        // as the rule signature.
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

// ---- host helpers ----

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
    use std::collections::HashMap;

    #[test]
    fn parse_passwd_basic() {
        let body = "\
root:x:0:0:root:/root:/bin/bash
daemon:x:1:1:daemon:/usr/sbin:/usr/sbin/nologin
alice:x:1000:1000:Alice Smith,,,:/home/alice:/bin/bash
";
        let groups = HashMap::from([(0, "root".to_string()), (1, "daemon".to_string()), (1000, "alice".to_string())]);
        let groups_by_user = HashMap::new();
        let users = parse_passwd(body, &groups, &groups_by_user);
        assert_eq!(users.len(), 3);

        assert_eq!(users[0].username, "root");
        assert_eq!(users[0].uid, 0);
        assert_eq!(users[0].full_name, "root");
        assert!(users[0].is_system);
        assert!(!users[0].is_locked);

        assert!(users[1].is_system);
        assert!(users[1].is_locked); // /usr/sbin/nologin

        assert!(!users[2].is_system);
        assert_eq!(users[2].full_name, "Alice Smith"); // GECOS first field only
        assert_eq!(users[2].groups, vec!["alice"]);
    }

    #[test]
    fn parse_passwd_skips_garbage() {
        let body = "\

# comment
malformed:line:without:enough:fields
root:x:0:0:root:/root:/bin/bash
";
        let users = parse_passwd(body, &HashMap::new(), &HashMap::new());
        assert_eq!(users.len(), 1);
    }

    #[test]
    fn parse_group_basic() {
        let body = "\
root:x:0:
sudo:x:27:alice,bob
docker:x:999:alice
";
        let groups = parse_group(body);
        assert_eq!(groups.len(), 3);
        assert_eq!(groups[0].name, "root");
        assert!(groups[0].members.is_empty());
        assert_eq!(groups[1].members, vec!["alice", "bob"]);
        assert_eq!(groups[2].gid, 999);
    }

    #[test]
    fn build_user_to_groups_indexes_membership() {
        let groups = vec![
            Group { name: "sudo".to_string(), gid: 27, members: vec!["alice".to_string(), "bob".to_string()] },
            Group { name: "docker".to_string(), gid: 999, members: vec!["alice".to_string()] },
        ];
        let by_user = build_user_to_groups(&groups);
        assert_eq!(by_user["alice"].len(), 2);
        assert!(by_user["alice"].contains(&"sudo".to_string()));
        assert!(by_user["alice"].contains(&"docker".to_string()));
        assert_eq!(by_user["bob"], vec!["sudo"]);
    }

    #[test]
    fn parse_sudoers_basic() {
        let body = "\
# /etc/sudoers
Defaults env_reset
Defaults mail_badpass
root ALL=(ALL:ALL) ALL
%sudo ALL=(ALL:ALL) ALL
%admin ALL=(ALL) NOPASSWD: ALL
alice ALL=(root) NOPASSWD: /usr/bin/systemctl restart nginx
@includedir /etc/sudoers.d
";
        let entries = parse_sudoers("/etc/sudoers", body);
        assert_eq!(entries.len(), 4);
        assert_eq!(entries[0].who, "root");
        assert_eq!(entries[1].who, "%sudo");
        assert_eq!(entries[2].who, "%admin");
        assert!(entries[2].how.contains("NOPASSWD"));
        assert_eq!(entries[3].who, "alice");
    }

    #[test]
    fn parse_sudoers_skips_aliases() {
        let body = "\
Cmnd_Alias REBOOT = /sbin/reboot, /sbin/halt
User_Alias OPS = alice, bob
alice ALL=(ALL) ALL
";
        let entries = parse_sudoers("/etc/sudoers", body);
        assert_eq!(entries.len(), 1);
        assert_eq!(entries[0].who, "alice");
    }
}
