// sys-files-read consolidates four older plugins into a single wasm:
//
//   list_dir + stat (RPCs)        ← was sys-listdir
//   read (stream pump)            ← was sys-file-read     (FILE_READ)
//   scan (stream pump)            ← was sys-file-scan     (FILE_SCAN)
//   archive (stream pump)         ← was sys-file-archive  (FILE_ARCHIVE)
//
// Capabilities required (declared in plugin.yaml):
//   fs.read with paths=["/"] — same posture as the four predecessors,
//                              all of which already declared `fs.read`.
//
// Why one plugin instead of four: the operator's authorisation
// boundary is the capability set, not the plugin id. Splitting four
// fs.read consumers across four plugins forced the operator to tick
// four boxes at enroll time AND four "Install" buttons in the per-tab
// guide whenever the host needed to browse + preview + scan + archive
// — this collapses all of that into a single grant.
//
// Wire formats stay byte-for-byte identical with the older plugins
// (FileReadResponse / FileChunk / FileScanResponse / FileArchiveResponse
// + FileChunk) so the server-side readers don't change.

use base64::{engine::general_purpose::STANDARD, Engine as _};
use extism_pdk::*;
use flate2::write::GzEncoder;
use flate2::Compression;
use serde::{Deserialize, Serialize};
use std::io::Write;

// ============================================================
// Shared host-fn declarations
// ============================================================
//
// Single block declaring every host fn any of the five plugin
// methods consults. Per-method modules call into the same set, so
// this stays in lib.rs scope. Gated to wasm32 so `cargo test` on the
// host can still compile the pure helpers below (varint, tar, proto
// encoding) and exercise them in #[cfg(test)] units — the host has
// no `extism:host/env` to link against.
#[cfg(target_arch = "wasm32")]
#[host_fn("platypus")]
extern "ExtismHost" {
    fn host_fs_listdir(path: String) -> Json<Envelope>;
    fn host_fs_stat(path: String) -> Json<Envelope>;
    fn host_fs_read_range(req: String) -> Json<Envelope>;
    fn host_link_write_frame(bytes: Vec<u8>) -> Json<Envelope>;
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

#[cfg(target_arch = "wasm32")]
fn write_frame(body: &[u8]) -> Result<(), Error> {
    let env: Envelope = unsafe { host_link_write_frame(body.to_vec())?.0 };
    if !env.ok {
        return Err(Error::msg(format!("host_link_write_frame: {}", env.error)));
    }
    Ok(())
}

// ============================================================
// Shared host-fn JSON wrappers
// ============================================================

#[derive(Deserialize)]
struct HostFsEntry {
    name: String,
    is_dir: bool,
    size: i64,
    mtime_unix: i64,
}

#[derive(Deserialize, Default, Debug, Clone)]
struct StatRespFull {
    #[serde(default)]
    is_dir: bool,
    #[serde(default)]
    size: i64,
    #[serde(default)]
    mode: u32,
    #[serde(default, rename = "mtime_unix")]
    mtime_unix: i64,
    #[serde(default, rename = "name")]
    _name: String,
}

#[derive(Deserialize, Default, Debug, Clone)]
struct ListEntryFull {
    name: String,
    #[serde(default)]
    is_dir: bool,
    #[serde(default)]
    size: i64,
    #[serde(default)]
    mode: u32,
    #[serde(default, rename = "mtime_unix")]
    mtime_unix: i64,
}

#[derive(Serialize)]
struct ReadRangeArgs {
    path: String,
    offset: i64,
    length: i64,
}

#[derive(Deserialize, Default)]
struct ReadRangeResp {
    #[serde(default)]
    data: String, // base64
    #[serde(default)]
    eof: bool,
    #[serde(default)]
    size: i64,
    #[serde(default)]
    mode: u32,
}

#[cfg(target_arch = "wasm32")]
fn call_stat_full(path: &str) -> Option<StatRespFull> {
    let env: Envelope = unsafe { host_fs_stat(path.to_string()).ok()?.0 };
    if !env.ok {
        return None;
    }
    serde_json::from_value(env.data).ok()
}

#[cfg(target_arch = "wasm32")]
fn call_listdir_full(path: &str) -> Option<Vec<ListEntryFull>> {
    let env: Envelope = unsafe { host_fs_listdir(path.to_string()).ok()?.0 };
    if !env.ok {
        return None;
    }
    serde_json::from_value(env.data).ok()
}

#[cfg(target_arch = "wasm32")]
fn call_read_range(path: &str, offset: i64, length: i64) -> Envelope {
    let args = ReadRangeArgs {
        path: path.to_string(),
        offset,
        length,
    };
    let s = match serde_json::to_string(&args) {
        Ok(s) => s,
        Err(e) => {
            return Envelope {
                ok: false,
                data: serde_json::Value::Null,
                error: format!("marshal: {e}"),
            }
        }
    };
    match unsafe { host_fs_read_range(s) } {
        Ok(j) => j.0,
        Err(e) => Envelope {
            ok: false,
            data: serde_json::Value::Null,
            error: e.to_string(),
        },
    }
}

// ============================================================
// list_dir + stat (RPCs)
// ============================================================
//
// JSON in / JSON out via extism's Json<T> wrapper. The agent's
// pluginbridge layer marshals the proto request to JSON, calls these
// exports, and unmarshals the JSON response back into the proto
// reply. See internal/agent/plugin/bridge/listdir.go + stat.go.

#[derive(Deserialize)]
pub struct ListDirRequest {
    pub path: String,
}

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

#[derive(Deserialize)]
pub struct StatRequest {
    pub path: String,
}

#[derive(Serialize, Default)]
pub struct StatResponse {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub entry: Option<FileEntry>,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub error: String,
}

#[cfg(target_arch = "wasm32")]
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

#[cfg(target_arch = "wasm32")]
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

// ============================================================
// read (stream)
// ============================================================
//
// Wire format: one length-prefixed FileReadResponse (header with
// size + mode, OR error then immediate close) followed by zero or
// more FileChunk frames; the last one carries eof=true.

const FILE_CHUNK_SIZE: i64 = 256 * 1024;

#[cfg(target_arch = "wasm32")]
#[plugin_fn]
pub fn read(input: Vec<u8>) -> FnResult<()> {
    let req = match parse_file_read_request(&input) {
        Ok(r) => r,
        Err(_) => {
            write_read_header(0, 0, "parse FileReadRequest")?;
            return Ok(());
        }
    };
    if req.path.is_empty() {
        write_read_header(0, 0, "empty path")?;
        return Ok(());
    }

    // Probe size+mode before streaming any bytes. A zero-length range
    // read reports them on every call, so doing one cheap probe avoids
    // a separate stat host fn.
    let probe = call_read_range(&req.path, 0, 0);
    if !probe.ok {
        write_read_header(0, 0, &probe.error)?;
        return Ok(());
    }
    let probe_resp: ReadRangeResp = serde_json::from_value(probe.data).unwrap_or_default();
    let total_size = probe_resp.size;
    let mode = probe_resp.mode;

    let mut offset = req.offset.max(0);
    if offset > total_size {
        offset = total_size;
    }
    let mut remaining = total_size - offset;
    if req.length > 0 && req.length < remaining {
        remaining = req.length;
    }

    write_read_header(total_size, mode, "")?;

    while remaining > 0 {
        let want = remaining.min(FILE_CHUNK_SIZE);
        let env = call_read_range(&req.path, offset, want);
        if !env.ok {
            write_file_chunk(&[], true, &env.error, offset)?;
            return Ok(());
        }
        let resp: ReadRangeResp = serde_json::from_value(env.data).unwrap_or_default();
        let bytes = STANDARD
            .decode(&resp.data)
            .map_err(|e| WithReturnCode::new(Error::msg(format!("base64: {e}")), 2))?;
        let n = bytes.len() as i64;
        offset += n;
        remaining -= n;
        let is_eof = remaining == 0 || resp.eof || n == 0;
        write_file_chunk(&bytes, is_eof, "", offset)?;
        if is_eof {
            return Ok(());
        }
    }
    write_file_chunk(&[], true, "", offset)?;
    Ok(())
}

#[derive(Default)]
struct FileReadRequestParsed {
    path: String,
    offset: i64,
    length: i64,
}

fn parse_file_read_request(buf: &[u8]) -> Result<FileReadRequestParsed, Error> {
    let mut req = FileReadRequestParsed::default();
    let mut i = 0;
    while i < buf.len() {
        let (tag, n) = read_varint(&buf[i..])?;
        i += n;
        let field_no = (tag >> 3) as u32;
        let wire_type = (tag & 0x7) as u32;
        match (field_no, wire_type) {
            (1, WIRE_LEN) => {
                let (len, m) = read_varint(&buf[i..])?;
                i += m;
                let end = i + len as usize;
                if end > buf.len() {
                    return Err(Error::msg("truncated string"));
                }
                req.path = String::from_utf8_lossy(&buf[i..end]).to_string();
                i = end;
            }
            (2, WIRE_VARINT) => {
                let (v, m) = read_varint(&buf[i..])?;
                i += m;
                req.offset = v as i64;
            }
            (3, WIRE_VARINT) => {
                let (v, m) = read_varint(&buf[i..])?;
                i += m;
                req.length = v as i64;
            }
            (_, WIRE_VARINT) => {
                let (_, m) = read_varint(&buf[i..])?;
                i += m;
            }
            (_, WIRE_LEN) => {
                let (len, m) = read_varint(&buf[i..])?;
                i += m;
                i += len as usize;
            }
            _ => return Err(Error::msg("unsupported wire type")),
        }
    }
    Ok(req)
}

#[cfg(target_arch = "wasm32")]
fn write_read_header(size: i64, mode: u32, error: &str) -> Result<(), Error> {
    let mut buf = Vec::with_capacity(32);
    if size != 0 {
        write_tag(&mut buf, 1, WIRE_VARINT);
        write_varint(&mut buf, size as u64);
    }
    if mode != 0 {
        write_tag(&mut buf, 2, WIRE_VARINT);
        write_varint(&mut buf, mode as u64);
    }
    if !error.is_empty() {
        write_tag(&mut buf, 3, WIRE_LEN);
        write_varint(&mut buf, error.len() as u64);
        buf.extend_from_slice(error.as_bytes());
    }
    write_frame(&buf)
}

// FileChunk frame writer shared between `read` and `archive`. The
// proto schema is identical (data=1:bytes, eof=2:bool, error=3:string,
// source_bytes_so_far=4:int64).
#[cfg(target_arch = "wasm32")]
fn write_file_chunk(data: &[u8], eof: bool, error: &str, source_bytes_so_far: i64) -> Result<(), Error> {
    let mut buf = Vec::with_capacity(data.len() + 32);
    if !data.is_empty() {
        write_tag(&mut buf, 1, WIRE_LEN);
        write_varint(&mut buf, data.len() as u64);
        buf.extend_from_slice(data);
    }
    if eof {
        write_tag(&mut buf, 2, WIRE_VARINT);
        write_varint(&mut buf, 1);
    }
    if !error.is_empty() {
        write_tag(&mut buf, 3, WIRE_LEN);
        write_varint(&mut buf, error.len() as u64);
        buf.extend_from_slice(error.as_bytes());
    }
    if source_bytes_so_far != 0 {
        write_tag(&mut buf, 4, WIRE_VARINT);
        write_varint(&mut buf, source_bytes_so_far as u64);
    }
    write_frame(&buf)
}

// ============================================================
// scan (stream)
// ============================================================
//
// Walks the requested paths via host_fs_listdir + host_fs_stat
// (recurses into subdirectories), then emits a single
// FileScanResponse via host_link_write_frame matching the legacy
// handler's wire format.

#[derive(Default)]
struct ScanCounts {
    files: i64,
    dirs: i64,
    bytes: i64,
}

#[derive(Default, Serialize)]
struct ScanRequest {
    paths: Vec<String>,
    follow_symlinks: bool,
}

#[cfg(target_arch = "wasm32")]
#[plugin_fn]
pub fn scan(input: Vec<u8>) -> FnResult<()> {
    let req = parse_scan_request(&input);
    if req.paths.is_empty() {
        write_scan_response(0, 0, 0, "no paths to scan")?;
        return Ok(());
    }

    let mut counts = ScanCounts::default();
    for root in &req.paths {
        let root_stat = match call_stat_full(root) {
            Some(s) => s,
            None => {
                write_scan_response(0, 0, 0, &format!("stat {}: not found", root))?;
                return Ok(());
            }
        };
        if !root_stat.is_dir {
            counts.files += 1;
            counts.bytes += root_stat.size;
            continue;
        }
        counts.dirs += 1;
        scan_walk(root, &mut counts);
    }
    write_scan_response(counts.files, counts.dirs, counts.bytes, "")?;
    Ok(())
}

// scan_walk is the iterative, stack-based directory walker. wasm has
// no stack-overflow protection beyond the linear memory cap, so a
// recursive impl on an adversarially-deep tree could OOM. Iterative
// + a Vec<String> queue stays safe.
#[cfg(target_arch = "wasm32")]
fn scan_walk(root: &str, counts: &mut ScanCounts) {
    let mut stack: Vec<String> = vec![root.to_string()];
    while let Some(dir) = stack.pop() {
        let entries = match call_listdir_full(&dir) {
            Some(e) => e,
            None => continue,
        };
        for e in entries {
            let path = if dir.ends_with('/') {
                format!("{}{}", dir, e.name)
            } else {
                format!("{}/{}", dir, e.name)
            };
            if e.is_dir {
                counts.dirs += 1;
                stack.push(path);
            } else {
                counts.files += 1;
                counts.bytes += e.size;
            }
        }
    }
}

fn parse_scan_request(buf: &[u8]) -> ScanRequest {
    let mut req = ScanRequest::default();
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
                req.paths.push(String::from_utf8_lossy(&buf[i..end]).to_string());
                i = end;
            }
            (2, WIRE_VARINT) => {
                let (v, m) = match read_varint(&buf[i..]) {
                    Ok(v) => v,
                    Err(_) => break,
                };
                i += m;
                req.follow_symlinks = v != 0;
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

#[cfg(target_arch = "wasm32")]
fn write_scan_response(file_count: i64, dir_count: i64, total_bytes: i64, error: &str) -> Result<(), Error> {
    let mut buf = Vec::with_capacity(48);
    if file_count != 0 {
        write_tag(&mut buf, 1, WIRE_VARINT);
        write_varint(&mut buf, file_count as u64);
    }
    if dir_count != 0 {
        write_tag(&mut buf, 2, WIRE_VARINT);
        write_varint(&mut buf, dir_count as u64);
    }
    if total_bytes != 0 {
        write_tag(&mut buf, 3, WIRE_VARINT);
        write_varint(&mut buf, total_bytes as u64);
    }
    if !error.is_empty() {
        write_tag(&mut buf, 4, WIRE_LEN);
        write_varint(&mut buf, error.len() as u64);
        buf.extend_from_slice(error.as_bytes());
    }
    write_frame(&buf)
}

// ============================================================
// archive (stream)
// ============================================================
//
// Format support:
//   - ARCHIVE_FORMAT_TAR    — supported
//   - ARCHIVE_FORMAT_TAR_GZ — supported (flate2)
//   - ARCHIVE_FORMAT_ZIP    — NOT supported (parity gap; see
//                             sys-file-archive/plugin.yaml notes)

const ARCHIVE_FORMAT_UNSPECIFIED: u32 = 0;
const ARCHIVE_FORMAT_TAR: u32 = 1;
const ARCHIVE_FORMAT_TAR_GZ: u32 = 2;
const ARCHIVE_FORMAT_ZIP: u32 = 3;

const FLUSH_BYTES: usize = 256 * 1024;
const TAR_BLOCK: usize = 512;

#[derive(Default)]
struct FrameBuffer {
    buf: Vec<u8>,
    source_bytes_so_far: i64,
    failed: Option<String>,
}

#[cfg(target_arch = "wasm32")]
impl FrameBuffer {
    fn new() -> Self {
        Self {
            buf: Vec::with_capacity(FLUSH_BYTES + 4096),
            source_bytes_so_far: 0,
            failed: None,
        }
    }
    fn push_bytes(&mut self, b: &[u8]) -> Result<(), Error> {
        if self.failed.is_some() {
            return Ok(());
        }
        self.buf.extend_from_slice(b);
        while self.buf.len() >= FLUSH_BYTES {
            let take = FLUSH_BYTES;
            let chunk: Vec<u8> = self.buf.drain(..take).collect();
            write_file_chunk(&chunk, false, "", self.source_bytes_so_far)?;
        }
        Ok(())
    }
    fn flush_partial_and_terminate(&mut self, error: &str) -> Result<(), Error> {
        if !self.buf.is_empty() {
            let chunk: Vec<u8> = std::mem::take(&mut self.buf);
            write_file_chunk(&chunk, false, "", self.source_bytes_so_far)?;
        }
        write_file_chunk(&[], true, error, self.source_bytes_so_far)?;
        Ok(())
    }
}

#[cfg(target_arch = "wasm32")]
impl Write for FrameBuffer {
    fn write(&mut self, buf: &[u8]) -> std::io::Result<usize> {
        match self.push_bytes(buf) {
            Ok(()) => Ok(buf.len()),
            Err(e) => Err(std::io::Error::new(std::io::ErrorKind::Other, e.to_string())),
        }
    }
    fn flush(&mut self) -> std::io::Result<()> {
        Ok(())
    }
}

#[cfg(target_arch = "wasm32")]
#[plugin_fn]
pub fn archive(input: Vec<u8>) -> FnResult<()> {
    let req = parse_archive_request(&input);
    if req.paths.is_empty() {
        write_archive_header("no paths to archive")?;
        write_file_chunk(&[], true, "", 0)?;
        return Ok(());
    }
    if req.format == ARCHIVE_FORMAT_UNSPECIFIED {
        write_archive_header("archive format not specified")?;
        write_file_chunk(&[], true, "", 0)?;
        return Ok(());
    }
    if req.format == ARCHIVE_FORMAT_ZIP {
        write_archive_header("zip format unsupported in this build — use TAR_GZ")?;
        write_file_chunk(&[], true, "", 0)?;
        return Ok(());
    }
    if req.format != ARCHIVE_FORMAT_TAR && req.format != ARCHIVE_FORMAT_TAR_GZ {
        write_archive_header(&format!("unsupported archive format: {}", req.format))?;
        write_file_chunk(&[], true, "", 0)?;
        return Ok(());
    }

    for p in &req.paths {
        if call_stat_full(p).is_none() {
            write_archive_header(&format!("stat {}: not found", p))?;
            write_file_chunk(&[], true, "", 0)?;
            return Ok(());
        }
    }

    write_archive_header("")?;

    let mut buf = FrameBuffer::new();
    let walk_err = match req.format {
        ARCHIVE_FORMAT_TAR => write_tar(&req.paths, &mut buf),
        ARCHIVE_FORMAT_TAR_GZ => {
            let level = clamp_gz_level(req.compression_level);
            let enc_buf = std::mem::take(&mut buf);
            let mut enc = GzEncoder::new(enc_buf, Compression::new(level));
            let r = write_tar(&req.paths, &mut enc);
            buf = match enc.finish() {
                Ok(b) => b,
                Err(e) => {
                    let msg = e.to_string();
                    write_file_chunk(&[], true, &msg, 0)?;
                    return Ok(());
                }
            };
            r
        }
        _ => unreachable!(),
    };

    let final_err = match walk_err {
        Ok(()) => String::new(),
        Err(e) => e.to_string(),
    };
    buf.flush_partial_and_terminate(&final_err)?;
    Ok(())
}

fn clamp_gz_level(req_level: i32) -> u32 {
    if req_level <= 0 {
        6
    } else if req_level >= 9 {
        9
    } else {
        req_level as u32
    }
}

#[cfg(target_arch = "wasm32")]
fn write_tar<W: Write>(paths: &[String], out: &mut W) -> Result<(), Error> {
    for root in paths {
        let stat = match call_stat_full(root) {
            Some(s) => s,
            None => continue,
        };
        let basename = path_basename(root);
        if !stat.is_dir {
            emit_tar_file(out, root, &basename, &stat)?;
            continue;
        }
        emit_tar_dir(out, &basename, &stat)?;
        walk_tar_dir(out, root, &basename)?;
    }
    let zeros = [0u8; TAR_BLOCK];
    out.write_all(&zeros).map_err(|e| Error::msg(format!("tar trailer: {e}")))?;
    out.write_all(&zeros).map_err(|e| Error::msg(format!("tar trailer: {e}")))?;
    Ok(())
}

#[cfg(target_arch = "wasm32")]
fn walk_tar_dir<W: Write>(out: &mut W, abs_dir: &str, name_prefix: &str) -> Result<(), Error> {
    let mut stack: Vec<(String, String)> = vec![(abs_dir.to_string(), name_prefix.to_string())];
    while let Some((cur_abs, cur_prefix)) = stack.pop() {
        let entries = match call_listdir_full(&cur_abs) {
            Some(e) => e,
            None => continue,
        };
        for e in entries {
            let abs_child = if cur_abs.ends_with('/') {
                format!("{}{}", cur_abs, e.name)
            } else {
                format!("{}/{}", cur_abs, e.name)
            };
            let arc_child = if cur_prefix.is_empty() {
                e.name.clone()
            } else {
                format!("{}/{}", cur_prefix, e.name)
            };
            if e.is_dir {
                let stat = call_stat_full(&abs_child).unwrap_or(StatRespFull {
                    is_dir: true,
                    size: 0,
                    mode: 0o755,
                    mtime_unix: e.mtime_unix,
                    _name: String::new(),
                });
                emit_tar_dir(out, &arc_child, &stat)?;
                stack.push((abs_child, arc_child));
            } else {
                let stat = call_stat_full(&abs_child).unwrap_or(StatRespFull {
                    is_dir: false,
                    size: e.size,
                    mode: 0o644,
                    mtime_unix: e.mtime_unix,
                    _name: String::new(),
                });
                if let Err(_) = emit_tar_file(out, &abs_child, &arc_child, &stat) {
                    continue;
                }
            }
        }
    }
    Ok(())
}

fn emit_tar_dir<W: Write>(out: &mut W, name: &str, stat: &StatRespFull) -> Result<(), Error> {
    let mut hdr = TarHeader::default();
    hdr.set_name(&format!("{}/", name.trim_end_matches('/')));
    hdr.size = 0;
    hdr.mode = if stat.mode != 0 { stat.mode & 0o7777 } else { 0o755 };
    hdr.mtime = stat.mtime_unix;
    hdr.typeflag = b'5';
    let block = hdr.encode();
    out.write_all(&block).map_err(|e| Error::msg(format!("tar dir header: {e}")))?;
    Ok(())
}

#[cfg(target_arch = "wasm32")]
fn emit_tar_file<W: Write>(out: &mut W, abs_path: &str, name: &str, stat: &StatRespFull) -> Result<(), Error> {
    let mut hdr = TarHeader::default();
    hdr.set_name(name);
    hdr.size = stat.size as u64;
    hdr.mode = if stat.mode != 0 { stat.mode & 0o7777 } else { 0o644 };
    hdr.mtime = stat.mtime_unix;
    hdr.typeflag = b'0';
    let block = hdr.encode();
    out.write_all(&block).map_err(|e| Error::msg(format!("tar file header: {e}")))?;

    let mut offset: i64 = 0;
    let mut written: u64 = 0;
    while written < stat.size as u64 {
        let want = (stat.size as u64 - written).min(FLUSH_BYTES as u64) as i64;
        let env = call_read_range(abs_path, offset, want);
        if !env.ok {
            let pad = (stat.size as u64 - written) as usize;
            let zeros = vec![0u8; pad];
            out.write_all(&zeros).map_err(|e| Error::msg(format!("tar file pad: {e}")))?;
            written = stat.size as u64;
            break;
        }
        let resp: ReadRangeResp = serde_json::from_value(env.data).unwrap_or_default();
        let bytes = STANDARD
            .decode(&resp.data)
            .map_err(|e| Error::msg(format!("base64 decode: {e}")))?;
        out.write_all(&bytes).map_err(|e| Error::msg(format!("tar file data: {e}")))?;
        offset += bytes.len() as i64;
        written += bytes.len() as u64;
        if bytes.is_empty() || resp.eof {
            break;
        }
    }
    let pad = (TAR_BLOCK - (written as usize % TAR_BLOCK)) % TAR_BLOCK;
    if pad > 0 {
        let zeros = vec![0u8; pad];
        out.write_all(&zeros).map_err(|e| Error::msg(format!("tar pad: {e}")))?;
    }
    Ok(())
}

#[derive(Default)]
struct TarHeader {
    name: String,
    mode: u32,
    size: u64,
    mtime: i64,
    typeflag: u8,
}

impl TarHeader {
    fn set_name(&mut self, n: &str) {
        self.name = if n.len() > 100 {
            n.chars().take(100).collect()
        } else {
            n.to_string()
        };
    }
    fn encode(&self) -> [u8; 512] {
        let mut buf = [0u8; 512];
        write_tar_field(&mut buf[0..100], self.name.as_bytes());
        write_tar_octal(&mut buf[100..108], self.mode as u64, 7);
        write_tar_octal(&mut buf[108..116], 0, 7);
        write_tar_octal(&mut buf[116..124], 0, 7);
        write_tar_octal(&mut buf[124..136], self.size, 11);
        write_tar_octal(&mut buf[136..148], self.mtime as u64, 11);
        for b in buf[148..156].iter_mut() {
            *b = b' ';
        }
        buf[156] = if self.typeflag == 0 { b'0' } else { self.typeflag };
        buf[257..263].copy_from_slice(b"ustar\0");
        buf[263..265].copy_from_slice(b"00");
        let sum: u32 = buf.iter().map(|&b| b as u32).sum();
        let chk_str = format!("{:06o}\0 ", sum);
        let chk_bytes = chk_str.as_bytes();
        for (i, &b) in chk_bytes.iter().enumerate().take(8) {
            buf[148 + i] = b;
        }
        buf
    }
}

fn write_tar_field(dst: &mut [u8], src: &[u8]) {
    let n = src.len().min(dst.len() - 1);
    dst[..n].copy_from_slice(&src[..n]);
}

fn write_tar_octal(dst: &mut [u8], n: u64, digits: usize) {
    let s = format!("{:0width$o}", n, width = digits);
    let bytes = s.as_bytes();
    for (i, &b) in bytes.iter().enumerate().take(digits) {
        dst[i] = b;
    }
    dst[digits] = 0;
}

fn path_basename(p: &str) -> String {
    p.trim_end_matches('/')
        .rsplit('/')
        .next()
        .unwrap_or(p)
        .to_string()
}

#[derive(Default)]
struct ArchiveRequest {
    paths: Vec<String>,
    format: u32,
    follow_symlinks: bool,
    compression_level: i32,
}

fn parse_archive_request(buf: &[u8]) -> ArchiveRequest {
    let mut req = ArchiveRequest::default();
    let mut i = 0;
    while i < buf.len() {
        let (tag, n) = match read_varint(&buf[i..]) {
            Ok(v) => v,
            Err(_) => break,
        };
        i += n;
        let field = (tag >> 3) as u32;
        let wire = (tag & 0x7) as u32;
        match (field, wire) {
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
                req.paths.push(String::from_utf8_lossy(&buf[i..end]).to_string());
                i = end;
            }
            (2, WIRE_VARINT) => {
                let (v, m) = match read_varint(&buf[i..]) {
                    Ok(v) => v,
                    Err(_) => break,
                };
                i += m;
                req.format = v as u32;
            }
            (3, WIRE_VARINT) => {
                let (v, m) = match read_varint(&buf[i..]) {
                    Ok(v) => v,
                    Err(_) => break,
                };
                i += m;
                req.follow_symlinks = v != 0;
            }
            (4, WIRE_VARINT) => {
                let (v, m) = match read_varint(&buf[i..]) {
                    Ok(v) => v,
                    Err(_) => break,
                };
                i += m;
                req.compression_level = v as i32;
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

#[cfg(target_arch = "wasm32")]
fn write_archive_header(error: &str) -> Result<(), Error> {
    let mut buf = Vec::with_capacity(error.len() + 8);
    if !error.is_empty() {
        write_tag(&mut buf, 1, WIRE_LEN);
        write_varint(&mut buf, error.len() as u64);
        buf.extend_from_slice(error.as_bytes());
    }
    write_frame(&buf)
}

// ============================================================
// Pure-function unit tests (host build, not wasm)
// ============================================================
//
// These exercise the proto-wire helpers, tar header encoder, path
// utilities, and request parsers — every function whose behaviour
// can be observed without crossing the wasm boundary. The wasm-only
// glue (host_fn declarations, plugin_fn entries, frame writers) is
// covered end-to-end by internal/agent/plugin/files_read_integration_test.go.
//
// Run with `cargo test --lib` from this crate's directory. Tests are
// compiled for the host triple, so the #[cfg(target_arch="wasm32")]
// glue above is excluded automatically.
#[cfg(test)]
mod tests {
    use super::*;

    // ---- varint encode / decode round-trip --------------------

    #[test]
    fn varint_zero_one_byte() {
        let mut buf = Vec::new();
        write_varint(&mut buf, 0);
        assert_eq!(buf, vec![0x00]);
        let (v, n) = read_varint(&buf).unwrap();
        assert_eq!((v, n), (0, 1));
    }

    #[test]
    fn varint_127_one_byte_boundary() {
        let mut buf = Vec::new();
        write_varint(&mut buf, 127);
        assert_eq!(buf, vec![0x7f]);
    }

    #[test]
    fn varint_128_two_bytes() {
        let mut buf = Vec::new();
        write_varint(&mut buf, 128);
        // 128 = 0b1000_0000 → low 7 bits = 0, continuation = 1
        // upper 7 bits = 1
        assert_eq!(buf, vec![0x80, 0x01]);
        let (v, n) = read_varint(&buf).unwrap();
        assert_eq!((v, n), (128, 2));
    }

    #[test]
    fn varint_round_trip_powers_of_two() {
        for shift in 0..63 {
            let n: u64 = 1 << shift;
            let mut buf = Vec::new();
            write_varint(&mut buf, n);
            let (got, _) = read_varint(&buf).unwrap();
            assert_eq!(got, n, "round-trip 1<<{} failed", shift);
        }
    }

    #[test]
    fn read_varint_truncated_returns_err() {
        // 0xFF has the continuation bit set but no follow-up byte.
        assert!(read_varint(&[0xFF]).is_err());
    }

    #[test]
    fn read_varint_overflow_after_64_bits() {
        // Ten consecutive 0x80 bytes is the worst case — varints are
        // capped at 10 bytes (9 * 7 + 1 = 64 bits). The 10th byte's
        // continuation bit pushes shift past 64 and must error.
        let bad = vec![0xFF; 11];
        assert!(read_varint(&bad).is_err());
    }

    // ---- write_tag composes (field<<3)|wire_type --------------

    #[test]
    fn write_tag_packs_field_and_wire_type() {
        let mut buf = Vec::new();
        write_tag(&mut buf, 1, WIRE_LEN); // (1<<3)|2 = 10 = 0x0a
        assert_eq!(buf, vec![0x0a]);
        buf.clear();
        write_tag(&mut buf, 4, WIRE_VARINT); // (4<<3)|0 = 32 = 0x20
        assert_eq!(buf, vec![0x20]);
    }

    // ---- parse_file_read_request ------------------------------

    #[test]
    fn parse_file_read_request_path_only() {
        // {path: "/tmp/x"} encoded by hand: tag(1, LEN) varint(6) bytes
        let mut buf = Vec::new();
        write_tag(&mut buf, 1, WIRE_LEN);
        write_varint(&mut buf, 6);
        buf.extend_from_slice(b"/tmp/x");
        let req = parse_file_read_request(&buf).unwrap();
        assert_eq!(req.path, "/tmp/x");
        assert_eq!(req.offset, 0);
        assert_eq!(req.length, 0);
    }

    #[test]
    fn parse_file_read_request_path_offset_length() {
        // {path: "f", offset: 100, length: 50}
        let mut buf = Vec::new();
        write_tag(&mut buf, 1, WIRE_LEN);
        write_varint(&mut buf, 1);
        buf.push(b'f');
        write_tag(&mut buf, 2, WIRE_VARINT);
        write_varint(&mut buf, 100);
        write_tag(&mut buf, 3, WIRE_VARINT);
        write_varint(&mut buf, 50);
        let req = parse_file_read_request(&buf).unwrap();
        assert_eq!(req.path, "f");
        assert_eq!(req.offset, 100);
        assert_eq!(req.length, 50);
    }

    #[test]
    fn parse_file_read_request_skips_unknown_fields() {
        // unknown varint field (99) + path
        let mut buf = Vec::new();
        write_tag(&mut buf, 99, WIRE_VARINT);
        write_varint(&mut buf, 42);
        write_tag(&mut buf, 1, WIRE_LEN);
        write_varint(&mut buf, 1);
        buf.push(b'g');
        let req = parse_file_read_request(&buf).unwrap();
        assert_eq!(req.path, "g");
    }

    #[test]
    fn parse_file_read_request_truncated_string_errors() {
        // Tag(1,LEN) + len(10) but only 3 bytes follow.
        let mut buf = Vec::new();
        write_tag(&mut buf, 1, WIRE_LEN);
        write_varint(&mut buf, 10);
        buf.extend_from_slice(b"abc");
        assert!(parse_file_read_request(&buf).is_err());
    }

    // ---- parse_archive_request --------------------------------

    #[test]
    fn parse_archive_request_paths_and_format() {
        // ArchiveRequest{paths: ["/a","/b"], format: 2 (TAR_GZ)}
        let mut buf = Vec::new();
        for p in ["/a", "/b"] {
            write_tag(&mut buf, 1, WIRE_LEN);
            write_varint(&mut buf, p.len() as u64);
            buf.extend_from_slice(p.as_bytes());
        }
        write_tag(&mut buf, 2, WIRE_VARINT);
        write_varint(&mut buf, ARCHIVE_FORMAT_TAR_GZ as u64);
        let req = parse_archive_request(&buf);
        assert_eq!(req.paths, vec!["/a".to_string(), "/b".to_string()]);
        assert_eq!(req.format, ARCHIVE_FORMAT_TAR_GZ);
    }

    // ---- TarHeader.encode ------------------------------------

    #[test]
    fn tar_header_basic_file() {
        let mut hdr = TarHeader::default();
        hdr.set_name("hello.txt");
        hdr.size = 11;
        hdr.mode = 0o644;
        hdr.mtime = 1700000000;
        hdr.typeflag = b'0';
        let block = hdr.encode();
        assert_eq!(block.len(), 512);
        // Name in the first 100 bytes.
        assert_eq!(&block[..9], b"hello.txt");
        assert_eq!(block[9], 0u8); // null terminator
        // ustar magic at 257..263.
        assert_eq!(&block[257..263], b"ustar\0");
        assert_eq!(&block[263..265], b"00");
        // Typeflag at 156.
        assert_eq!(block[156], b'0');
    }

    #[test]
    fn tar_header_directory_typeflag() {
        let mut hdr = TarHeader::default();
        hdr.set_name("dir/");
        hdr.typeflag = b'5';
        let block = hdr.encode();
        assert_eq!(block[156], b'5');
    }

    #[test]
    fn tar_header_truncates_long_name() {
        let mut hdr = TarHeader::default();
        let long_name: String = "x".repeat(150);
        hdr.set_name(&long_name);
        // set_name caps to 100 chars; encode's write_tar_field writes
        // only dst.len() - 1 = 99 bytes into the 100-byte slot,
        // leaving byte 99 as a null terminator.
        let block = hdr.encode();
        for (i, b) in block[..99].iter().enumerate() {
            assert_eq!(*b, b'x', "byte {} = {}", i, b);
        }
        assert_eq!(block[99], 0u8, "byte 99 must be null terminator");
    }

    #[test]
    fn tar_header_checksum_is_octal_string() {
        let mut hdr = TarHeader::default();
        hdr.set_name("f");
        hdr.size = 5;
        hdr.mode = 0o644;
        hdr.typeflag = b'0';
        let block = hdr.encode();
        let chk = &block[148..156];
        // Format: "NNNNNN\0 " — six octal digits, a NUL, a space.
        assert_eq!(chk[6], 0u8);
        assert_eq!(chk[7], b' ');
        for b in &chk[..6] {
            assert!(b.is_ascii_digit() && *b < b'8',
                "checksum digit out of octal range: {}", b);
        }
    }

    // ---- path_basename ---------------------------------------

    #[test]
    fn basename_drops_trailing_slash() {
        assert_eq!(path_basename("/var/log/"), "log");
        assert_eq!(path_basename("/var/log"), "log");
        assert_eq!(path_basename("foo"), "foo");
    }

    #[test]
    fn basename_root_returns_empty() {
        // p = "/" → trim_end_matches('/') → "" → rsplit('/').next() → ""
        assert_eq!(path_basename("/"), "");
    }

    // ---- clamp_gz_level --------------------------------------

    #[test]
    fn clamp_gz_level_default_when_zero() {
        assert_eq!(clamp_gz_level(0), 6);
        assert_eq!(clamp_gz_level(-3), 6);
    }

    #[test]
    fn clamp_gz_level_caps_at_nine() {
        assert_eq!(clamp_gz_level(9), 9);
        assert_eq!(clamp_gz_level(50), 9);
    }

    #[test]
    fn clamp_gz_level_passes_in_range() {
        for level in 1..=8 {
            assert_eq!(clamp_gz_level(level), level as u32);
        }
    }
}
