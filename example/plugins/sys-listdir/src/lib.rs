// sys-listdir is the plugin-ified replacement for the agent's
// built-in ListDir RPC. Receives a JSON request specifying the
// directory path, calls host_fs_listdir, and returns the entries
// as JSON the agent's bridge wrapper can convert back into a
// v2pb.ListDirResponse.
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
#[host_fn("platypus")]
extern "ExtismHost" {
    fn host_fs_listdir(path: String) -> Json<Envelope>;
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
