// sys-fs-write bundles the plugin-ified replacements for the agent's
// fs.write-class RPCs (Mkdir, Chmod, Rename, Delete). Each method
// translates the typed wire request to the JSON envelope the host
// fn family expects.
//
// Capability: fs.write with paths=["/"] (system plugin posture).

use extism_pdk::*;
use serde::{Deserialize, Serialize};

#[derive(Deserialize)]
pub struct MkdirRequest {
    pub path: String,
    #[serde(default)]
    pub mode: u32,
    #[serde(default)]
    pub mkdirs: bool,
}

#[derive(Deserialize)]
pub struct ChmodRequest {
    pub path: String,
    pub mode: u32,
}

#[derive(Deserialize)]
pub struct DeleteRequest {
    pub path: String,
    #[serde(default)]
    pub recursive: bool,
}

#[derive(Deserialize)]
pub struct RenameRequest {
    pub from: String,
    pub to: String,
}

#[derive(Serialize, Default)]
pub struct ErrorOnlyResponse {
    #[serde(skip_serializing_if = "String::is_empty")]
    pub error: String,
}

#[host_fn("platypus")]
extern "ExtismHost" {
    fn host_fs_mkdir(req: String) -> Json<Envelope>;
    fn host_fs_chmod(req: String) -> Json<Envelope>;
    fn host_fs_delete(req: String) -> Json<Envelope>;
    fn host_fs_rename(req: String) -> Json<Envelope>;
}

#[derive(Deserialize)]
struct Envelope {
    ok: bool,
    #[serde(default)]
    error: String,
}

/// hostFsWriteJSON is the JSON shape the host fn family decodes.
/// Mirrors the Go-side fsWriteRequest struct in host_fs_write.go.
#[derive(Serialize)]
struct HostFsWriteJSON<'a> {
    path: &'a str,
    #[serde(skip_serializing_if = "is_zero_u32")]
    mode: u32,
    #[serde(skip_serializing_if = "std::ops::Not::not")]
    mkdirs: bool,
    #[serde(skip_serializing_if = "std::ops::Not::not")]
    recursive: bool,
}

#[derive(Serialize)]
struct HostFsRenameJSON<'a> {
    from: &'a str,
    to: &'a str,
}

fn is_zero_u32(x: &u32) -> bool {
    *x == 0
}

fn into_resp(env: Envelope) -> ErrorOnlyResponse {
    if env.ok {
        ErrorOnlyResponse::default()
    } else {
        ErrorOnlyResponse { error: env.error }
    }
}

#[plugin_fn]
pub fn mkdir(req: Json<MkdirRequest>) -> FnResult<Json<ErrorOnlyResponse>> {
    let body = serde_json::to_string(&HostFsWriteJSON {
        path: &req.0.path,
        mode: req.0.mode,
        mkdirs: req.0.mkdirs,
        recursive: false,
    })?;
    let env: Envelope = unsafe { host_fs_mkdir(body)?.0 };
    Ok(Json(into_resp(env)))
}

#[plugin_fn]
pub fn chmod(req: Json<ChmodRequest>) -> FnResult<Json<ErrorOnlyResponse>> {
    let body = serde_json::to_string(&HostFsWriteJSON {
        path: &req.0.path,
        mode: req.0.mode,
        mkdirs: false,
        recursive: false,
    })?;
    let env: Envelope = unsafe { host_fs_chmod(body)?.0 };
    Ok(Json(into_resp(env)))
}

#[plugin_fn]
pub fn delete(req: Json<DeleteRequest>) -> FnResult<Json<ErrorOnlyResponse>> {
    let body = serde_json::to_string(&HostFsWriteJSON {
        path: &req.0.path,
        mode: 0,
        mkdirs: false,
        recursive: req.0.recursive,
    })?;
    let env: Envelope = unsafe { host_fs_delete(body)?.0 };
    Ok(Json(into_resp(env)))
}

#[plugin_fn]
pub fn rename(req: Json<RenameRequest>) -> FnResult<Json<ErrorOnlyResponse>> {
    let body = serde_json::to_string(&HostFsRenameJSON {
        from: &req.0.from,
        to: &req.0.to,
    })?;
    let env: Envelope = unsafe { host_fs_rename(body)?.0 };
    Ok(Json(into_resp(env)))
}
