// sys-file-read is the wasm replacement for the legacy Go
// HandleFileReadStream. It claims STREAM_TYPE_FILE_READ via the
// `wasm:read` host_handler marker; the agent's
// DispatchLegacyWasmStream invokes `read` with the marshalled
// FileReadRequest as input, and we hand-roll FileReadResponse +
// FileChunk frames straight onto the wire via host_link_write_frame.
//
// Wire format (mirrors internal/agent/file_read_stream.go):
//   1. exactly one length-prefixed FileReadResponse — header with
//      size + mode, OR error (then immediate close)
//   2. zero or more FileChunk frames, the last with eof=true
//
// Capabilities required: fs.read with paths=["/"] — same posture as
// the legacy handler had implicitly (it ran in-process with the
// agent's full fs access).

use base64::{engine::general_purpose::STANDARD, Engine as _};
use extism_pdk::*;
use serde::{Deserialize, Serialize};

#[host_fn("platypus")]
extern "ExtismHost" {
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

// FileChunkSize matches the legacy handler's emit chunk size. Stays
// well under internal/link.FrameMaxBytes (1 MiB) so the per-frame
// length prefix + protobuf overhead never overruns the wire bound.
const FILE_CHUNK_SIZE: i64 = 256 * 1024;

#[plugin_fn]
pub fn read(input: Vec<u8>) -> FnResult<()> {
    // Decode FileReadRequest{path=1:string, offset=2:int64, length=3:int64}.
    let req = match parse_file_read_request(&input) {
        Ok(r) => r,
        Err(_) => {
            write_header(0, 0, "parse FileReadRequest")?;
            return Ok(());
        }
    };
    if req.path.is_empty() {
        write_header(0, 0, "empty path")?;
        return Ok(());
    }

    // Probe the file with a zero-length range read so we have size +
    // mode for the header before streaming any bytes. host_fs_read_range
    // reports them on every call; doing one cheap probe avoids a
    // separate stat host fn.
    let probe = call_read_range(&req.path, 0, 0)?;
    if !probe.ok {
        write_header(0, 0, &probe.error)?;
        return Ok(());
    }
    let probe_resp: ReadRangeResp = serde_json::from_value(probe.data).unwrap_or_default();
    let total_size = probe_resp.size;
    let mode = probe_resp.mode;

    // Clamp offset and length the same way the legacy handler did.
    let mut offset = req.offset.max(0);
    if offset > total_size {
        offset = total_size;
    }
    let mut remaining = total_size - offset;
    if req.length > 0 && req.length < remaining {
        remaining = req.length;
    }

    write_header(total_size, mode, "")?;

    while remaining > 0 {
        let want = remaining.min(FILE_CHUNK_SIZE);
        let env = call_read_range(&req.path, offset, want)?;
        if !env.ok {
            // Mid-transfer error — emit a final chunk carrying the
            // error message + eof, matching the legacy contract.
            write_chunk(&[], true, &env.error, offset)?;
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
        write_chunk(&bytes, is_eof, "", offset)?;
        if is_eof {
            return Ok(());
        }
    }
    // Loop exited because remaining hit 0 without an explicit eof
    // chunk above (e.g. requested length matched file size exactly).
    // Emit a trailer.
    write_chunk(&[], true, "", offset)?;
    Ok(())
}

fn call_read_range(path: &str, offset: i64, length: i64) -> Result<Envelope, Error> {
    let args = ReadRangeArgs {
        path: path.to_string(),
        offset,
        length,
    };
    let s = serde_json::to_string(&args).map_err(Error::msg)?;
    let env: Envelope = unsafe { host_fs_read_range(s)?.0 };
    Ok(env)
}

// ---- proto encoding (writer side) ---------------------------------

fn write_header(size: i64, mode: u32, error: &str) -> Result<(), Error> {
    let mut buf = Vec::with_capacity(32);
    if size != 0 {
        write_tag(&mut buf, 1, WIRE_VARINT);
        write_varint(&mut buf, zigzag_unused_int64_to_u64(size));
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

fn write_chunk(data: &[u8], eof: bool, error: &str, source_bytes_so_far: i64) -> Result<(), Error> {
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
        write_varint(&mut buf, zigzag_unused_int64_to_u64(source_bytes_so_far));
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

// ---- proto decoding (reader side, FileReadRequest only) -----------

#[derive(Default)]
struct FileReadRequest {
    path: String,
    offset: i64,
    length: i64,
}

fn parse_file_read_request(buf: &[u8]) -> Result<FileReadRequest, Error> {
    let mut req = FileReadRequest::default();
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

// ---- proto wire helpers -------------------------------------------

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

// proto3 int64 fields use varint (NOT zigzag) for non-`sint64` types,
// which matches FileReadResponse.size + FileChunk.source_bytes_so_far.
// Function name kept descriptive so a future reader doesn't reach
// for the zigzag transform here.
fn zigzag_unused_int64_to_u64(v: i64) -> u64 {
    v as u64
}
