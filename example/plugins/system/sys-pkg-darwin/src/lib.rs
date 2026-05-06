// sys-pkg-darwin — Homebrew queries.
//
// Brew binary lives at one of two well-known paths on macOS:
//   /opt/homebrew/bin/brew   — Apple Silicon default (M1+)
//   /usr/local/bin/brew      — Intel default
//
// We try the first; on capability_denied or "not found" we fall
// through to the second. This means the Rust plugin works on
// either CPU without the operator having to know which path is
// actually deployed.

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

#[derive(Deserialize, Default)]
struct ListInstalledRequest {
    #[serde(default)]
    query: String,
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

const BREW_PATHS: &[&str] = &["/opt/homebrew/bin/brew", "/usr/local/bin/brew"];

#[cfg(target_arch = "wasm32")]
#[plugin_fn]
pub fn list_installed(req: Json<ListInstalledRequest>) -> FnResult<String> {
    let r = req.0;
    let exec_resp = match run_brew(&["list".to_string(), "--versions".to_string()], 50_000) {
        Ok(v) => v,
        Err(e) => {
            return Ok(serde_json::to_string(&ListInstalledResponse {
                packages: Vec::new(),
                backend: "brew".to_string(),
                truncated_at: 0,
                error: e,
            })?)
        }
    };
    if exec_resp.exit_code != 0 {
        return Ok(serde_json::to_string(&ListInstalledResponse {
            packages: Vec::new(),
            backend: "brew".to_string(),
            truncated_at: 0,
            error: format!(
                "brew exit {}: {}",
                exec_resp.exit_code,
                exec_resp.stderr.trim()
            ),
        })?);
    }
    let mut packages = parse_brew_list_versions(&exec_resp.stdout);
    if !r.query.is_empty() {
        packages.retain(|p| p.name.contains(&r.query));
    }
    let cap = if r.max_results == 0 {
        DEFAULT_MAX_RESULTS
    } else {
        r.max_results
    };
    let mut truncated_at = 0;
    if packages.len() > cap as usize {
        truncated_at = packages.len() as u32;
        packages.truncate(cap as usize);
    }
    Ok(serde_json::to_string(&ListInstalledResponse {
        packages,
        backend: "brew".to_string(),
        truncated_at,
        error: String::new(),
    })?)
}

#[cfg(target_arch = "wasm32")]
#[plugin_fn]
pub fn list_upgradable(_: Json<ListUpgradableRequest>) -> FnResult<String> {
    let exec_resp = match run_brew(&["outdated".to_string(), "--json=v2".to_string()], 50_000) {
        Ok(v) => v,
        Err(e) => {
            return Ok(serde_json::to_string(&ListUpgradableResponse {
                updates: Vec::new(),
                backend: "brew".to_string(),
                error: e,
            })?)
        }
    };
    if exec_resp.exit_code != 0 {
        return Ok(serde_json::to_string(&ListUpgradableResponse {
            updates: Vec::new(),
            backend: "brew".to_string(),
            error: format!(
                "brew exit {}: {}",
                exec_resp.exit_code,
                exec_resp.stderr.trim()
            ),
        })?);
    }
    let updates = parse_brew_outdated(&exec_resp.stdout);
    Ok(serde_json::to_string(&ListUpgradableResponse {
        updates,
        backend: "brew".to_string(),
        error: String::new(),
    })?)
}

// ---------- exec helper ----------

#[cfg(target_arch = "wasm32")]
fn run_brew(args: &[String], timeout_ms: u32) -> Result<ExecResponse, String> {
    let mut last_err: Option<String> = None;
    for path in BREW_PATHS {
        let req = ExecRequest {
            command: path.to_string(),
            args: args.to_vec(),
            timeout_ms,
        };
        let body = serde_json::to_string(&req).map_err(|e| format!("encode_exec_req: {}", e))?;
        let env: Envelope = unsafe {
            host_exec(body)
                .map_err(|e| format!("host_exec: {}", e))?
                .0
        };
        if !env.ok {
            // capability_denied surfaces here when neither path is in
            // the operator's allowlist. Distinct from "binary missing"
            // which surfaces as ok=false with "no such file" — fall
            // through both ways.
            last_err = Some(env.error);
            continue;
        }
        let resp: ExecResponse = serde_json::from_value(env.data)
            .map_err(|e| format!("decode_exec_resp: {}", e))?;
        // Some brew installations stub one path with an error
        // wrapper that exits non-zero with "no such file" on stderr.
        // If exit_code != 0 + stderr mentions "No such file", try
        // the next path.
        if resp.exit_code != 0
            && (resp.stderr.contains("No such file") || resp.stderr.contains("not found"))
        {
            last_err = Some(resp.stderr.trim().to_string());
            continue;
        }
        return Ok(resp);
    }
    Err(last_err.unwrap_or_else(|| "brew_not_found".to_string()))
}

#[cfg(not(target_arch = "wasm32"))]
fn run_brew(_args: &[String], _timeout_ms: u32) -> Result<ExecResponse, String> {
    Err("not available on host build".to_string())
}

// ---------- pure parsers ----------

// brew list --versions output:
//   bash 5.2.32
//   git 2.46.0
//   python@3.12 3.12.5
//   wget 1.24.5
//
// Multi-version installs:
//   ruby 3.0.6_1 3.1.4 3.2.2  (all installed kegs)
//
// We keep just the first (most recent) version per row to match
// linux's "single row per package name" shape.
pub fn parse_brew_list_versions(stdout: &str) -> Vec<Package> {
    let mut out = Vec::new();
    for line in stdout.lines() {
        let trimmed = line.trim();
        if trimmed.is_empty() {
            continue;
        }
        let mut iter = trimmed.split_whitespace();
        let name_full = match iter.next() {
            Some(s) => s,
            None => continue,
        };
        let version = match iter.next() {
            Some(s) => s.to_string(),
            None => continue,
        };
        // Tap-style "<name>@<channel>" → split into name + channel.
        let (name, channel) = match name_full.rsplit_once('@') {
            Some((n, c)) => (n.to_string(), c.to_string()),
            None => (name_full.to_string(), String::new()),
        };
        out.push(Package {
            name,
            version,
            arch: channel,
        });
    }
    out
}

// brew outdated --json=v2 output:
//   {
//     "formulae": [
//       {"name": "git", "installed_versions": ["2.45.0"], "current_version": "2.46.0", ...},
//       ...
//     ],
//     "casks": [
//       {"name": "iterm2", "installed_versions": "3.4.23", "current_version": "3.5.7", ...}
//     ]
//   }
//
// We flatten formulae + casks into a single updates list.
pub fn parse_brew_outdated(stdout: &str) -> Vec<Update> {
    let v: serde_json::Value = match serde_json::from_str(stdout.trim()) {
        Ok(v) => v,
        Err(_) => return Vec::new(),
    };
    let mut out = Vec::new();
    for key in &["formulae", "casks"] {
        let arr = match v.get(*key).and_then(|x| x.as_array()) {
            Some(a) => a,
            None => continue,
        };
        for entry in arr {
            let obj = match entry.as_object() {
                Some(o) => o,
                None => continue,
            };
            let name = obj
                .get("name")
                .and_then(|x| x.as_str())
                .unwrap_or_default()
                .to_string();
            if name.is_empty() {
                continue;
            }
            let current = first_installed_version(obj);
            let available = obj
                .get("current_version")
                .and_then(|x| x.as_str())
                .unwrap_or_default()
                .to_string();
            out.push(Update {
                name,
                current_version: current,
                available_version: available,
            });
        }
    }
    out
}

fn first_installed_version(obj: &serde_json::Map<String, serde_json::Value>) -> String {
    let v = match obj.get("installed_versions") {
        Some(v) => v,
        None => return String::new(),
    };
    if let Some(s) = v.as_str() {
        return s.to_string();
    }
    if let Some(arr) = v.as_array() {
        if let Some(first) = arr.first().and_then(|x| x.as_str()) {
            return first.to_string();
        }
    }
    String::new()
}

// ============================================================
// tests (host-build only — pure parsers)
// ============================================================

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parse_brew_list_versions_basic() {
        let stdout = "bash 5.2.32\ngit 2.46.0\npython@3.12 3.12.5\nwget 1.24.5\n";
        let got = parse_brew_list_versions(stdout);
        assert_eq!(got.len(), 4);
        assert_eq!(got[0].name, "bash");
        assert_eq!(got[0].version, "5.2.32");
        assert_eq!(got[0].arch, "");
        assert_eq!(got[2].name, "python");
        assert_eq!(got[2].arch, "3.12");
    }

    #[test]
    fn parse_brew_list_versions_first_version_only() {
        // Multi-keg row: "ruby 3.0.6_1 3.1.4 3.2.2". We keep the
        // first version only (matching linux row-per-name shape).
        let stdout = "ruby 3.0.6_1 3.1.4 3.2.2\n";
        let got = parse_brew_list_versions(stdout);
        assert_eq!(got.len(), 1);
        assert_eq!(got[0].version, "3.0.6_1");
    }

    #[test]
    fn parse_brew_list_versions_drops_short_rows() {
        let stdout = "\nlone-name-no-version\nfoo 1.0\n";
        let got = parse_brew_list_versions(stdout);
        assert_eq!(got.len(), 1);
        assert_eq!(got[0].name, "foo");
    }

    #[test]
    fn parse_brew_outdated_formulae_and_casks() {
        let stdout = r#"{
            "formulae": [
                {"name": "git", "installed_versions": ["2.45.0"], "current_version": "2.46.0"},
                {"name": "wget", "installed_versions": ["1.24.4"], "current_version": "1.24.5"}
            ],
            "casks": [
                {"name": "iterm2", "installed_versions": "3.4.23", "current_version": "3.5.7"}
            ]
        }"#;
        let got = parse_brew_outdated(stdout);
        assert_eq!(got.len(), 3);
        assert_eq!(got[0].name, "git");
        assert_eq!(got[0].current_version, "2.45.0");
        assert_eq!(got[0].available_version, "2.46.0");
        // cask uses string (not array) for installed_versions.
        assert_eq!(got[2].name, "iterm2");
        assert_eq!(got[2].current_version, "3.4.23");
    }

    #[test]
    fn parse_brew_outdated_drops_anonymous_entries() {
        let stdout = r#"{"formulae":[{"installed_versions":["1.0"],"current_version":"2.0"}]}"#;
        // Missing "name" → drop, don't panic.
        assert!(parse_brew_outdated(stdout).is_empty());
    }

    #[test]
    fn parse_brew_outdated_empty_object() {
        assert!(parse_brew_outdated(r#"{"formulae":[],"casks":[]}"#).is_empty());
    }

    #[test]
    fn parse_brew_outdated_garbage() {
        assert!(parse_brew_outdated("not json").is_empty());
        assert!(parse_brew_outdated("").is_empty());
    }
}
