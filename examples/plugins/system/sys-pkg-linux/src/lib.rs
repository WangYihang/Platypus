// sys-pkg-linux — multi-backend package manager queries.
//
// Backend detection: probe via `/bin/sh -c "command -v <bin>"`,
// pick the first found in priority order. apt is preferred over
// dnf where both exist (no system has both natively, but a dnf
// userspace + apt symlink is conceivable on weird derivatives).
//
// All RPCs route through a single `query_backend` helper that
// dispatches the per-backend command and parses the per-backend
// output format into a unified wire shape. Tests cover the
// pure parsers per backend.

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

// ---------- request / response wire shapes ----------

#[derive(Deserialize, Default)]
struct ListInstalledRequest {
    /// Substring filter against package name. Empty = all.
    #[serde(default)]
    query: String,
    /// Cap on returned entries. 0 = use plugin default (5000).
    #[serde(default)]
    max_results: u32,
}

#[derive(Deserialize, Default)]
struct ListUpgradableRequest {}

#[derive(Serialize, Default)]
pub struct Package {
    pub name: String,
    pub version: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub arch: String,
}

#[derive(Serialize, Default)]
pub struct Update {
    pub name: String,
    #[serde(rename = "currentVersion", skip_serializing_if = "String::is_empty")]
    pub current_version: String,
    #[serde(rename = "availableVersion")]
    pub available_version: String,
}

#[derive(Serialize, Default)]
struct ListInstalledResponse {
    packages: Vec<Package>,
    backend: String,
    #[serde(skip_serializing_if = "is_zero_u32")]
    truncated_at: u32,
    #[serde(skip_serializing_if = "String::is_empty")]
    error: String,
}

#[derive(Serialize, Default)]
struct ListUpgradableResponse {
    updates: Vec<Update>,
    backend: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    error: String,
}

fn is_zero_u32(n: &u32) -> bool {
    *n == 0
}

const DEFAULT_MAX_RESULTS: u32 = 5_000;

// ---------- backend descriptor ----------

#[derive(Debug, PartialEq, Clone, Copy)]
pub enum Backend {
    Apt,
    Dnf,
    Yum,
    Zypper,
    Pacman,
}

impl Backend {
    pub fn name(&self) -> &'static str {
        match self {
            Backend::Apt => "apt",
            Backend::Dnf => "dnf",
            Backend::Yum => "yum",
            Backend::Zypper => "zypper",
            Backend::Pacman => "pacman",
        }
    }

    /// Probe binary the detector greps for via `command -v`.
    pub fn probe_bin(&self) -> &'static str {
        match self {
            Backend::Apt => "dpkg-query",
            Backend::Dnf => "dnf",
            Backend::Yum => "yum",
            Backend::Zypper => "zypper",
            Backend::Pacman => "pacman",
        }
    }
}

const BACKEND_PRIORITY: &[Backend] = &[
    Backend::Apt,
    Backend::Dnf,
    Backend::Yum,
    Backend::Zypper,
    Backend::Pacman,
];

// ---------- entry points ----------

#[cfg(target_arch = "wasm32")]
#[plugin_fn]
pub fn list_installed(req: Json<ListInstalledRequest>) -> FnResult<String> {
    let r = req.0;
    let backend = match detect_backend() {
        Some(b) => b,
        None => {
            return Ok(serde_json::to_string(&ListInstalledResponse {
                packages: Vec::new(),
                backend: String::new(),
                truncated_at: 0,
                error: "no_supported_package_manager".to_string(),
            })?)
        }
    };

    let exec_resp = match run_list_installed(backend, 25_000) {
        Ok(v) => v,
        Err(e) => {
            return Ok(serde_json::to_string(&ListInstalledResponse {
                packages: Vec::new(),
                backend: backend.name().to_string(),
                truncated_at: 0,
                error: e,
            })?)
        }
    };
    if exec_resp.exit_code != 0 {
        return Ok(serde_json::to_string(&ListInstalledResponse {
            packages: Vec::new(),
            backend: backend.name().to_string(),
            truncated_at: 0,
            error: format!(
                "{} exit {}: {}",
                backend.name(),
                exec_resp.exit_code,
                exec_resp.stderr.trim()
            ),
        })?);
    }
    let mut packages = parse_list_installed(backend, &exec_resp.stdout);
    if !r.query.is_empty() {
        packages.retain(|p| p.name.contains(&r.query));
    }
    let cap = if r.max_results == 0 {
        DEFAULT_MAX_RESULTS
    } else {
        r.max_results
    };
    let mut truncated_at: u32 = 0;
    if packages.len() > cap as usize {
        truncated_at = packages.len() as u32;
        packages.truncate(cap as usize);
    }
    Ok(serde_json::to_string(&ListInstalledResponse {
        packages,
        backend: backend.name().to_string(),
        truncated_at,
        error: String::new(),
    })?)
}

#[cfg(target_arch = "wasm32")]
#[plugin_fn]
pub fn list_upgradable(_: Json<ListUpgradableRequest>) -> FnResult<String> {
    let backend = match detect_backend() {
        Some(b) => b,
        None => {
            return Ok(serde_json::to_string(&ListUpgradableResponse {
                updates: Vec::new(),
                backend: String::new(),
                error: "no_supported_package_manager".to_string(),
            })?)
        }
    };
    let exec_resp = match run_list_upgradable(backend, 25_000) {
        Ok(v) => v,
        Err(e) => {
            return Ok(serde_json::to_string(&ListUpgradableResponse {
                updates: Vec::new(),
                backend: backend.name().to_string(),
                error: e,
            })?)
        }
    };
    if exec_resp.exit_code != 0 && exec_resp.stderr.trim().contains("error") {
        return Ok(serde_json::to_string(&ListUpgradableResponse {
            updates: Vec::new(),
            backend: backend.name().to_string(),
            error: format!(
                "{} exit {}: {}",
                backend.name(),
                exec_resp.exit_code,
                exec_resp.stderr.trim()
            ),
        })?);
    }
    let updates = parse_list_upgradable(backend, &exec_resp.stdout);
    Ok(serde_json::to_string(&ListUpgradableResponse {
        updates,
        backend: backend.name().to_string(),
        error: String::new(),
    })?)
}

// ---------- backend detection ----------

#[cfg(target_arch = "wasm32")]
fn detect_backend() -> Option<Backend> {
    for b in BACKEND_PRIORITY {
        let probe_cmd = format!("command -v {}", b.probe_bin());
        let req = ExecRequest {
            command: "/bin/sh".to_string(),
            args: vec!["-c".to_string(), probe_cmd],
            timeout_ms: 3_000,
        };
        let body = serde_json::to_string(&req).ok()?;
        let env: Envelope = unsafe { host_exec(body).ok()?.0 };
        if !env.ok {
            continue;
        }
        let resp: ExecResponse = match serde_json::from_value(env.data) {
            Ok(v) => v,
            Err(_) => continue,
        };
        // command -v exits 0 + writes the path on success.
        if resp.exit_code == 0 && !resp.stdout.trim().is_empty() {
            return Some(*b);
        }
    }
    None
}

// ---------- per-backend exec helpers ----------

#[cfg(target_arch = "wasm32")]
fn run_list_installed(backend: Backend, timeout_ms: u32) -> Result<ExecResponse, String> {
    let (command, args) = match backend {
        Backend::Apt => (
            "/usr/bin/dpkg-query".to_string(),
            vec![
                "-W".to_string(),
                "-f=${Package}\\t${Version}\\t${Architecture}\\n".to_string(),
            ],
        ),
        Backend::Dnf => (
            "/usr/bin/dnf".to_string(),
            vec![
                "list".to_string(),
                "--installed".to_string(),
                "-q".to_string(),
            ],
        ),
        Backend::Yum => (
            "/usr/bin/yum".to_string(),
            vec![
                "list".to_string(),
                "installed".to_string(),
                "-q".to_string(),
            ],
        ),
        Backend::Zypper => (
            "/usr/bin/zypper".to_string(),
            vec![
                "--non-interactive".to_string(),
                "se".to_string(),
                "-i".to_string(),
            ],
        ),
        Backend::Pacman => (
            "/usr/bin/pacman".to_string(),
            vec!["-Q".to_string()],
        ),
    };
    let req = ExecRequest {
        command,
        args,
        timeout_ms,
    };
    let body = serde_json::to_string(&req).map_err(|e| format!("encode_exec_req: {}", e))?;
    let env: Envelope = unsafe {
        host_exec(body)
            .map_err(|e| format!("host_exec: {}", e))?
            .0
    };
    if !env.ok {
        return Err(env.error);
    }
    serde_json::from_value(env.data).map_err(|e| format!("decode_exec_resp: {}", e))
}

#[cfg(target_arch = "wasm32")]
fn run_list_upgradable(backend: Backend, timeout_ms: u32) -> Result<ExecResponse, String> {
    let (command, args) = match backend {
        Backend::Apt => (
            "/usr/bin/apt".to_string(),
            vec!["list".to_string(), "--upgradable".to_string()],
        ),
        Backend::Dnf => (
            "/usr/bin/dnf".to_string(),
            vec![
                "list".to_string(),
                "--upgrades".to_string(),
                "-q".to_string(),
            ],
        ),
        Backend::Yum => (
            "/usr/bin/yum".to_string(),
            vec!["check-update".to_string(), "-q".to_string()],
        ),
        Backend::Zypper => (
            "/usr/bin/zypper".to_string(),
            vec![
                "--non-interactive".to_string(),
                "list-updates".to_string(),
            ],
        ),
        Backend::Pacman => (
            "/usr/bin/pacman".to_string(),
            vec!["-Qu".to_string()],
        ),
    };
    let req = ExecRequest {
        command,
        args,
        timeout_ms,
    };
    let body = serde_json::to_string(&req).map_err(|e| format!("encode_exec_req: {}", e))?;
    let env: Envelope = unsafe {
        host_exec(body)
            .map_err(|e| format!("host_exec: {}", e))?
            .0
    };
    if !env.ok {
        return Err(env.error);
    }
    serde_json::from_value(env.data).map_err(|e| format!("decode_exec_resp: {}", e))
}

#[cfg(not(target_arch = "wasm32"))]
fn detect_backend() -> Option<Backend> {
    None
}

// ---------- per-backend pure parsers ----------

pub fn parse_list_installed(backend: Backend, stdout: &str) -> Vec<Package> {
    match backend {
        Backend::Apt => parse_apt_installed(stdout),
        Backend::Dnf | Backend::Yum => parse_dnf_installed(stdout),
        Backend::Zypper => parse_zypper_installed(stdout),
        Backend::Pacman => parse_pacman_installed(stdout),
    }
}

pub fn parse_list_upgradable(backend: Backend, stdout: &str) -> Vec<Update> {
    match backend {
        Backend::Apt => parse_apt_upgradable(stdout),
        Backend::Dnf | Backend::Yum => parse_dnf_upgradable(stdout),
        Backend::Zypper => parse_zypper_upgradable(stdout),
        Backend::Pacman => parse_pacman_upgradable(stdout),
    }
}

// dpkg-query -W -f='${Package}\t${Version}\t${Architecture}\n'
fn parse_apt_installed(stdout: &str) -> Vec<Package> {
    let mut out = Vec::new();
    for line in stdout.lines() {
        let parts: Vec<&str> = line.split('\t').collect();
        if parts.len() < 2 || parts[0].is_empty() {
            continue;
        }
        out.push(Package {
            name: parts[0].to_string(),
            version: parts[1].to_string(),
            arch: parts.get(2).map(|s| s.to_string()).unwrap_or_default(),
        });
    }
    out
}

// dnf list --installed -q output:
//   Installed Packages
//   bash.x86_64                 5.2.21-1.fc40    @anaconda
//   coreutils.x86_64            9.4-7.fc40       @System
fn parse_dnf_installed(stdout: &str) -> Vec<Package> {
    let mut out = Vec::new();
    for line in stdout.lines() {
        let trimmed = line.trim();
        if trimmed.is_empty() || trimmed.ends_with("Packages") || trimmed.starts_with("Last metadata") {
            continue;
        }
        let cols: Vec<&str> = trimmed.split_whitespace().collect();
        if cols.len() < 2 {
            continue;
        }
        // First column is "<name>.<arch>"; rsplit the dot to recover.
        let name_arch = cols[0];
        let (name, arch) = match name_arch.rsplit_once('.') {
            Some((n, a)) => (n.to_string(), a.to_string()),
            None => (name_arch.to_string(), String::new()),
        };
        let version = cols[1].to_string();
        out.push(Package { name, version, arch });
    }
    out
}

// zypper se -i output (tabular with " | " separators):
//   S | Name           | Type    | Version | Arch  | Repository
//   --+----------------+---------+---------+-------+-----------
//   i | bash           | package | 5.2.21  | x86_64| openSUSE
fn parse_zypper_installed(stdout: &str) -> Vec<Package> {
    let mut out = Vec::new();
    for line in stdout.lines() {
        if !line.contains('|') {
            continue;
        }
        let cols: Vec<&str> = line.split('|').map(|s| s.trim()).collect();
        // Expect at least 5 columns (S, Name, Type, Version, Arch).
        if cols.len() < 5 {
            continue;
        }
        // Skip header / separator rows.
        if cols[0] == "S" || cols[0].chars().all(|c| c == '-' || c == '+') {
            continue;
        }
        let name = cols[1].to_string();
        let version = cols[3].to_string();
        let arch = cols[4].to_string();
        if name.is_empty() {
            continue;
        }
        out.push(Package { name, version, arch });
    }
    out
}

// pacman -Q output: "<name> <version>" per line.
fn parse_pacman_installed(stdout: &str) -> Vec<Package> {
    let mut out = Vec::new();
    for line in stdout.lines() {
        let mut iter = line.split_whitespace();
        let name = match iter.next() {
            Some(s) => s.to_string(),
            None => continue,
        };
        let version = iter.next().map(|s| s.to_string()).unwrap_or_default();
        if name.is_empty() {
            continue;
        }
        // pacman doesn't expose arch in -Q; leave empty.
        out.push(Package {
            name,
            version,
            arch: String::new(),
        });
    }
    out
}

// apt list --upgradable output:
//   Listing...
//   bash/jammy-updates 5.1-6ubuntu1.1 amd64 [upgradable from: 5.1-6ubuntu1]
fn parse_apt_upgradable(stdout: &str) -> Vec<Update> {
    let mut out = Vec::new();
    for line in stdout.lines() {
        let trimmed = line.trim();
        if trimmed.is_empty() || trimmed.starts_with("Listing") {
            continue;
        }
        // Split by whitespace. First token is "<name>/<repo>".
        let cols: Vec<&str> = trimmed.split_whitespace().collect();
        if cols.len() < 2 {
            continue;
        }
        let name = match cols[0].split_once('/') {
            Some((n, _)) => n.to_string(),
            None => cols[0].to_string(),
        };
        let available_version = cols[1].to_string();
        // Tail "[upgradable from: <ver>]" carries the current version.
        let current = trimmed
            .find("upgradable from:")
            .and_then(|i| trimmed[i + "upgradable from:".len()..].trim().strip_suffix(']'))
            .map(|s| s.trim().to_string())
            .unwrap_or_default();
        out.push(Update {
            name,
            current_version: current,
            available_version,
        });
    }
    out
}

// dnf list --upgrades -q output (similar shape to --installed):
//   Available Upgrades
//   bash.x86_64    5.2.22-1.fc40    updates
fn parse_dnf_upgradable(stdout: &str) -> Vec<Update> {
    let mut out = Vec::new();
    for line in stdout.lines() {
        let trimmed = line.trim();
        if trimmed.is_empty() || trimmed.ends_with("Upgrades") || trimmed.ends_with("Updates") {
            continue;
        }
        let cols: Vec<&str> = trimmed.split_whitespace().collect();
        if cols.len() < 2 {
            continue;
        }
        let name = match cols[0].rsplit_once('.') {
            Some((n, _)) => n.to_string(),
            None => cols[0].to_string(),
        };
        out.push(Update {
            name,
            current_version: String::new(),
            available_version: cols[1].to_string(),
        });
    }
    out
}

// zypper list-updates output (tabular | -separated):
//   S | Repository | Name | Current Version | Available Version | Arch
fn parse_zypper_upgradable(stdout: &str) -> Vec<Update> {
    let mut out = Vec::new();
    for line in stdout.lines() {
        if !line.contains('|') {
            continue;
        }
        let cols: Vec<&str> = line.split('|').map(|s| s.trim()).collect();
        if cols.len() < 5 {
            continue;
        }
        if cols[0] == "S" || cols[0].chars().all(|c| c == '-' || c == '+') {
            continue;
        }
        // S | Repository | Name | Current Version | Available Version | Arch
        let name = cols[2].to_string();
        let current = cols[3].to_string();
        let available = cols[4].to_string();
        if name.is_empty() {
            continue;
        }
        out.push(Update {
            name,
            current_version: current,
            available_version: available,
        });
    }
    out
}

// pacman -Qu output: "<name> <current> -> <new>"
fn parse_pacman_upgradable(stdout: &str) -> Vec<Update> {
    let mut out = Vec::new();
    for line in stdout.lines() {
        let cols: Vec<&str> = line.split_whitespace().collect();
        if cols.len() < 4 || cols[2] != "->" {
            continue;
        }
        out.push(Update {
            name: cols[0].to_string(),
            current_version: cols[1].to_string(),
            available_version: cols[3].to_string(),
        });
    }
    out
}

// ============================================================
// tests (host-build only — pure parsers)
// ============================================================

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parse_apt_installed_basic() {
        let stdout = "bash\t5.1-6ubuntu1\tamd64\ncoreutils\t8.32-4.1ubuntu1\tamd64\nlibc6\t2.35-0ubuntu3.1\tamd64\n";
        let got = parse_apt_installed(stdout);
        assert_eq!(got.len(), 3);
        assert_eq!(got[0].name, "bash");
        assert_eq!(got[0].version, "5.1-6ubuntu1");
        assert_eq!(got[0].arch, "amd64");
        assert_eq!(got[2].name, "libc6");
    }

    #[test]
    fn parse_apt_installed_drops_short_rows() {
        let stdout = "\nbash\nfoo\t1.0\n";
        let got = parse_apt_installed(stdout);
        // First two have <2 fields; third is valid.
        assert_eq!(got.len(), 1);
        assert_eq!(got[0].name, "foo");
    }

    #[test]
    fn parse_dnf_installed_basic() {
        let stdout = "Installed Packages\nbash.x86_64                 5.2.21-1.fc40    @anaconda\ncoreutils.x86_64            9.4-7.fc40       @System\n";
        let got = parse_dnf_installed(stdout);
        assert_eq!(got.len(), 2);
        assert_eq!(got[0].name, "bash");
        assert_eq!(got[0].arch, "x86_64");
        assert_eq!(got[0].version, "5.2.21-1.fc40");
    }

    #[test]
    fn parse_dnf_installed_skips_metadata_line() {
        let stdout = "Last metadata expiration check: 0:01 ago.\nInstalled Packages\nbash.x86_64    5.2.21-1.fc40    @anaconda\n";
        let got = parse_dnf_installed(stdout);
        assert_eq!(got.len(), 1);
    }

    #[test]
    fn parse_pacman_installed_basic() {
        let stdout = "bash 5.2.026-2\ncoreutils 9.5-1\nlibc 2.40-1\n";
        let got = parse_pacman_installed(stdout);
        assert_eq!(got.len(), 3);
        assert_eq!(got[0].name, "bash");
        assert_eq!(got[1].version, "9.5-1");
    }

    #[test]
    fn parse_zypper_installed_basic() {
        let stdout = "S | Name           | Type    | Version | Arch    | Repository
--+----------------+---------+---------+---------+-----------
i | bash           | package | 5.2.21  | x86_64  | openSUSE
i | coreutils      | package | 9.4     | x86_64  | openSUSE
";
        let got = parse_zypper_installed(stdout);
        assert_eq!(got.len(), 2);
        assert_eq!(got[0].name, "bash");
        assert_eq!(got[0].version, "5.2.21");
        assert_eq!(got[0].arch, "x86_64");
    }

    #[test]
    fn parse_apt_upgradable_basic() {
        let stdout = "Listing...\nbash/jammy-updates 5.1-6ubuntu1.1 amd64 [upgradable from: 5.1-6ubuntu1]\nlibc6/jammy-updates 2.35-0ubuntu3.1 amd64 [upgradable from: 2.35-0ubuntu3]\n";
        let got = parse_apt_upgradable(stdout);
        assert_eq!(got.len(), 2);
        assert_eq!(got[0].name, "bash");
        assert_eq!(got[0].available_version, "5.1-6ubuntu1.1");
        assert_eq!(got[0].current_version, "5.1-6ubuntu1");
    }

    #[test]
    fn parse_pacman_upgradable_basic() {
        let stdout = "linux 6.7.0 -> 6.8.0\nbash 5.2.026 -> 5.2.030\n";
        let got = parse_pacman_upgradable(stdout);
        assert_eq!(got.len(), 2);
        assert_eq!(got[0].name, "linux");
        assert_eq!(got[0].current_version, "6.7.0");
        assert_eq!(got[0].available_version, "6.8.0");
    }

    #[test]
    fn parse_dnf_upgradable_basic() {
        let stdout = "Available Upgrades\nbash.x86_64    5.2.22-1.fc40    updates\nkernel.x86_64  6.8.4-200.fc40   updates\n";
        let got = parse_dnf_upgradable(stdout);
        assert_eq!(got.len(), 2);
        assert_eq!(got[0].name, "bash");
        assert_eq!(got[0].available_version, "5.2.22-1.fc40");
    }

    #[test]
    fn parse_zypper_upgradable_basic() {
        let stdout = "S | Repository | Name | Current Version | Available Version | Arch
--+------------+------+-----------------+-------------------+----
v | tumbleweed | bash | 5.2.21          | 5.2.22            | x86_64
";
        let got = parse_zypper_upgradable(stdout);
        assert_eq!(got.len(), 1);
        assert_eq!(got[0].name, "bash");
        assert_eq!(got[0].current_version, "5.2.21");
        assert_eq!(got[0].available_version, "5.2.22");
    }

    #[test]
    fn parse_dispatcher_chooses_per_backend() {
        // Dispatch table sanity: same hand-rolled fixture parses
        // differently per backend.
        let pkgs = parse_list_installed(Backend::Pacman, "bash 5.2.026-2\n");
        assert_eq!(pkgs.len(), 1);
        let updates = parse_list_upgradable(Backend::Pacman, "bash 5.2.026 -> 5.2.030\n");
        assert_eq!(updates.len(), 1);
    }

    #[test]
    fn backend_names_are_stable() {
        assert_eq!(Backend::Apt.name(), "apt");
        assert_eq!(Backend::Pacman.name(), "pacman");
        assert_eq!(Backend::Apt.probe_bin(), "dpkg-query");
    }
}
