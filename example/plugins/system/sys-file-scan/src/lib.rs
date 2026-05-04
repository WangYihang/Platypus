// sys-file-scan replaces the Go HandleFileScanStream. Walks the
// requested paths using host_fs_listdir + host_fs_stat (recursing
// into subdirectories on the wasm side), then emits a single
// FileScanResponse via host_link_write_frame matching the legacy
// handler's wire format.
//
// Mid-walk errors (permission denied on a deep subdir) are silently
// skipped — same posture as the legacy handler. A missing root is
// reported as the response's `error` field so the operator sees a
// useful failure.

use extism_pdk::*;
use serde::{Deserialize, Serialize};

#[host_fn("platypus")]
extern "ExtismHost" {
    fn host_fs_listdir(path: String) -> Json<Envelope>;
    fn host_fs_stat(path: String) -> Json<Envelope>;
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
struct StatEntry {
    #[serde(default)]
    is_dir: bool,
    #[serde(default)]
    size: i64,
    #[serde(default)]
    mode: u32,
    #[serde(default, rename = "name")]
    _name: String,
}

#[derive(Deserialize, Default, Debug, Clone)]
struct ListEntry {
    name: String,
    #[serde(default)]
    is_dir: bool,
    #[serde(default)]
    size: i64,
}

#[derive(Default)]
struct Counts {
    files: i64,
    dirs: i64,
    bytes: i64,
}

#[plugin_fn]
pub fn scan(input: Vec<u8>) -> FnResult<()> {
    let req = parse_file_scan_request(&input);
    if req.paths.is_empty() {
        write_response(0, 0, 0, "no paths to scan")?;
        return Ok(());
    }

    let mut counts = Counts::default();
    for root in &req.paths {
        // Stat the root first — a missing root is a fatal error
        // (matches the legacy handler).
        let root_stat = match call_stat(root) {
            Some(s) => s,
            None => {
                write_response(0, 0, 0, &format!("stat {}: not found", root))?;
                return Ok(());
            }
        };
        if !root_stat.is_dir {
            // Single-file root: count it directly, no walk.
            counts.files += 1;
            counts.bytes += root_stat.size;
            continue;
        }
        counts.dirs += 1;
        walk(root, &mut counts);
    }
    write_response(counts.files, counts.dirs, counts.bytes, "")?;
    Ok(())
}

// walk is a non-recursive (stack-based) directory walker. wasm has no
// stack-overflow protection beyond the linear memory cap, so an
// adversarially-deep tree (a million-deep nested fs) on a recursive
// implementation could OOM. Iterative + a Vec<String> queue stays
// safe.
fn walk(root: &str, counts: &mut Counts) {
    let mut stack: Vec<String> = vec![root.to_string()];
    while let Some(dir) = stack.pop() {
        let entries = match call_listdir(&dir) {
            Some(e) => e,
            None => continue, // permission denied / disappeared — skip
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

fn call_stat(path: &str) -> Option<StatEntry> {
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

// ---- proto helpers ------------------------------------------------

#[derive(Default, Serialize)]
struct FileScanRequest {
    paths: Vec<String>,
    follow_symlinks: bool,
}

fn parse_file_scan_request(buf: &[u8]) -> FileScanRequest {
    let mut req = FileScanRequest::default();
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

fn write_response(file_count: i64, dir_count: i64, total_bytes: i64, error: &str) -> Result<(), Error> {
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
    let env: Envelope = unsafe { host_link_write_frame(buf)?.0 };
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
