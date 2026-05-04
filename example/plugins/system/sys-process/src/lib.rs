// sys-process consolidates two older plugins into a single wasm:
//
//   exec (RPC)              ← was sys-exec        (capability `exec`)
//   open (stream pump)      ← was sys-process-open (capability `process`)
//
// Distinct capabilities are PRESERVED by the merge — the manifest
// declares both `exec` and `process`, and the host's runtime check
// gates each one independently. The merge only collapses two
// plugin-ids into one for the operator's enroll list.
//
// `exec` is the synchronous one-shot path: ExecRequest in, ExecResponse
// out, no stream. `open` is the long-lived interactive PTY: parses
// ProcessOpenRequest, calls host_process_spawn + host_process_relay,
// host owns the bidirectional pump until the child exits.

use extism_pdk::*;
use serde::{Deserialize, Serialize};
use std::collections::{BTreeMap, HashMap};

// ============================================================
// Shared host-fn declarations
// ============================================================

#[cfg(target_arch = "wasm32")]
#[host_fn("platypus")]
extern "ExtismHost" {
    fn host_exec(req: String) -> Json<Envelope>;
    fn host_link_write_frame(bytes: Vec<u8>) -> Json<Envelope>;
    fn host_process_spawn(spec: String) -> Json<Envelope>;
    fn host_process_relay(handle: String) -> Json<Envelope>;
    fn host_process_kill(handle: String) -> Json<Envelope>;
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

// ============================================================
// exec (RPC)
// ============================================================

#[derive(Deserialize, Serialize, Default)]
pub struct ExecRequest {
    pub command: String,
    #[serde(default)]
    pub args: Vec<String>,
    #[serde(default)]
    pub env: HashMap<String, String>,
    #[serde(default)]
    pub cwd: String,
    #[serde(default)]
    pub timeout_ms: u32,
}

#[derive(Serialize, Deserialize, Default)]
pub struct ExecResponse {
    #[serde(default)]
    pub exit_code: i32,
    #[serde(default)]
    pub stdout: String,
    #[serde(default)]
    pub stderr: String,
    #[serde(default)]
    pub error: String,
}

#[cfg(target_arch = "wasm32")]
#[plugin_fn]
pub fn exec(req: Json<ExecRequest>) -> FnResult<Json<ExecResponse>> {
    let body = serde_json::to_string(&req.0)?;
    let env: Envelope = unsafe { host_exec(body)?.0 };
    if !env.ok {
        return Ok(Json(ExecResponse {
            error: env.error,
            ..Default::default()
        }));
    }
    let resp: ExecResponse = serde_json::from_value(env.data)?;
    Ok(Json(resp))
}

// ============================================================
// open (stream)
// ============================================================
//
// Wasm stays thin by design: policy lives here, the host owns the
// long-lived bidirectional pump.
//
// Flow:
//   1. parse ProcessOpenRequest from input bytes
//   2. host_process_spawn(spec_json) → {handle, pid}
//   3. write ProcessOpenResponse via host_link_write_frame (or an
//      error response if the spawn fails)
//   4. host_process_relay(handle) → blocks until the child exits;
//      during the call the host pumps wire ↔ child PTY/pipes
//   5. host writes the terminal ProcessFrame.exit itself before
//      returning; nothing left for wasm to do

#[derive(Serialize)]
struct SpawnSpec {
    command: String,
    args: Vec<String>,
    cwd: String,
    env: BTreeMap<String, String>,
    pty: bool,
    cols: u32,
    rows: u32,
}

#[derive(Deserialize, Default)]
struct SpawnResponse {
    handle: u32,
    pid: i32,
}

#[cfg(target_arch = "wasm32")]
#[plugin_fn]
pub fn open(input: Vec<u8>) -> FnResult<()> {
    let req = parse_process_open_request(&input);
    if req.command.is_empty() {
        write_open_response(0, "empty_command")?;
        return Ok(());
    }

    let spec = SpawnSpec {
        command: req.command.clone(),
        args: req.args.clone(),
        cwd: req.cwd.clone(),
        env: req.env.clone(),
        pty: req.pty,
        cols: req.cols,
        rows: req.rows,
    };
    let spec_json = serde_json::to_string(&spec)
        .map_err(|e| WithReturnCode::new(Error::msg(format!("encode spec: {e}")), 1))?;
    let env: Envelope = unsafe { host_process_spawn(spec_json)?.0 };
    if !env.ok {
        write_open_response(0, &env.error)?;
        return Ok(());
    }
    let spawn: SpawnResponse = serde_json::from_value(env.data).unwrap_or_default();

    write_open_response(spawn.pid as i64, "")?;

    let handle_arg = serde_json::to_string(&spawn.handle)
        .map_err(|e| WithReturnCode::new(Error::msg(format!("encode handle: {e}")), 1))?;
    let relay: Envelope = unsafe { host_process_relay(handle_arg.clone())?.0 };
    if !relay.ok {
        let _ = unsafe { host_process_kill(handle_arg)?.0 };
        return Err(WithReturnCode::new(
            Error::msg(format!("host_process_relay: {}", relay.error)),
            1,
        )
        .into());
    }
    Ok(())
}

#[cfg(target_arch = "wasm32")]
fn write_open_response(pid: i64, error: &str) -> Result<(), Error> {
    // ProcessOpenResponse{pid=1:int64, error=2:string}
    let mut buf = Vec::with_capacity(32);
    if pid != 0 {
        write_tag(&mut buf, 1, WIRE_VARINT);
        write_varint(&mut buf, pid as u64);
    }
    if !error.is_empty() {
        write_tag(&mut buf, 2, WIRE_LEN);
        write_varint(&mut buf, error.len() as u64);
        buf.extend_from_slice(error.as_bytes());
    }
    let env: Envelope = unsafe { host_link_write_frame(buf)?.0 };
    if !env.ok {
        return Err(Error::msg(format!("host_link_write_frame: {}", env.error)));
    }
    Ok(())
}

#[derive(Default)]
struct ProcessOpenRequest {
    command: String,
    args: Vec<String>,
    cwd: String,
    env: BTreeMap<String, String>,
    cols: u32,
    rows: u32,
    pty: bool,
}

fn parse_process_open_request(buf: &[u8]) -> ProcessOpenRequest {
    let mut req = ProcessOpenRequest::default();
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
                req.command = String::from_utf8_lossy(&buf[i..end]).to_string();
                i = end;
            }
            (2, WIRE_LEN) => {
                let (len, m) = match read_varint(&buf[i..]) {
                    Ok(v) => v,
                    Err(_) => break,
                };
                i += m;
                let end = i + len as usize;
                if end > buf.len() {
                    break;
                }
                req.args.push(String::from_utf8_lossy(&buf[i..end]).to_string());
                i = end;
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
                req.cwd = String::from_utf8_lossy(&buf[i..end]).to_string();
                i = end;
            }
            (4, WIRE_LEN) => {
                let (len, m) = match read_varint(&buf[i..]) {
                    Ok(v) => v,
                    Err(_) => break,
                };
                i += m;
                let end = i + len as usize;
                if end > buf.len() {
                    break;
                }
                let (k, v) = parse_map_entry(&buf[i..end]);
                if !k.is_empty() {
                    req.env.insert(k, v);
                }
                i = end;
            }
            (5, WIRE_VARINT) => {
                let (v, m) = match read_varint(&buf[i..]) {
                    Ok(v) => v,
                    Err(_) => break,
                };
                i += m;
                req.cols = v as u32;
            }
            (6, WIRE_VARINT) => {
                let (v, m) = match read_varint(&buf[i..]) {
                    Ok(v) => v,
                    Err(_) => break,
                };
                i += m;
                req.rows = v as u32;
            }
            (7, WIRE_VARINT) => {
                let (v, m) = match read_varint(&buf[i..]) {
                    Ok(v) => v,
                    Err(_) => break,
                };
                i += m;
                req.pty = v != 0;
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

fn parse_map_entry(buf: &[u8]) -> (String, String) {
    let mut k = String::new();
    let mut v = String::new();
    let mut i = 0;
    while i < buf.len() {
        let (tag, n) = match read_varint(&buf[i..]) {
            Ok(t) => t,
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
                k = String::from_utf8_lossy(&buf[i..end]).to_string();
                i = end;
            }
            (2, WIRE_LEN) => {
                let (len, m) = match read_varint(&buf[i..]) {
                    Ok(v) => v,
                    Err(_) => break,
                };
                i += m;
                let end = i + len as usize;
                if end > buf.len() {
                    break;
                }
                v = String::from_utf8_lossy(&buf[i..end]).to_string();
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
    (k, v)
}

// ============================================================
// Pure-function unit tests (host build, not wasm)
// ============================================================
//
// Covers the proto wire helpers + the two request-decoders that
// take raw bytes off the stream. The wasm-only host_fn bridge layer
// is exercised end-to-end by Go's
// internal/agent/plugin/process_open_integration_test.go and
// process_exec_integration_test.go.
#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn varint_round_trip_boundaries() {
        for n in [0u64, 1, 127, 128, 16_383, 16_384, u64::MAX] {
            let mut buf = Vec::new();
            write_varint(&mut buf, n);
            let (got, _) = read_varint(&buf).unwrap();
            assert_eq!(got, n, "round-trip failed for {n}");
        }
    }

    #[test]
    fn write_tag_layout() {
        let mut buf = Vec::new();
        write_tag(&mut buf, 7, WIRE_VARINT); // (7<<3)|0 = 56 = 0x38
        assert_eq!(buf, vec![0x38]);
    }

    // ---- parse_process_open_request --------------------------

    #[test]
    fn parse_process_open_request_command_only() {
        // {command: "/bin/sh"}
        let mut buf = Vec::new();
        write_tag(&mut buf, 1, WIRE_LEN);
        write_varint(&mut buf, 7);
        buf.extend_from_slice(b"/bin/sh");
        let req = parse_process_open_request(&buf);
        assert_eq!(req.command, "/bin/sh");
        assert!(req.args.is_empty());
        assert_eq!(req.cwd, "");
        assert!(!req.pty);
    }

    #[test]
    fn parse_process_open_request_with_args_pty_dimensions() {
        // {command: "ls", args: ["-l", "/tmp"], pty: true, cols: 80, rows: 24}
        let mut buf = Vec::new();
        // command
        write_tag(&mut buf, 1, WIRE_LEN);
        write_varint(&mut buf, 2);
        buf.extend_from_slice(b"ls");
        // args (repeated, field 2)
        for arg in ["-l", "/tmp"] {
            write_tag(&mut buf, 2, WIRE_LEN);
            write_varint(&mut buf, arg.len() as u64);
            buf.extend_from_slice(arg.as_bytes());
        }
        // cols (field 5)
        write_tag(&mut buf, 5, WIRE_VARINT);
        write_varint(&mut buf, 80);
        // rows (field 6)
        write_tag(&mut buf, 6, WIRE_VARINT);
        write_varint(&mut buf, 24);
        // pty (field 7)
        write_tag(&mut buf, 7, WIRE_VARINT);
        write_varint(&mut buf, 1);

        let req = parse_process_open_request(&buf);
        assert_eq!(req.command, "ls");
        assert_eq!(req.args, vec!["-l".to_string(), "/tmp".to_string()]);
        assert_eq!(req.cols, 80);
        assert_eq!(req.rows, 24);
        assert!(req.pty);
    }

    #[test]
    fn parse_process_open_request_truncated_string_does_not_panic() {
        // command tag promises 50 bytes but provides 3.
        let mut buf = Vec::new();
        write_tag(&mut buf, 1, WIRE_LEN);
        write_varint(&mut buf, 50);
        buf.extend_from_slice(b"abc");
        let req = parse_process_open_request(&buf);
        // Aborts before the command field is filled.
        assert_eq!(req.command, "");
    }

    #[test]
    fn parse_process_open_request_skips_unknown_fields() {
        // unknown field 99 (varint) before command
        let mut buf = Vec::new();
        write_tag(&mut buf, 99, WIRE_VARINT);
        write_varint(&mut buf, 7);
        write_tag(&mut buf, 1, WIRE_LEN);
        write_varint(&mut buf, 1);
        buf.push(b'q');
        let req = parse_process_open_request(&buf);
        assert_eq!(req.command, "q");
    }

    // ---- parse_map_entry -------------------------------------
    //
    // Proto map<string,string> wire: each entry is a length-delimited
    // submessage with field 1 = key, field 2 = value (both LEN-prefixed
    // strings).

    #[test]
    fn parse_map_entry_full_kv() {
        let mut sub = Vec::new();
        write_tag(&mut sub, 1, WIRE_LEN);
        write_varint(&mut sub, 4);
        sub.extend_from_slice(b"PATH");
        write_tag(&mut sub, 2, WIRE_LEN);
        write_varint(&mut sub, 8);
        sub.extend_from_slice(b"/usr/bin");
        let (k, v) = parse_map_entry(&sub);
        assert_eq!(k, "PATH");
        assert_eq!(v, "/usr/bin");
    }

    #[test]
    fn parse_map_entry_value_only_keeps_key_empty() {
        let mut sub = Vec::new();
        write_tag(&mut sub, 2, WIRE_LEN);
        write_varint(&mut sub, 1);
        sub.push(b'x');
        let (k, v) = parse_map_entry(&sub);
        assert_eq!(k, "");
        assert_eq!(v, "x");
    }

    // env field is field 4 in ProcessOpenRequest, where each entry
    // is a LEN-prefixed (key,value) submessage. Verify the full
    // round-trip from request bytes through to populated env map.
    #[test]
    fn parse_process_open_request_env_map() {
        let mut buf = Vec::new();
        write_tag(&mut buf, 1, WIRE_LEN);
        write_varint(&mut buf, 1);
        buf.push(b'c');
        // Build the env entry submessage (key=K, value=V).
        let mut entry = Vec::new();
        write_tag(&mut entry, 1, WIRE_LEN);
        write_varint(&mut entry, 1);
        entry.push(b'K');
        write_tag(&mut entry, 2, WIRE_LEN);
        write_varint(&mut entry, 1);
        entry.push(b'V');
        // Embed the submessage as field 4.
        write_tag(&mut buf, 4, WIRE_LEN);
        write_varint(&mut buf, entry.len() as u64);
        buf.extend_from_slice(&entry);

        let req = parse_process_open_request(&buf);
        assert_eq!(req.env.get("K"), Some(&"V".to_string()));
    }
}
