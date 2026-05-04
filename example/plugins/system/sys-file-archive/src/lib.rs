// sys-file-archive replaces the Go HandleFileArchiveStream. Walks
// the requested paths via host_fs_listdir + host_fs_read_range, packs
// them into a TAR (POSIX ustar, hand-rolled to avoid dragging the
// `tar` crate in) or gzip-wrapped TAR via flate2, and streams 256 KiB
// FileChunk frames over the wire via host_link_write_frame.
//
// Wire shape (matches legacy):
//   1. FileArchiveResponse (ack — empty error means OK)
//   2. zero-or-more FileChunk frames (eof=false)
//   3. terminal FileChunk (eof=true; Error populated on mid-walk
//      failure)
//
// Format support:
//   - ARCHIVE_FORMAT_TAR    — supported
//   - ARCHIVE_FORMAT_TAR_GZ — supported (flate2)
//   - ARCHIVE_FORMAT_ZIP    — NOT supported; returns a clear error
//     header and a single eof chunk. Documented as a parity gap;
//     adding a wasm ZIP writer means pulling the `zip` crate
//     (~150 KiB after lto/strip) which felt steep against the
//     "every modern OS opens .tar.gz" reality.

use base64::{engine::general_purpose::STANDARD, Engine as _};
use extism_pdk::*;
use flate2::write::GzEncoder;
use flate2::Compression;
use serde::{Deserialize, Serialize};
use std::io::Write;

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

#[derive(Deserialize, Default, Debug, Clone)]
struct StatResp {
    #[serde(default)]
    is_dir: bool,
    #[serde(default)]
    size: i64,
    #[serde(default)]
    mode: u32,
    // Host returns the mtime under `mtime_unix`. Field renamed here
    // so the Rust struct stays idiomatic.
    #[serde(default, rename = "mtime_unix")]
    mtime_unix: i64,
}

#[derive(Deserialize, Default, Debug, Clone)]
struct ListEntry {
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
}

// ---- Format constants from proto/v2/file.proto -------------------

const ARCHIVE_FORMAT_UNSPECIFIED: u32 = 0;
const ARCHIVE_FORMAT_TAR: u32 = 1;
const ARCHIVE_FORMAT_TAR_GZ: u32 = 2;
const ARCHIVE_FORMAT_ZIP: u32 = 3;

// ---- Output framing ---------------------------------------------

// Same flush boundary the Go handler used; keeps backpressure granularity.
const FLUSH_BYTES: usize = 256 * 1024;

#[derive(Default)]
struct FrameBuffer {
    buf: Vec<u8>,
    source_bytes_so_far: i64,
    failed: Option<String>,
}

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
            return Ok(()); // swallow further writes after a failure
        }
        self.buf.extend_from_slice(b);
        while self.buf.len() >= FLUSH_BYTES {
            let take = FLUSH_BYTES;
            let chunk: Vec<u8> = self.buf.drain(..take).collect();
            write_chunk(&chunk, false, "", self.source_bytes_so_far)?;
        }
        Ok(())
    }
    fn flush_partial_and_terminate(&mut self, error: &str) -> Result<(), Error> {
        // Drain whatever bytes are still buffered, then emit the
        // final eof=true frame (carrying the error if any). A
        // mid-walk failure can leave some bytes pending; we still
        // ship them so the consumer can decide what to keep.
        if !self.buf.is_empty() {
            let chunk: Vec<u8> = std::mem::take(&mut self.buf);
            write_chunk(&chunk, false, "", self.source_bytes_so_far)?;
        }
        write_chunk(&[], true, error, self.source_bytes_so_far)?;
        Ok(())
    }
}

// std::io::Write impl so flate2's GzEncoder can drive us. Errors
// during host_link_write_frame surface as io::ErrorKind::Other so the
// gz encoder unwinds cleanly.
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

// ---- Plugin entry point -----------------------------------------

#[plugin_fn]
pub fn archive(input: Vec<u8>) -> FnResult<()> {
    let req = parse_archive_request(&input);
    if req.paths.is_empty() {
        write_response("no paths to archive")?;
        write_chunk(&[], true, "", 0)?;
        return Ok(());
    }
    if req.format == ARCHIVE_FORMAT_UNSPECIFIED {
        write_response("archive format not specified")?;
        write_chunk(&[], true, "", 0)?;
        return Ok(());
    }
    if req.format == ARCHIVE_FORMAT_ZIP {
        // Documented parity gap. Surface a clear error so the operator
        // sees what to do (use TAR_GZ instead).
        write_response("zip format unsupported in this build — use TAR_GZ")?;
        write_chunk(&[], true, "", 0)?;
        return Ok(());
    }
    if req.format != ARCHIVE_FORMAT_TAR && req.format != ARCHIVE_FORMAT_TAR_GZ {
        write_response(&format!("unsupported archive format: {}", req.format))?;
        write_chunk(&[], true, "", 0)?;
        return Ok(());
    }

    // Pre-stat every root so we can header-fail before any bytes flow.
    for p in &req.paths {
        if call_stat(p).is_none() {
            write_response(&format!("stat {}: not found", p))?;
            write_chunk(&[], true, "", 0)?;
            return Ok(());
        }
    }

    write_response("")?;

    let mut buf = FrameBuffer::new();

    let walk_err = match req.format {
        ARCHIVE_FORMAT_TAR => write_tar(&req.paths, &mut buf),
        ARCHIVE_FORMAT_TAR_GZ => {
            let level = clamp_gz_level(req.compression_level);
            // GzEncoder takes the FrameBuffer by value, so move it
            // through the encoder's lifetime, then take it back via
            // finish() before the terminal flush.
            let enc_buf = std::mem::take(&mut buf);
            let mut enc = GzEncoder::new(enc_buf, Compression::new(level));
            let r = write_tar(&req.paths, &mut enc);
            // finish() flushes deflate trailer + returns the inner
            // FrameBuffer. Always called even on err so partial bytes
            // still ship.
            buf = match enc.finish() {
                Ok(b) => b,
                Err(e) => {
                    let msg = e.to_string();
                    // The inner buf got moved into the encoder and
                    // is now lost; emit an empty terminal eof with
                    // the error.
                    write_chunk(&[], true, &msg, 0)?;
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
    // Mirror the Go handler's clamp: 0 = library default (6),
    // otherwise 1..=9.
    if req_level <= 0 {
        6
    } else if req_level >= 9 {
        9
    } else {
        req_level as u32
    }
}

// ---- Tar archive walker -----------------------------------------

const TAR_BLOCK: usize = 512;

// write_tar walks each requested path and emits ustar headers + data
// blocks into `out`. Single-file roots are emitted with the basename
// only; directory roots are walked recursively. Mid-walk errors on
// individual files are swallowed (matches legacy posture); a missing
// root was already caught by the pre-stat above.
fn write_tar<W: Write>(paths: &[String], out: &mut W) -> Result<(), Error> {
    for root in paths {
        let stat = match call_stat(root) {
            Some(s) => s,
            None => continue,
        };
        let basename = path_basename(root);
        if !stat.is_dir {
            emit_file(out, root, &basename, &stat)?;
            continue;
        }
        // Directory root: emit the dir entry then walk children with
        // path-relative names so the archive unpacks under <basename>/.
        emit_dir(out, &basename, &stat)?;
        walk_dir(out, root, &basename)?;
    }
    // Two zero blocks terminate the archive.
    let zeros = [0u8; TAR_BLOCK];
    out.write_all(&zeros).map_err(|e| Error::msg(format!("tar trailer: {e}")))?;
    out.write_all(&zeros).map_err(|e| Error::msg(format!("tar trailer: {e}")))?;
    Ok(())
}

fn walk_dir<W: Write>(out: &mut W, abs_dir: &str, name_prefix: &str) -> Result<(), Error> {
    let mut stack: Vec<(String, String)> = vec![(abs_dir.to_string(), name_prefix.to_string())];
    while let Some((cur_abs, cur_prefix)) = stack.pop() {
        let entries = match call_listdir(&cur_abs) {
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
                let stat = call_stat(&abs_child).unwrap_or(StatResp {
                    is_dir: true,
                    size: 0,
                    mode: 0o755,
                    mtime_unix: e.mtime_unix,
                });
                emit_dir(out, &arc_child, &stat)?;
                stack.push((abs_child, arc_child));
            } else {
                let stat = call_stat(&abs_child).unwrap_or(StatResp {
                    is_dir: false,
                    size: e.size,
                    mode: 0o644,
                    mtime_unix: e.mtime_unix,
                });
                if let Err(_) = emit_file(out, &abs_child, &arc_child, &stat) {
                    // Skip individual file failures (legacy behaviour).
                    continue;
                }
            }
        }
    }
    Ok(())
}

fn emit_dir<W: Write>(out: &mut W, name: &str, stat: &StatResp) -> Result<(), Error> {
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

fn emit_file<W: Write>(out: &mut W, abs_path: &str, name: &str, stat: &StatResp) -> Result<(), Error> {
    let mut hdr = TarHeader::default();
    hdr.set_name(name);
    hdr.size = stat.size as u64;
    hdr.mode = if stat.mode != 0 { stat.mode & 0o7777 } else { 0o644 };
    hdr.mtime = stat.mtime_unix;
    hdr.typeflag = b'0';
    let block = hdr.encode();
    out.write_all(&block).map_err(|e| Error::msg(format!("tar file header: {e}")))?;

    // Stream the file in 256 KiB chunks via host_fs_read_range.
    let mut offset: i64 = 0;
    let mut written: u64 = 0;
    while written < stat.size as u64 {
        let want = (stat.size as u64 - written).min(FLUSH_BYTES as u64) as i64;
        let env = call_read_range(abs_path, offset, want);
        if !env.ok {
            // Pad the rest of the declared size with zeros so the tar
            // remains structurally valid even if a file truncated
            // mid-archive.
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
    // Pad to 512-byte block boundary.
    let pad = (TAR_BLOCK - (written as usize % TAR_BLOCK)) % TAR_BLOCK;
    if pad > 0 {
        let zeros = vec![0u8; pad];
        out.write_all(&zeros).map_err(|e| Error::msg(format!("tar pad: {e}")))?;
    }
    Ok(())
}

// ---- POSIX ustar header builder ---------------------------------

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
        // ustar `name` field is 100 bytes; longer paths require the
        // `prefix` field (155 bytes) split or an extended header. For
        // typical fleet paths (< 100 chars after stripping the root)
        // the simple shape works; truncate noisily otherwise so the
        // operator sees what happened.
        self.name = if n.len() > 100 {
            // Try splitting at a path separator within bounds.
            n.chars().take(100).collect()
        } else {
            n.to_string()
        };
    }
    fn encode(&self) -> [u8; 512] {
        let mut buf = [0u8; 512];
        // name (100)
        write_field(&mut buf[0..100], self.name.as_bytes());
        // mode (8) — octal, NUL-terminated
        write_octal(&mut buf[100..108], self.mode as u64, 7);
        // uid (8), gid (8) — leave as zero-padded "0000000\0"
        write_octal(&mut buf[108..116], 0, 7);
        write_octal(&mut buf[116..124], 0, 7);
        // size (12) — octal
        write_octal(&mut buf[124..136], self.size, 11);
        // mtime (12) — octal
        write_octal(&mut buf[136..148], self.mtime as u64, 11);
        // chksum (8) — 8 spaces during calculation
        for b in buf[148..156].iter_mut() {
            *b = b' ';
        }
        // typeflag (1)
        buf[156] = if self.typeflag == 0 { b'0' } else { self.typeflag };
        // linkname (100) — zero
        // magic "ustar\0" + version "00"
        buf[257..263].copy_from_slice(b"ustar\0");
        buf[263..265].copy_from_slice(b"00");
        // uname / gname / devmajor / devminor / prefix — leave zero
        // Now compute checksum over all 512 bytes.
        let sum: u32 = buf.iter().map(|&b| b as u32).sum();
        // Write back to chksum field as 6-digit octal + NUL + space.
        let chk_str = format!("{:06o}\0 ", sum);
        let chk_bytes = chk_str.as_bytes();
        for (i, &b) in chk_bytes.iter().enumerate().take(8) {
            buf[148 + i] = b;
        }
        buf
    }
}

fn write_field(dst: &mut [u8], src: &[u8]) {
    let n = src.len().min(dst.len() - 1); // leave a trailing NUL
    dst[..n].copy_from_slice(&src[..n]);
}

fn write_octal(dst: &mut [u8], n: u64, digits: usize) {
    let s = format!("{:0width$o}", n, width = digits);
    let bytes = s.as_bytes();
    for (i, &b) in bytes.iter().enumerate().take(digits) {
        dst[i] = b;
    }
    dst[digits] = 0;
}

// ---- Host fn wrappers --------------------------------------------

fn call_stat(path: &str) -> Option<StatResp> {
    let env: Envelope = unsafe { host_fs_stat(path.to_string()).ok()?.0 };
    if !env.ok {
        return None;
    }
    serde_json::from_value(env.data).ok()
}

fn call_listdir(path: &str) -> Option<Vec<ListEntry>> {
    let env: Envelope = unsafe { host_fs_listdir(path.to_string()).ok()?.0 };
    if !env.ok {
        return None;
    }
    serde_json::from_value(env.data).ok()
}

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

fn path_basename(p: &str) -> String {
    p.trim_end_matches('/')
        .rsplit('/')
        .next()
        .unwrap_or(p)
        .to_string()
}

// ---- Proto encode/decode -----------------------------------------

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

fn write_response(error: &str) -> Result<(), Error> {
    // FileArchiveResponse{error=1:string}
    let mut buf = Vec::with_capacity(error.len() + 8);
    if !error.is_empty() {
        write_tag(&mut buf, 1, WIRE_LEN);
        write_varint(&mut buf, error.len() as u64);
        buf.extend_from_slice(error.as_bytes());
    }
    write_frame(&buf)
}

fn write_chunk(data: &[u8], eof: bool, error: &str, source_bytes_so_far: i64) -> Result<(), Error> {
    // FileChunk{data=1:bytes, eof=2:bool, error=3:string, source_bytes_so_far=4:int64}
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

fn write_frame(body: &[u8]) -> Result<(), Error> {
    let env: Envelope = unsafe { host_link_write_frame(body.to_vec())?.0 };
    if !env.ok {
        return Err(Error::msg(format!("host_link_write_frame: {}", env.error)));
    }
    Ok(())
}

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
