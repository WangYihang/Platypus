// sys-tunnel-pull replaces HandleTunnelPullStream. The wasm side is
// thin: parse TunnelPullRequest, apply dial policy via the
// capability allowlist, hand off to the host's host_net_dial +
// host_net_relay. The host owns the bidirectional byte splice
// between the operator's wire and the dialed TCP conn.
//
// Flow:
//   1. parse TunnelPullRequest from input bytes (target, timeout)
//   2. host_net_dial(spec_json) → {handle, resolved_addr} or error
//   3. write TunnelPullResponse via host_stream_write
//   4. host_net_relay(handle) → blocks until either side closes;
//      during the call the host pumps wire ↔ TCP raw bytes
//   5. return — the host already closed the dialed conn

use extism_pdk::*;
use serde::{Deserialize, Serialize};

#[cfg(target_arch = "wasm32")]
#[host_fn("platypus")]
extern "ExtismHost" {
    fn host_stream_write(bytes: Vec<u8>) -> Json<Envelope>;
    fn host_net_dial(spec: String) -> Json<Envelope>;
    fn host_net_relay(handle: String) -> Json<Envelope>;
    fn host_net_close(handle: String) -> Json<Envelope>;
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
struct DialSpec {
    target: String,
    dial_timeout_ms: u32,
}

#[derive(Deserialize, Default)]
struct DialResponse {
    handle: u32,
    resolved_addr: String,
}

#[cfg(target_arch = "wasm32")]
#[plugin_fn]
pub fn pull(input: Vec<u8>) -> FnResult<()> {
    let req = parse_tunnel_pull_request(&input);
    if req.target.is_empty() {
        write_pull_response("", "empty target")?;
        return Ok(());
    }

    let spec = DialSpec {
        target: req.target.clone(),
        dial_timeout_ms: req.dial_timeout_ms,
    };
    let spec_json = serde_json::to_string(&spec)
        .map_err(|e| WithReturnCode::new(Error::msg(format!("encode spec: {e}")), 1))?;
    let env: Envelope = unsafe { host_net_dial(spec_json)?.0 };
    if !env.ok {
        // Dial failure (allowlist denial, refused, timeout). Propagate
        // to the operator via the response header. No bytes flow.
        write_pull_response("", &env.error)?;
        return Ok(());
    }
    let dial: DialResponse = serde_json::from_value(env.data).unwrap_or_default();

    write_pull_response(&dial.resolved_addr, "")?;

    // Hand off to the host's bidirectional pump. Blocks until either
    // side closes; the host closes the dialed conn itself, then the
    // dispatcher's deferred stream.Close handles the wire.
    let handle_arg = serde_json::to_string(&dial.handle)
        .map_err(|e| WithReturnCode::new(Error::msg(format!("encode handle: {e}")), 1))?;
    let relay: Envelope = unsafe { host_net_relay(handle_arg.clone())?.0 };
    if !relay.ok {
        let _ = unsafe { host_net_close(handle_arg)?.0 };
        return Err(WithReturnCode::new(
            Error::msg(format!("host_net_relay: {}", relay.error)),
            1,
        )
        .into());
    }
    Ok(())
}

// ---- TunnelPullResponse encoder ---------------------------------

#[cfg(target_arch = "wasm32")]
fn write_pull_response(resolved: &str, error: &str) -> Result<(), Error> {
    // TunnelPullResponse{resolved_addr=1:string, error=2:string}
    let mut buf = Vec::with_capacity(resolved.len() + error.len() + 8);
    if !resolved.is_empty() {
        write_tag(&mut buf, 1, WIRE_LEN);
        write_varint(&mut buf, resolved.len() as u64);
        buf.extend_from_slice(resolved.as_bytes());
    }
    if !error.is_empty() {
        write_tag(&mut buf, 2, WIRE_LEN);
        write_varint(&mut buf, error.len() as u64);
        buf.extend_from_slice(error.as_bytes());
    }
    let env: Envelope = unsafe { host_stream_write(buf)?.0 };
    if !env.ok {
        return Err(Error::msg(format!("host_stream_write: {}", env.error)));
    }
    Ok(())
}

// ---- TunnelPullRequest decoder ----------------------------------

#[derive(Default)]
struct TunnelPullRequest {
    target: String,
    dial_timeout_ms: u32,
}

fn parse_tunnel_pull_request(buf: &[u8]) -> TunnelPullRequest {
    let mut req = TunnelPullRequest::default();
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
                req.target = String::from_utf8_lossy(&buf[i..end]).to_string();
                i = end;
            }
            (2, WIRE_VARINT) => {
                let (v, m) = match read_varint(&buf[i..]) {
                    Ok(v) => v,
                    Err(_) => break,
                };
                i += m;
                req.dial_timeout_ms = v as u32;
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
// Pure-function unit tests (host build, not wasm)
// ============================================================
//
// `cargo test --lib` runs these on the host triple. The wasm-only
// glue (host_fn declarations, plugin_fn entries, write_pull_response
// frame writer) is excluded by cfg(target_arch="wasm32") above; full
// end-to-end coverage of the wasm side lives in
// internal/agent/plugin/tunnel_pull_integration_test.go.
#[cfg(test)]
mod tests {
    use super::*;

    // ---- varint round-trip ----------------------------------

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
    fn varint_truncated_errors() {
        assert!(read_varint(&[0xFF]).is_err());
    }

    // ---- write_tag ------------------------------------------

    #[test]
    fn write_tag_packs_field_and_wire_type() {
        let mut buf = Vec::new();
        write_tag(&mut buf, 1, WIRE_LEN); // (1<<3)|2 = 10
        assert_eq!(buf, vec![0x0a]);
    }

    // ---- parse_tunnel_pull_request --------------------------

    #[test]
    fn parse_tunnel_pull_request_target_only() {
        // {target: "10.0.0.1:443"}
        let mut buf = Vec::new();
        write_tag(&mut buf, 1, WIRE_LEN);
        let target = b"10.0.0.1:443";
        write_varint(&mut buf, target.len() as u64);
        buf.extend_from_slice(target);
        let req = parse_tunnel_pull_request(&buf);
        assert_eq!(req.target, "10.0.0.1:443");
        assert_eq!(req.dial_timeout_ms, 0);
    }

    #[test]
    fn parse_tunnel_pull_request_with_timeout() {
        // {target: "host:80", dial_timeout_ms: 5000}
        let mut buf = Vec::new();
        write_tag(&mut buf, 1, WIRE_LEN);
        write_varint(&mut buf, 7);
        buf.extend_from_slice(b"host:80");
        write_tag(&mut buf, 2, WIRE_VARINT);
        write_varint(&mut buf, 5000);
        let req = parse_tunnel_pull_request(&buf);
        assert_eq!(req.target, "host:80");
        assert_eq!(req.dial_timeout_ms, 5000);
    }

    #[test]
    fn parse_tunnel_pull_request_truncated_string_does_not_panic() {
        // Promises 50-byte target but provides 5.
        let mut buf = Vec::new();
        write_tag(&mut buf, 1, WIRE_LEN);
        write_varint(&mut buf, 50);
        buf.extend_from_slice(b"abcde");
        let req = parse_tunnel_pull_request(&buf);
        // Aborts cleanly without writing target.
        assert_eq!(req.target, "");
    }

    #[test]
    fn parse_tunnel_pull_request_skips_unknown_fields() {
        let mut buf = Vec::new();
        write_tag(&mut buf, 99, WIRE_VARINT);
        write_varint(&mut buf, 42);
        write_tag(&mut buf, 1, WIRE_LEN);
        write_varint(&mut buf, 5);
        buf.extend_from_slice(b"a:123");
        let req = parse_tunnel_pull_request(&buf);
        assert_eq!(req.target, "a:123");
    }

    #[test]
    fn parse_tunnel_pull_request_empty_buffer() {
        let req = parse_tunnel_pull_request(&[]);
        assert_eq!(req.target, "");
        assert_eq!(req.dial_timeout_ms, 0);
    }
}
