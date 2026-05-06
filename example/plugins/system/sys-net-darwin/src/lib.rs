// sys-net-darwin — TCP socket enumeration via /usr/sbin/netstat -anv -p tcp.
//
// BSD netstat output (truncated; columns can be wider):
//
//   Active Internet connections (including servers)
//   Proto Recv-Q Send-Q  Local Address          Foreign Address        (state)
//   tcp4       0      0  127.0.0.1.51234        127.0.0.1.443          ESTABLISHED
//   tcp4       0      0  *.22                   *.*                    LISTEN
//   tcp6       0      0  ::1.5000               *.*                    LISTEN
//
// Apple's netstat uses '.' as the addr/port separator (POSIX form
// used "<addr>.<port>" before colon-port became universal). The
// final column is always state. Wildcards "*" mean any address /
// any port.
//
// Wire shape: identical to sys-net-linux. The proto column is
// "tcp" or "tcp6" (we drop the trailing "4" suffix BSD adds).

use extism_pdk::*;
use serde::{Deserialize, Serialize};

#[cfg(target_arch = "wasm32")]
#[host_fn("platypus")]
extern "ExtismHost" {
    fn host_exec(req: String) -> Json<Envelope>;
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
struct ExecRequest {
    command: String,
    args: Vec<String>,
    #[serde(rename = "timeout_ms")]
    timeout_ms: u32,
}

#[derive(Deserialize, Default)]
struct ExecResponse {
    #[serde(default, rename = "exit_code")]
    exit_code: i32,
    #[serde(default)]
    stdout: String,
    #[serde(default)]
    stderr: String,
}

#[derive(Deserialize, Default)]
struct ListListenersRequest {}

#[derive(Deserialize, Default)]
struct ListConnectionsRequest {
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

#[cfg(target_arch = "wasm32")]
#[plugin_fn]
pub fn list_listeners(_: Json<ListListenersRequest>) -> FnResult<String> {
    let exec_resp = match run_netstat(7_000) {
        Ok(v) => v,
        Err(e) => {
            return Ok(serde_json::to_string(&ListListenersResponse {
                listeners: Vec::new(),
                error: e,
            })?)
        }
    };
    if exec_resp.exit_code != 0 {
        return Ok(serde_json::to_string(&ListListenersResponse {
            listeners: Vec::new(),
            error: format!(
                "netstat exit {}: {}",
                exec_resp.exit_code,
                exec_resp.stderr.trim()
            ),
        })?);
    }
    let rows = parse_netstat_output(&exec_resp.stdout);
    let listeners: Vec<Listener> = rows
        .into_iter()
        .filter(|r| r.state == "LISTEN")
        .map(|r| Listener {
            proto: r.proto,
            local_addr: r.local_addr,
            local_port: r.local_port,
        })
        .collect();
    Ok(serde_json::to_string(&ListListenersResponse {
        listeners,
        error: String::new(),
    })?)
}

#[cfg(target_arch = "wasm32")]
#[plugin_fn]
pub fn list_connections(req: Json<ListConnectionsRequest>) -> FnResult<String> {
    let want_state = req.0.state.to_uppercase();
    let exec_resp = match run_netstat(7_000) {
        Ok(v) => v,
        Err(e) => {
            return Ok(serde_json::to_string(&ListConnectionsResponse {
                connections: Vec::new(),
                error: e,
            })?)
        }
    };
    if exec_resp.exit_code != 0 {
        return Ok(serde_json::to_string(&ListConnectionsResponse {
            connections: Vec::new(),
            error: format!(
                "netstat exit {}: {}",
                exec_resp.exit_code,
                exec_resp.stderr.trim()
            ),
        })?);
    }
    let rows = parse_netstat_output(&exec_resp.stdout);
    let conns: Vec<Connection> = rows
        .into_iter()
        .filter(|r| want_state.is_empty() || r.state == want_state)
        .map(|r| Connection {
            proto: r.proto,
            local_addr: r.local_addr,
            local_port: r.local_port,
            peer_addr: r.peer_addr,
            peer_port: r.peer_port,
            state: r.state,
        })
        .collect();
    Ok(serde_json::to_string(&ListConnectionsResponse {
        connections: conns,
        error: String::new(),
    })?)
}

// ---------- pure parsers ----------

#[derive(Default, Debug, PartialEq)]
pub struct Row {
    pub proto: String,
    pub local_addr: String,
    pub local_port: u16,
    pub peer_addr: String,
    pub peer_port: u16,
    pub state: String,
}

// parse_netstat_output reads BSD netstat -anv -p tcp output.
// Layout: heading line + column-header line + N data rows. Column
// order: Proto Recv-Q Send-Q Local-Address Foreign-Address (state)
// + extras netstat -v adds (rhiwat, shiwat, pid, epid, state...).
//
// Strategy: skip until we see a line whose first whitespace token
// starts with "tcp"; from there each row is parsed. Last column is
// always state.
pub fn parse_netstat_output(stdout: &str) -> Vec<Row> {
    let mut out = Vec::new();
    for line in stdout.lines() {
        let trimmed = line.trim();
        if trimmed.is_empty() {
            continue;
        }
        let tokens: Vec<&str> = trimmed.split_whitespace().collect();
        if tokens.len() < 6 {
            continue;
        }
        let proto_raw = tokens[0];
        // Only parse rows whose proto starts with "tcp". Skip "Proto"
        // header / "Active Internet ..." preamble / udp4 etc.
        if !proto_raw.starts_with("tcp") {
            continue;
        }
        // BSD's "tcp4" → "tcp", "tcp6" stays "tcp6".
        let proto = if proto_raw == "tcp4" { "tcp" } else { proto_raw };

        let local = tokens[3];
        let foreign = tokens[4];
        // -v output appends extra cols after state. The last token
        // that's a known TCP state is the one we want; scan from the
        // right.
        let state = pick_state(&tokens).unwrap_or_else(|| "UNKNOWN".to_string());

        let (la, lp) = match split_addr_port(local) {
            Some(x) => x,
            None => continue,
        };
        let (fa, fp) = split_addr_port(foreign).unwrap_or_default();

        out.push(Row {
            proto: proto.to_string(),
            local_addr: la,
            local_port: lp,
            peer_addr: fa,
            peer_port: fp,
            state,
        });
    }
    out
}

const KNOWN_STATES: &[&str] = &[
    "LISTEN",
    "ESTABLISHED",
    "SYN_SENT",
    "SYN_RECEIVED",
    "FIN_WAIT_1",
    "FIN_WAIT_2",
    "CLOSE_WAIT",
    "TIME_WAIT",
    "LAST_ACK",
    "CLOSING",
    "CLOSED",
];

fn pick_state(tokens: &[&str]) -> Option<String> {
    for tok in tokens.iter().rev() {
        let upper = tok.to_uppercase();
        if KNOWN_STATES.contains(&upper.as_str()) {
            return Some(upper);
        }
    }
    None
}

// split_addr_port handles BSD netstat's "<addr>.<port>" form. The
// last `.` separates port from address. "*" wildcards stay as-is.
//
// IPv6 addresses contain ':' but no '.' until the port suffix, so
// rsplit_once('.') still picks the right separator.
pub fn split_addr_port(s: &str) -> Option<(String, u16)> {
    if s == "*.*" {
        return Some(("*".to_string(), 0));
    }
    let (addr, port) = s.rsplit_once('.')?;
    if port == "*" {
        return Some((normalise_addr(addr), 0));
    }
    let port: u16 = port.parse().ok()?;
    Some((normalise_addr(addr), port))
}

fn normalise_addr(s: &str) -> String {
    if s == "*" {
        return s.to_string();
    }
    s.to_string()
}

// ---------- exec helper ----------

#[cfg(target_arch = "wasm32")]
fn run_netstat(timeout_ms: u32) -> Result<ExecResponse, String> {
    let args: Vec<String> = vec![
        "-anv".to_string(),
        "-p".to_string(),
        "tcp".to_string(),
    ];
    let req = ExecRequest {
        command: "/usr/sbin/netstat".to_string(),
        args,
        timeout_ms,
    };
    let body = serde_json::to_string(&req).map_err(|e| format!("encode_exec_req: {}", e))?;
    let env: Envelope = unsafe {
        host_exec(body)
            .map_err(|e| format!("host_exec: {}", e))?
            .0
    };
    if !env.ok {
        return Err(env.error);
    }
    serde_json::from_value(env.data).map_err(|e| format!("decode_exec_resp: {}", e))
}

#[cfg(not(target_arch = "wasm32"))]
fn run_netstat(_timeout_ms: u32) -> Result<ExecResponse, String> {
    Err("not available on host build".to_string())
}

// ============================================================
// tests (host-build only — pure parser)
// ============================================================

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn split_addr_port_basic() {
        assert_eq!(
            split_addr_port("127.0.0.1.51234"),
            Some(("127.0.0.1".into(), 51234))
        );
        assert_eq!(split_addr_port("*.22"), Some(("*".into(), 22)));
        assert_eq!(split_addr_port("*.*"), Some(("*".into(), 0)));
        assert_eq!(split_addr_port("::1.443"), Some(("::1".into(), 443)));
    }

    #[test]
    fn split_addr_port_wildcard_port() {
        assert_eq!(split_addr_port("10.0.0.5.*"), Some(("10.0.0.5".into(), 0)));
    }

    #[test]
    fn split_addr_port_rejects_garbage() {
        assert!(split_addr_port("noport").is_none());
    }

    #[test]
    fn pick_state_recovers_from_tail() {
        // -v output adds extra trailing cols (rhiwat, shiwat, pid, …)
        // before / after the state column depending on macOS version.
        let toks = vec![
            "tcp4", "0", "0", "127.0.0.1.22", "127.0.0.1.45678", "ESTABLISHED",
            "131072", "131072", "1234",
        ];
        assert_eq!(pick_state(&toks).as_deref(), Some("ESTABLISHED"));
    }

    #[test]
    fn parse_listeners_and_connections() {
        let stdout = "\
Active Internet connections (including servers)
Proto Recv-Q Send-Q  Local Address          Foreign Address        (state)
tcp4       0      0  *.22                   *.*                    LISTEN
tcp4       0      0  127.0.0.1.51234        93.184.216.34.443      ESTABLISHED
tcp6       0      0  ::1.5000               *.*                    LISTEN
udp4       0      0  *.123                  *.*
";
        let got = parse_netstat_output(stdout);
        assert_eq!(got.len(), 3);
        // tcp4 normalised to "tcp"
        assert_eq!(got[0].proto, "tcp");
        assert_eq!(got[0].local_addr, "*");
        assert_eq!(got[0].local_port, 22);
        assert_eq!(got[0].state, "LISTEN");
        // tcp6 stays
        assert_eq!(got[2].proto, "tcp6");
        assert_eq!(got[2].local_addr, "::1");
    }

    #[test]
    fn parse_skips_udp_and_preamble() {
        let stdout = "\
Active Internet connections
Proto Recv-Q Send-Q  Local Address  Foreign Address  (state)
udp4       0      0  *.123           *.*
";
        assert!(parse_netstat_output(stdout).is_empty());
    }

    #[test]
    fn parse_drops_short_rows() {
        let stdout = "\
Active Internet connections
tcp4 short
";
        assert!(parse_netstat_output(stdout).is_empty());
    }

    #[test]
    fn parse_assigns_unknown_state_to_rows_lacking_one() {
        // Rare but possible: a row truncated mid-output.
        let stdout = "tcp4 0 0 127.0.0.1.80 0.0.0.0.0 garbage1 garbage2\n";
        let got = parse_netstat_output(stdout);
        assert_eq!(got.len(), 1);
        assert_eq!(got[0].state, "UNKNOWN");
    }
}
