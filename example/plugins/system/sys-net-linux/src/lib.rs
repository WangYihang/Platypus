// sys-net-linux — TCP socket enumeration via /proc/net/tcp{,6}.
//
// /proc/net/tcp lines (after the header row):
//   0: 0100007F:1A0B 00000000:0000 0A 00000000:00000000 ... <inode> ...
//
// Fields used:
//   local_address  (col 1)  — "<8 hex>:<4 hex>" for IPv4
//                            "<32 hex>:<4 hex>" for IPv6 (in tcp6)
//   rem_address    (col 2)  — same encoding
//   st             (col 3)  — TCP state (1=ESTABLISHED, 0A=LISTEN, …)
//
// IPv4 address bytes are little-endian per /proc convention:
//   "0100007F" → bytes [0x01,0x00,0x00,0x7F] read LE → 127.0.0.1
//
// IPv6 is grouped into 4-byte words also stored LE; we reconstruct
// 16 bytes total then format as `aaaa:bbbb:cccc:...:hhhh`.
//
// Ports are big-endian hex (network byte order, no flipping).

use extism_pdk::*;
use serde::{Deserialize, Serialize};

#[cfg(target_arch = "wasm32")]
#[host_fn("platypus")]
extern "ExtismHost" {
    fn host_fs_read(path: String) -> Json<Envelope>;
}

#[derive(Deserialize, Default)]
struct Envelope {
    ok: bool,
    #[serde(default)]
    data: serde_json::Value,
    #[serde(default)]
    error: String,
}

// ---------- request / response shapes ----------

#[derive(Deserialize, Default)]
struct ListListenersRequest {}

#[derive(Deserialize, Default)]
struct ListConnectionsRequest {
    /// Filter by TCP state name (case-insensitive). Empty = all.
    /// Examples: "ESTABLISHED", "TIME_WAIT", "CLOSE_WAIT".
    #[serde(default)]
    state: String,
}

#[derive(Serialize, Default)]
pub struct Listener {
    pub proto: String,
    #[serde(rename = "localAddr")]
    pub local_addr: String,
    #[serde(rename = "localPort")]
    pub local_port: u16,
}

#[derive(Serialize, Default)]
pub struct Connection {
    pub proto: String,
    #[serde(rename = "localAddr")]
    pub local_addr: String,
    #[serde(rename = "localPort")]
    pub local_port: u16,
    #[serde(rename = "peerAddr")]
    pub peer_addr: String,
    #[serde(rename = "peerPort")]
    pub peer_port: u16,
    pub state: String,
}

#[derive(Serialize, Default)]
struct ListListenersResponse {
    listeners: Vec<Listener>,
    #[serde(skip_serializing_if = "String::is_empty")]
    error: String,
}

#[derive(Serialize, Default)]
struct ListConnectionsResponse {
    connections: Vec<Connection>,
    #[serde(skip_serializing_if = "String::is_empty")]
    error: String,
}

// ---------- entry points ----------

#[cfg(target_arch = "wasm32")]
#[plugin_fn]
pub fn list_listeners(_: Json<ListListenersRequest>) -> FnResult<String> {
    let mut listeners = Vec::new();

    if let Ok(text) = read_proc("/proc/net/tcp") {
        for row in parse_proc_net_tcp(&text, /*is_v6*/ false) {
            if row.state == "LISTEN" {
                listeners.push(Listener {
                    proto: "tcp".to_string(),
                    local_addr: row.local_addr,
                    local_port: row.local_port,
                });
            }
        }
    }
    if let Ok(text) = read_proc("/proc/net/tcp6") {
        for row in parse_proc_net_tcp(&text, /*is_v6*/ true) {
            if row.state == "LISTEN" {
                listeners.push(Listener {
                    proto: "tcp6".to_string(),
                    local_addr: row.local_addr,
                    local_port: row.local_port,
                });
            }
        }
    }

    Ok(serde_json::to_string(&ListListenersResponse {
        listeners,
        error: String::new(),
    })?)
}

#[cfg(target_arch = "wasm32")]
#[plugin_fn]
pub fn list_connections(req: Json<ListConnectionsRequest>) -> FnResult<String> {
    let want_state = req.0.state.to_uppercase();
    let mut connections = Vec::new();

    if let Ok(text) = read_proc("/proc/net/tcp") {
        for row in parse_proc_net_tcp(&text, false) {
            if !want_state.is_empty() && row.state != want_state {
                continue;
            }
            connections.push(Connection {
                proto: "tcp".to_string(),
                local_addr: row.local_addr,
                local_port: row.local_port,
                peer_addr: row.peer_addr,
                peer_port: row.peer_port,
                state: row.state,
            });
        }
    }
    if let Ok(text) = read_proc("/proc/net/tcp6") {
        for row in parse_proc_net_tcp(&text, true) {
            if !want_state.is_empty() && row.state != want_state {
                continue;
            }
            connections.push(Connection {
                proto: "tcp6".to_string(),
                local_addr: row.local_addr,
                local_port: row.local_port,
                peer_addr: row.peer_addr,
                peer_port: row.peer_port,
                state: row.state,
            });
        }
    }

    Ok(serde_json::to_string(&ListConnectionsResponse {
        connections,
        error: String::new(),
    })?)
}

// ---------- pure parsers ----------

#[derive(Default, Debug, PartialEq)]
pub struct Row {
    pub local_addr: String,
    pub local_port: u16,
    pub peer_addr: String,
    pub peer_port: u16,
    pub state: String,
}

// parse_proc_net_tcp consumes /proc/net/tcp or tcp6 contents and
// returns one Row per non-header line. Malformed lines are silently
// dropped (defensive — kernels rarely change the format but a
// half-truncated read shouldn't panic).
pub fn parse_proc_net_tcp(text: &str, is_v6: bool) -> Vec<Row> {
    let mut out = Vec::new();
    for (i, line) in text.lines().enumerate() {
        if i == 0 {
            // Header row: "  sl  local_address rem_address ..."
            continue;
        }
        let trimmed = line.trim();
        if trimmed.is_empty() {
            continue;
        }
        let tokens: Vec<&str> = trimmed.split_whitespace().collect();
        if tokens.len() < 4 {
            continue;
        }
        let local = tokens[1];
        let peer = tokens[2];
        let st = tokens[3];

        let (la, lp) = match split_addr_port(local, is_v6) {
            Some(x) => x,
            None => continue,
        };
        let (pa, pp) = match split_addr_port(peer, is_v6) {
            Some(x) => x,
            None => continue,
        };
        let state = decode_state(st).to_string();

        out.push(Row {
            local_addr: la,
            local_port: lp,
            peer_addr: pa,
            peer_port: pp,
            state,
        });
    }
    out
}

// split_addr_port parses "<addr-hex>:<port-hex>" tokens.
// IPv4 (is_v6=false): addr_hex is 8 chars, decoded as 4 LE bytes.
// IPv6 (is_v6=true):  addr_hex is 32 chars, decoded as 4 LE u32 words
//                     each emitting 4 BE bytes.
pub fn split_addr_port(s: &str, is_v6: bool) -> Option<(String, u16)> {
    let (addr_hex, port_hex) = s.rsplit_once(':')?;
    let port = u16::from_str_radix(port_hex, 16).ok()?;
    let addr = if is_v6 {
        decode_ipv6(addr_hex)?
    } else {
        decode_ipv4(addr_hex)?
    };
    Some((addr, port))
}

fn decode_ipv4(hex: &str) -> Option<String> {
    if hex.len() != 8 {
        return None;
    }
    let bytes = hex_to_bytes(hex)?;
    // Bytes in /proc are stored little-endian; flip.
    Some(format!(
        "{}.{}.{}.{}",
        bytes[3], bytes[2], bytes[1], bytes[0]
    ))
}

fn decode_ipv6(hex: &str) -> Option<String> {
    if hex.len() != 32 {
        return None;
    }
    // /proc/net/tcp6 stores the 16-byte address as four 4-byte
    // little-endian u32 words concatenated. We recover the network-
    // order byte sequence by flipping each 4-byte chunk.
    let mut bytes = [0u8; 16];
    for i in 0..4 {
        let chunk = &hex[i * 8..(i + 1) * 8];
        let raw = hex_to_bytes(chunk)?;
        bytes[i * 4 + 0] = raw[3];
        bytes[i * 4 + 1] = raw[2];
        bytes[i * 4 + 2] = raw[1];
        bytes[i * 4 + 3] = raw[0];
    }
    // Format as 8 colon-separated u16 groups.
    let mut groups = [0u16; 8];
    for i in 0..8 {
        groups[i] = ((bytes[i * 2] as u16) << 8) | (bytes[i * 2 + 1] as u16);
    }
    Some(
        groups
            .iter()
            .map(|g| format!("{:x}", g))
            .collect::<Vec<_>>()
            .join(":"),
    )
}

fn hex_to_bytes(hex: &str) -> Option<Vec<u8>> {
    if hex.len() % 2 != 0 {
        return None;
    }
    let mut out = Vec::with_capacity(hex.len() / 2);
    for chunk in hex.as_bytes().chunks(2) {
        let s = std::str::from_utf8(chunk).ok()?;
        out.push(u8::from_str_radix(s, 16).ok()?);
    }
    Some(out)
}

// decode_state maps the kernel's TCP state code (2 hex chars) to
// the human-readable name.  Source: include/net/tcp_states.h.
pub fn decode_state(hex: &str) -> &'static str {
    match hex {
        "01" => "ESTABLISHED",
        "02" => "SYN_SENT",
        "03" => "SYN_RECV",
        "04" => "FIN_WAIT1",
        "05" => "FIN_WAIT2",
        "06" => "TIME_WAIT",
        "07" => "CLOSE",
        "08" => "CLOSE_WAIT",
        "09" => "LAST_ACK",
        "0A" => "LISTEN",
        "0B" => "CLOSING",
        "0C" => "NEW_SYN_RECV",
        _ => "UNKNOWN",
    }
}

// ---------- host fn helper ----------

#[cfg(target_arch = "wasm32")]
fn read_proc(path: &str) -> Result<String, String> {
    let env: Envelope = unsafe {
        host_fs_read(path.to_string())
            .map_err(|e| format!("host_fs_read: {}", e))?
            .0
    };
    if !env.ok {
        return Err(env.error);
    }
    // host_fs_read returns the file contents as a JSON string.
    env.data
        .as_str()
        .map(|s| s.to_string())
        .ok_or_else(|| "host_fs_read: data not a string".to_string())
}

#[cfg(not(target_arch = "wasm32"))]
fn read_proc(_path: &str) -> Result<String, String> {
    Err("not available on host build".to_string())
}

// ============================================================
// tests (host-build only — pure parsers)
// ============================================================

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn decode_ipv4_loopback() {
        assert_eq!(decode_ipv4("0100007F").as_deref(), Some("127.0.0.1"));
    }

    #[test]
    fn decode_ipv4_zero() {
        assert_eq!(decode_ipv4("00000000").as_deref(), Some("0.0.0.0"));
    }

    #[test]
    fn decode_ipv4_rejects_short_hex() {
        assert_eq!(decode_ipv4("0100").as_deref(), None);
    }

    #[test]
    fn split_addr_port_ipv4() {
        let (a, p) = split_addr_port("0100007F:1A0B", false).unwrap();
        assert_eq!(a, "127.0.0.1");
        assert_eq!(p, 0x1A0B);
    }

    #[test]
    fn split_addr_port_rejects_garbage() {
        assert!(split_addr_port("nothex:zzzz", false).is_none());
    }

    #[test]
    fn decode_ipv6_loopback() {
        // ::1 is stored as 16 bytes [0,0,0,0, 0,0,0,0, 0,0,0,0, 0,0,0,1]
        // grouped LE-per-4: 00000000 00000000 00000000 01000000
        let got = decode_ipv6("00000000000000000000000001000000").unwrap();
        assert_eq!(got, "0:0:0:0:0:0:0:1");
    }

    #[test]
    fn decode_state_known_codes() {
        assert_eq!(decode_state("01"), "ESTABLISHED");
        assert_eq!(decode_state("0A"), "LISTEN");
        assert_eq!(decode_state("06"), "TIME_WAIT");
        assert_eq!(decode_state("ZZ"), "UNKNOWN");
    }

    #[test]
    fn parse_proc_net_tcp_basic() {
        let text = "\
  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 0100007F:1A0B 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 38 1
   1: 0100007F:0050 0200A8C0:E89E 01 00000000:00000000 00:00000000 00000000     0        0 99 1
";
        let got = parse_proc_net_tcp(text, false);
        assert_eq!(got.len(), 2);
        assert_eq!(got[0].local_addr, "127.0.0.1");
        assert_eq!(got[0].local_port, 0x1A0B);
        assert_eq!(got[0].state, "LISTEN");
        assert_eq!(got[1].state, "ESTABLISHED");
        assert_eq!(got[1].peer_addr, "192.168.0.2");
        assert_eq!(got[1].peer_port, 0xE89E);
    }

    #[test]
    fn parse_proc_net_tcp_skips_header_only() {
        let text =
            "  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode\n";
        assert!(parse_proc_net_tcp(text, false).is_empty());
    }

    #[test]
    fn parse_proc_net_tcp_drops_short_rows() {
        let text = "  sl local rem st\n   0: malformed\n";
        assert!(parse_proc_net_tcp(text, false).is_empty());
    }

    #[test]
    fn parse_proc_net_tcp6_basic() {
        // ::1 in /proc/net/tcp6 stored as 32-char LE hex. Port 8080 = 1F90.
        let text = "\
  sl  local_address rem_address st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 00000000000000000000000001000000:1F90 00000000000000000000000000000000:0000 0A ...
";
        let got = parse_proc_net_tcp(text, true);
        assert_eq!(got.len(), 1);
        assert_eq!(got[0].local_addr, "0:0:0:0:0:0:0:1");
        assert_eq!(got[0].local_port, 0x1F90);
        assert_eq!(got[0].state, "LISTEN");
    }
}
