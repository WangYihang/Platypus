// sys-listdir bundles the plugin-ified replacements for the agent's
// fs.read-class RPCs (ListDir + Stat). Receives a JSON request,
// calls the matching host_fs_* function, and returns JSON the agent's
// bridge wrapper converts back into the typed proto response.
//
// Capabilities required (declared in plugin.yaml):
//   fs.read with paths=["/"] — system plugin gets unrestricted
//                              read access, mirroring the legacy
//                              handler's posture.
//
// ABI: bridge wrappers in the agent serialize/deserialize JSON for
// the wire-level proto types. See internal/agent/plugin/bridge/.

use extism_pdk::*;
use serde::{Deserialize, Serialize};

#[derive(Deserialize)]
pub struct ListDirRequest {
    pub path: String,
}

/// FileEntry mirrors the wire shape v2pb.FileEntry expects. The agent
/// bridge maps these field names directly. Mode is u32 here because
/// the legacy handler returns os.FileMode bits as-is; same encoding.
#[derive(Serialize)]
pub struct FileEntry {
    pub name: String,
    pub mode: u32,
    pub size: i64,
    #[serde(rename = "mtime_unix_nano")]
    pub mtime_unix_nano: i64,
    #[serde(rename = "symlink_target", skip_serializing_if = "String::is_empty")]
    pub symlink_target: String,
}

#[derive(Serialize)]
pub struct ListDirResponse {
    pub entries: Vec<FileEntry>,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub error: String,
}

/// host_fs_listdir lives in the platypus namespace and returns the
/// JSON envelope { ok, data: [{name, is_dir, size, mtime_unix}], error }
/// from internal/agent/plugin/host_fs.go.
///
/// host_fs_stat is the same shape but `data` is a single entry,
/// not an array.
#[host_fn("platypus")]
extern "ExtismHost" {
    fn host_fs_listdir(path: String) -> Json<Envelope>;
    fn host_fs_stat(path: String) -> Json<Envelope>;
}

#[derive(Deserialize)]
struct Envelope {
    ok: bool,
    #[serde(default)]
    data: serde_json::Value,
    #[serde(default)]
    error: String,
}

#[derive(Deserialize)]
struct HostFsEntry {
    name: String,
    is_dir: bool,
    size: i64,
    mtime_unix: i64,
}

/// StatRequest mirrors v2pb.StatRequest. One path; the response wraps
/// a single FileEntry (or an error string).
#[derive(Deserialize)]
pub struct StatRequest {
    pub path: String,
}

#[derive(Serialize, Default)]
pub struct StatResponse {
    /// Boxed-or-omitted to match v2pb.StatResponse.entry semantics
    /// (entry is nil when there's an error).
    #[serde(skip_serializing_if = "Option::is_none")]
    pub entry: Option<FileEntry>,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub error: String,
}

#[plugin_fn]
pub fn stat(req: Json<StatRequest>) -> FnResult<Json<StatResponse>> {
    let env: Envelope = unsafe { host_fs_stat(req.0.path.clone())?.0 };
    if !env.ok {
        return Ok(Json(StatResponse {
            entry: None,
            error: env.error,
        }));
    }
    let raw: HostFsEntry = match serde_json::from_value(env.data) {
        Ok(v) => v,
        Err(e) => {
            return Ok(Json(StatResponse {
                entry: None,
                error: format!("decode entry: {}", e),
            }))
        }
    };
    Ok(Json(StatResponse {
        entry: Some(FileEntry {
            name: raw.name,
            mode: if raw.is_dir { 0o040000 } else { 0o100000 },
            size: raw.size,
            mtime_unix_nano: raw.mtime_unix.saturating_mul(1_000_000_000),
            symlink_target: String::new(),
        }),
        error: String::new(),
    }))
}

#[plugin_fn]
pub fn list_dir(req: Json<ListDirRequest>) -> FnResult<Json<ListDirResponse>> {
    let env: Envelope = unsafe { host_fs_listdir(req.0.path.clone())?.0 };
    if !env.ok {
        return Ok(Json(ListDirResponse {
            entries: Vec::new(),
            error: env.error,
        }));
    }
    let raw_entries: Vec<HostFsEntry> = match serde_json::from_value(env.data) {
        Ok(v) => v,
        Err(e) => {
            return Ok(Json(ListDirResponse {
                entries: Vec::new(),
                error: format!("decode entries: {}", e),
            }))
        }
    };
    // host_fs_listdir returns mtime in seconds; the wire type wants
    // nanoseconds. Multiplying here keeps the bridge wrapper
    // schema-clean.
    let entries = raw_entries
        .into_iter()
        .map(|e| FileEntry {
            name: e.name,
            mode: if e.is_dir { 0o040000 } else { 0o100000 },
            size: e.size,
            mtime_unix_nano: e.mtime_unix.saturating_mul(1_000_000_000),
            symlink_target: String::new(),
        })
        .collect();
    Ok(Json(ListDirResponse {
        entries,
        error: String::new(),
    }))
}
