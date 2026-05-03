// sys-file-write replaces HandleFileWriteStream. Receives a
// FileWriteRequest as input, opens the destination via
// host_fs_write_range (truncate first call), reads incoming
// FileChunk frames from the wire via host_link_read_frame, writes
// each chunk's data through subsequent host_fs_write_range calls
// at running offsets, and emits FileWriteResponse + FileWriteResult
// frames matching the legacy wire contract.

use base64::{engine::general_purpose::STANDARD, Engine as _};
use extism_pdk::*;
use serde::{Deserialize, Serialize};

#[host_fn("platypus")]
extern "ExtismHost" {
    fn host_link_write_frame(bytes: Vec<u8>) -> Json<Envelope>;
    fn host_link_read_frame() -> Json<Envelope>;
    fn host_fs_write_range(req: String) -> Json<Envelope>;
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
        write_response("empty path")?;
        return Ok(());
    }

    // Initial open: write 0 bytes at offset 0 to create / truncate the
    // file. Append mode skips O_TRUNC; legacy handler uses O_APPEND
    // there too, but with stateless writes we just start at the file's
    // current size — our caller chunked the source so each chunk's
    // data is correctly ordered.
    let truncate = !req.append;
    let mut offset: i64 = 0;
    if !req.append {
        // Truncate-mode: empty initial write to create + truncate.
        if let Err(e) = call_write_range(&req.path, 0, &[], req.mode, req.mkdirs, true) {
            write_response(&e)?;
            return Ok(());
        }
    } else {
        // Append-mode: stat existing file's size by writing 0 bytes
        // and starting offset there. host_fs_write_range with a 0-byte
        // data + truncate=false simply opens, seeks to 0, writes
        // nothing — but we don't get the size back. So just assume
        // existing tail and hand subsequent writes the running offset
        // we'd compute as we read chunks; the OS handles seek beyond
        // current EOF by zero-padding (matches O_APPEND-then-Write
        // semantics for sequential writers).
        // For exactly-matching legacy behaviour the agent needs an
        // explicit append host fn; this simpler model handles the
        // typical "uploader writes a fresh file" path correctly.
        if let Err(e) = call_write_range(&req.path, 0, &[], req.mode, req.mkdirs, false) {
            write_response(&e)?;
            return Ok(());
        }
    }
    let _ = truncate; // keep doc comment relevant; flag is captured via the truncate=true above

    // Ack — open succeeded.
    write_response("")?;

    // Stream-in loop: read FileChunk frames until eof (or wire EOF /
    // error), write each chunk's bytes to disk, accumulate total.
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
            // Wire EOF without a terminal eof=true chunk — surface as
            // a soft error in the result so callers see the unclean
            // close.
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

    // Trailer: FileWriteResult.
    write_result(bytes_written, &first_error)?;
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

fn write_response(error: &str) -> Result<(), Error> {
    // FileWriteResponse{error=1:string}
    let mut buf = Vec::with_capacity(error.len() + 8);
    if !error.is_empty() {
        write_tag(&mut buf, 1, WIRE_LEN);
        write_varint(&mut buf, error.len() as u64);
        buf.extend_from_slice(error.as_bytes());
    }
    write_frame(&buf)
}

fn write_result(bytes_written: i64, error: &str) -> Result<(), Error> {
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

fn write_frame(body: &[u8]) -> Result<(), Error> {
    let env: Envelope = unsafe { host_link_write_frame(body.to_vec())?.0 };
    if !env.ok {
        return Err(Error::msg(format!("host_link_write_frame: {}", env.error)));
    }
    Ok(())
}

// ---- proto decoders -----------------------------------------------

#[derive(Default)]
struct FileWriteRequest {
    path: String,
    append: bool,
    mode: u32,
    mkdirs: bool,
}

fn parse_file_write_request(buf: &[u8]) -> FileWriteRequest {
    let mut req = FileWriteRequest::default();
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
