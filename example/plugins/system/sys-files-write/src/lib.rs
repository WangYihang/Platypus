// sys-files-write consolidates two older plugins into a single wasm:
//
//   mkdir + chmod + delete + rename  ← was sys-fs-write
//   write (stream pump)              ← was sys-file-write (FILE_WRITE)
//
// Capabilities required (declared in plugin.yaml):
//   fs.write with paths=["/"] — same posture as the two predecessors.
//
// Wire formats stay byte-for-byte identical (FileWriteResponse +
// FileWriteResult for the stream; ErrorOnlyResponse JSON for the
// RPCs) so the server-side readers don't change.

use base64::{engine::general_purpose::STANDARD, Engine as _};
use extism_pdk::*;
use serde::{Deserialize, Serialize};

// ============================================================
// Shared host-fn declarations
// ============================================================

#[host_fn("platypus")]
extern "ExtismHost" {
    fn host_fs_mkdir(req: String) -> Json<Envelope>;
    fn host_fs_chmod(req: String) -> Json<Envelope>;
    fn host_fs_delete(req: String) -> Json<Envelope>;
    fn host_fs_rename(req: String) -> Json<Envelope>;
    fn host_fs_write_range(req: String) -> Json<Envelope>;
    fn host_link_write_frame(bytes: Vec<u8>) -> Json<Envelope>;
    fn host_link_read_frame() -> Json<Envelope>;
}

#[derive(Deserialize, Default)]
struct Envelope {
    ok: bool,
    #[serde(default)]
    data: serde_json::Value,
    #[serde(default)]
    error: String,
}

// ============================================================
// Shared proto-wire helpers
// ============================================================

const WIRE_VARINT: u32 = 0;
const WIRE_LEN: u32 = 2;

fn write_tag(buf: &mut Vec<u8>, field: u32, wire_type: u32) {
    write_varint(buf, ((field << 3) | wire_type) as u64);
}

fn write_varint(buf: &mut Vec<u8>, mut n: u64) {
    while n >= 0x80 {
        buf.push((n as u8) | 0x80);
        n >>= 7;
    }
    buf.push(n as u8);
}

fn read_varint(buf: &[u8]) -> Result<(u64, usize), Error> {
    let mut result: u64 = 0;
    let mut shift: u32 = 0;
    for (i, &b) in buf.iter().enumerate() {
        result |= ((b & 0x7f) as u64) << shift;
        if b & 0x80 == 0 {
            return Ok((result, i + 1));
        }
        shift += 7;
        if shift >= 64 {
            return Err(Error::msg("varint overflow"));
        }
    }
    Err(Error::msg("truncated varint"))
}

fn write_frame(body: &[u8]) -> Result<(), Error> {
    let env: Envelope = unsafe { host_link_write_frame(body.to_vec())?.0 };
    if !env.ok {
        return Err(Error::msg(format!("host_link_write_frame: {}", env.error)));
    }
    Ok(())
}

// ============================================================
// mkdir + chmod + delete + rename (RPCs)
// ============================================================

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

/// HostFsWriteJSON is the JSON shape the host fn family decodes.
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

// ============================================================
// write (stream)
// ============================================================
//
// Receives a FileWriteRequest as input, opens the destination via
// host_fs_write_range (truncate first call), reads incoming
// FileChunk frames from the wire via host_link_read_frame, writes
// each chunk's data through subsequent host_fs_write_range calls
// at running offsets, emits FileWriteResponse + FileWriteResult
// frames matching the legacy wire contract.

#[derive(Serialize)]
struct WriteRangeArgs {
    path: String,
    offset: i64,
    data: String, // base64
    mode: u32,
    mkdirs: bool,
    truncate: bool,
}

#[plugin_fn]
pub fn write(input: Vec<u8>) -> FnResult<()> {
    let req = parse_file_write_request(&input);
    if req.path.is_empty() {
        write_response_frame("empty path")?;
        return Ok(());
    }

    let mut offset: i64 = 0;
    if !req.append {
        if let Err(e) = call_write_range(&req.path, 0, &[], req.mode, req.mkdirs, true) {
            write_response_frame(&e)?;
            return Ok(());
        }
    } else {
        if let Err(e) = call_write_range(&req.path, 0, &[], req.mode, req.mkdirs, false) {
            write_response_frame(&e)?;
            return Ok(());
        }
    }

    write_response_frame("")?;

    let mut bytes_written: i64 = 0;
    let mut first_error = String::new();
    loop {
        let env: Envelope = unsafe { host_link_read_frame()?.0 };
        if !env.ok {
            first_error = format!("read frame: {}", env.error);
            break;
        }
        let body_b64 = env.data.as_str().unwrap_or("");
        if body_b64.is_empty() {
            first_error = "stream closed before eof chunk".into();
            break;
        }
        let body = STANDARD
            .decode(body_b64)
            .map_err(|e| WithReturnCode::new(Error::msg(format!("base64: {e}")), 2))?;
        let chunk = parse_file_chunk(&body);
        if !chunk.error.is_empty() && first_error.is_empty() {
            first_error = chunk.error.clone();
        }
        if !chunk.data.is_empty() {
            if let Err(e) = call_write_range(&req.path, offset, &chunk.data, req.mode, false, false) {
                if first_error.is_empty() {
                    first_error = format!("write @ {}: {}", offset, e);
                }
                break;
            }
            offset += chunk.data.len() as i64;
            bytes_written += chunk.data.len() as i64;
        }
        if chunk.eof {
            break;
        }
    }

    write_result_frame(bytes_written, &first_error)?;
    Ok(())
}

fn call_write_range(
    path: &str,
    offset: i64,
    data: &[u8],
    mode: u32,
    mkdirs: bool,
    truncate: bool,
) -> Result<(), String> {
    let args = WriteRangeArgs {
        path: path.to_string(),
        offset,
        data: STANDARD.encode(data),
        mode,
        mkdirs,
        truncate,
    };
    let json = serde_json::to_string(&args).map_err(|e| e.to_string())?;
    let env: Envelope = unsafe { host_fs_write_range(json).map_err(|e| e.to_string())?.0 };
    if !env.ok {
        return Err(env.error);
    }
    Ok(())
}

fn write_response_frame(error: &str) -> Result<(), Error> {
    // FileWriteResponse{error=1:string}
    let mut buf = Vec::with_capacity(error.len() + 8);
    if !error.is_empty() {
        write_tag(&mut buf, 1, WIRE_LEN);
        write_varint(&mut buf, error.len() as u64);
        buf.extend_from_slice(error.as_bytes());
    }
    write_frame(&buf)
}

fn write_result_frame(bytes_written: i64, error: &str) -> Result<(), Error> {
    // FileWriteResult{bytes_written=1:int64, error=2:string}
    let mut buf = Vec::with_capacity(error.len() + 16);
    if bytes_written != 0 {
        write_tag(&mut buf, 1, WIRE_VARINT);
        write_varint(&mut buf, bytes_written as u64);
    }
    if !error.is_empty() {
        write_tag(&mut buf, 2, WIRE_LEN);
        write_varint(&mut buf, error.len() as u64);
        buf.extend_from_slice(error.as_bytes());
    }
    write_frame(&buf)
}

// ---- proto decoders -----------------------------------------------

#[derive(Default)]
struct FileWriteRequestParsed {
    path: String,
    append: bool,
    mode: u32,
    mkdirs: bool,
}

fn parse_file_write_request(buf: &[u8]) -> FileWriteRequestParsed {
    let mut req = FileWriteRequestParsed::default();
    let mut i = 0;
    while i < buf.len() {
        let (tag, n) = match read_varint(&buf[i..]) {
            Ok(v) => v,
            Err(_) => break,
        };
        i += n;
        let field_no = (tag >> 3) as u32;
        let wire_type = (tag & 0x7) as u32;
        match (field_no, wire_type) {
            (1, WIRE_LEN) => {
                let (len, m) = match read_varint(&buf[i..]) {
                    Ok(v) => v,
                    Err(_) => break,
                };
                i += m;
                let end = i + len as usize;
                if end > buf.len() {
                    break;
                }
                req.path = String::from_utf8_lossy(&buf[i..end]).to_string();
                i = end;
            }
            (2, WIRE_VARINT) => {
                let (v, m) = match read_varint(&buf[i..]) {
                    Ok(v) => v,
                    Err(_) => break,
                };
                i += m;
                req.append = v != 0;
            }
            (3, WIRE_VARINT) => {
                let (v, m) = match read_varint(&buf[i..]) {
                    Ok(v) => v,
                    Err(_) => break,
                };
                i += m;
                req.mode = v as u32;
            }
            (4, WIRE_VARINT) => {
                let (v, m) = match read_varint(&buf[i..]) {
                    Ok(v) => v,
                    Err(_) => break,
                };
                i += m;
                req.mkdirs = v != 0;
            }
            (_, WIRE_VARINT) => {
                let (_, m) = match read_varint(&buf[i..]) {
                    Ok(v) => v,
                    Err(_) => break,
                };
                i += m;
            }
            (_, WIRE_LEN) => {
                let (len, m) = match read_varint(&buf[i..]) {
                    Ok(v) => v,
                    Err(_) => break,
                };
                i += m;
                i += len as usize;
            }
            _ => break,
        }
    }
    req
}

#[derive(Default)]
struct FileChunkParsed {
    data: Vec<u8>,
    eof: bool,
    error: String,
}

fn parse_file_chunk(buf: &[u8]) -> FileChunkParsed {
    let mut out = FileChunkParsed::default();
    let mut i = 0;
    while i < buf.len() {
        let (tag, n) = match read_varint(&buf[i..]) {
            Ok(v) => v,
            Err(_) => break,
        };
        i += n;
        let field_no = (tag >> 3) as u32;
        let wire_type = (tag & 0x7) as u32;
        match (field_no, wire_type) {
            (1, WIRE_LEN) => {
                let (len, m) = match read_varint(&buf[i..]) {
                    Ok(v) => v,
                    Err(_) => break,
                };
                i += m;
                let end = i + len as usize;
                if end > buf.len() {
                    break;
                }
                out.data = buf[i..end].to_vec();
                i = end;
            }
            (2, WIRE_VARINT) => {
                let (v, m) = match read_varint(&buf[i..]) {
                    Ok(v) => v,
                    Err(_) => break,
                };
                i += m;
                out.eof = v != 0;
            }
            (3, WIRE_LEN) => {
                let (len, m) = match read_varint(&buf[i..]) {
                    Ok(v) => v,
                    Err(_) => break,
                };
                i += m;
                let end = i + len as usize;
                if end > buf.len() {
                    break;
                }
                out.error = String::from_utf8_lossy(&buf[i..end]).to_string();
                i = end;
            }
            (_, WIRE_VARINT) => {
                let (_, m) = match read_varint(&buf[i..]) {
                    Ok(v) => v,
                    Err(_) => break,
                };
                i += m;
            }
            (_, WIRE_LEN) => {
                let (len, m) = match read_varint(&buf[i..]) {
                    Ok(v) => v,
                    Err(_) => break,
                };
                i += m;
                i += len as usize;
            }
            _ => break,
        }
    }
    out
}
