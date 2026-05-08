// sys-net-windows — Get-NetTCPConnection via PowerShell.
//
// PS pipeline emits one JSON object per TCP socket:
//   { LocalAddress: "0.0.0.0", LocalPort: 22, RemoteAddress: "0.0.0.0",
//     RemotePort: 0, State: 2, OwningProcess: 1234 }
//
// State is an integer enum (Microsoft.Management.Infrastructure /
// MSFT_NetTCPConnection State enum):
//   1=Closed, 2=Listen, 3=SynSent, 4=SynReceived, 5=Established,
//   6=FinWait1, 7=FinWait2, 8=CloseWait, 9=Closing, 10=LastAck,
//   11=TimeWait, 12=DeleteTCB
//
// Mapped to the linux/darwin uppercase string forms for wire
// uniformity.

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
struct ListListenersRequest {
    #[serde(default)]
    offset: u32,
    #[serde(default)]
    limit: u32,
}

#[derive(Deserialize, Default)]
struct ListConnectionsRequest {
    #[serde(default)]
    state: String,
    #[serde(default)]
    offset: u32,
    #[serde(default)]
    limit: u32,
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
    #[serde(rename = "totalCount", skip_serializing_if = "is_zero_u32")]
    total_count: u32,
    #[serde(rename = "hasMore", skip_serializing_if = "is_false")]
    has_more: bool,
}

#[derive(Serialize, Default)]
struct ListConnectionsResponse {
    connections: Vec<Connection>,
    #[serde(skip_serializing_if = "String::is_empty")]
    error: String,
    #[serde(rename = "totalCount", skip_serializing_if = "is_zero_u32")]
    total_count: u32,
    #[serde(rename = "hasMore", skip_serializing_if = "is_false")]
    has_more: bool,
}

fn is_zero_u32(n: &u32) -> bool { *n == 0 }
fn is_false(b: &bool) -> bool { !*b }

const DEFAULT_LIMIT: u32 = 200;
const HARD_LIMIT: u32 = 5_000;

fn effective_limit(requested: u32) -> u32 {
    let n = if requested == 0 { DEFAULT_LIMIT } else { requested };
    n.min(HARD_LIMIT)
}

fn paginate<T>(items: Vec<T>, offset: u32, limit: u32) -> (Vec<T>, u32, bool) {
    let total = items.len() as u32;
    let off = (offset as usize).min(items.len());
    let lim = effective_limit(limit) as usize;
    let end = (off + lim).min(items.len());
    let slice: Vec<T> = items.into_iter().skip(off).take(end - off).collect();
    let has_more = (off + slice.len()) < total as usize;
    (slice, total, has_more)
}

const PS_SCRIPT: &str = "[Console]::OutputEncoding = [System.Text.Encoding]::UTF8; \
    Get-NetTCPConnection | \
    Select-Object LocalAddress,LocalPort,RemoteAddress,RemotePort,State,OwningProcess | \
    ConvertTo-Json -Compress -Depth 2";

#[cfg(target_arch = "wasm32")]
#[plugin_fn]
pub fn list_listeners(req: Json<ListListenersRequest>) -> FnResult<String> {
    let r = req.0;
    let exec_resp = match run_powershell(12_000) {
        Ok(v) => v,
        Err(e) => {
            return Ok(serde_json::to_string(&ListListenersResponse {
                listeners: Vec::new(),
                error: e,
                ..Default::default()
            })?)
        }
    };
    if exec_resp.exit_code != 0 {
        return Ok(serde_json::to_string(&ListListenersResponse {
            listeners: Vec::new(),
            error: format!(
                "powershell exit {}: {}",
                exec_resp.exit_code,
                exec_resp.stderr.trim()
            ),
            ..Default::default()
        })?);
    }
    let rows = parse_powershell_json(&exec_resp.stdout);
    let listeners: Vec<Listener> = rows
        .into_iter()
        .filter(|r| r.state == "LISTEN")
        .map(|r| Listener {
            proto: row_proto(&r),
            local_addr: r.local_addr,
            local_port: r.local_port,
        })
        .collect();
    let (sliced, total, has_more) = paginate(listeners, r.offset, r.limit);
    Ok(serde_json::to_string(&ListListenersResponse {
        listeners: sliced,
        error: String::new(),
        total_count: total,
        has_more,
    })?)
}

#[cfg(target_arch = "wasm32")]
#[plugin_fn]
pub fn list_connections(req: Json<ListConnectionsRequest>) -> FnResult<String> {
    let r = req.0;
    let want_state = r.state.to_uppercase();
    let exec_resp = match run_powershell(12_000) {
        Ok(v) => v,
        Err(e) => {
            return Ok(serde_json::to_string(&ListConnectionsResponse {
                connections: Vec::new(),
                error: e,
                ..Default::default()
            })?)
        }
    };
    if exec_resp.exit_code != 0 {
        return Ok(serde_json::to_string(&ListConnectionsResponse {
            connections: Vec::new(),
            error: format!(
                "powershell exit {}: {}",
                exec_resp.exit_code,
                exec_resp.stderr.trim()
            ),
            ..Default::default()
        })?);
    }
    let rows = parse_powershell_json(&exec_resp.stdout);
    let conns: Vec<Connection> = rows
        .into_iter()
        .filter(|r| want_state.is_empty() || r.state == want_state)
        .map(|r| Connection {
            proto: row_proto(&r),
            local_addr: r.local_addr.clone(),
            local_port: r.local_port,
            peer_addr: r.peer_addr.clone(),
            peer_port: r.peer_port,
            state: r.state,
        })
        .collect();
    let (sliced, total, has_more) = paginate(conns, r.offset, r.limit);
    Ok(serde_json::to_string(&ListConnectionsResponse {
        connections: sliced,
        error: String::new(),
        total_count: total,
        has_more,
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

fn row_proto(row: &Row) -> String {
    // Get-NetTCPConnection collapses tcp + tcp6 into a single
    // pipeline; differentiate by address shape.
    if row.local_addr.contains(':') {
        "tcp6".to_string()
    } else {
        "tcp".to_string()
    }
}

// parse_powershell_json handles array, single-object, and empty
// pipeline shapes (same as sys-procs-windows / sys-disk-windows).
pub fn parse_powershell_json(stdout: &str) -> Vec<Row> {
    let trimmed = stdout.trim();
    if trimmed.is_empty() || trimmed == "null" {
        return Vec::new();
    }
    let v: serde_json::Value = match serde_json::from_str(trimmed) {
        Ok(v) => v,
        Err(_) => return Vec::new(),
    };
    let rows: Vec<&serde_json::Value> = match &v {
        serde_json::Value::Array(arr) => arr.iter().collect(),
        serde_json::Value::Object(_) => vec![&v],
        _ => return Vec::new(),
    };
    rows.into_iter().filter_map(extract_row).collect()
}

fn extract_row(row: &serde_json::Value) -> Option<Row> {
    let obj = row.as_object()?;
    let local_addr = obj
        .get("LocalAddress")
        .and_then(|x| x.as_str())
        .unwrap_or_default()
        .to_string();
    let peer_addr = obj
        .get("RemoteAddress")
        .and_then(|x| x.as_str())
        .unwrap_or_default()
        .to_string();
    let local_port = pick_u16(obj, "LocalPort");
    let peer_port = pick_u16(obj, "RemotePort");
    let state_int = pick_u32(obj, "State");
    let state = decode_state(state_int).to_string();
    Some(Row {
        local_addr,
        local_port,
        peer_addr,
        peer_port,
        state,
    })
}

fn pick_u16(obj: &serde_json::Map<String, serde_json::Value>, key: &str) -> u16 {
    let val = match obj.get(key) {
        Some(v) => v,
        None => return 0,
    };
    if let Some(n) = val.as_u64() {
        return (n & 0xFFFF) as u16;
    }
    if let Some(s) = val.as_str() {
        return s.parse().unwrap_or(0);
    }
    0
}

fn pick_u32(obj: &serde_json::Map<String, serde_json::Value>, key: &str) -> u32 {
    let val = match obj.get(key) {
        Some(v) => v,
        None => return 0,
    };
    if let Some(n) = val.as_u64() {
        return n as u32;
    }
    if let Some(s) = val.as_str() {
        return s.parse().unwrap_or(0);
    }
    0
}

// decode_state maps the MSFT_NetTCPConnection State enum to the
// uppercase string used by sys-net-linux + sys-net-darwin.
pub fn decode_state(n: u32) -> &'static str {
    match n {
        1 => "CLOSED",
        2 => "LISTEN",
        3 => "SYN_SENT",
        4 => "SYN_RECEIVED",
        5 => "ESTABLISHED",
        6 => "FIN_WAIT_1",
        7 => "FIN_WAIT_2",
        8 => "CLOSE_WAIT",
        9 => "CLOSING",
        10 => "LAST_ACK",
        11 => "TIME_WAIT",
        12 => "DELETE_TCB",
        _ => "UNKNOWN",
    }
}

#[cfg(target_arch = "wasm32")]
fn run_powershell(timeout_ms: u32) -> Result<ExecResponse, String> {
    let args: Vec<String> = vec![
        "-NoProfile".to_string(),
        "-NonInteractive".to_string(),
        "-OutputFormat".to_string(),
        "Text".to_string(),
        "-Command".to_string(),
        PS_SCRIPT.to_string(),
    ];
    let req = ExecRequest {
        command: "C:\\Windows\\System32\\WindowsPowerShell\\v1.0\\powershell.exe".to_string(),
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
fn run_powershell(_timeout_ms: u32) -> Result<ExecResponse, String> {
    Err("not available on host build".to_string())
}

// ============================================================
// tests
// ============================================================

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn decode_state_known_codes() {
        assert_eq!(decode_state(2), "LISTEN");
        assert_eq!(decode_state(5), "ESTABLISHED");
        assert_eq!(decode_state(11), "TIME_WAIT");
        assert_eq!(decode_state(99), "UNKNOWN");
    }

    #[test]
    fn parse_array_two_rows() {
        let stdout = r#"[
            {"LocalAddress":"0.0.0.0","LocalPort":22,"RemoteAddress":"0.0.0.0","RemotePort":0,"State":2,"OwningProcess":1234},
            {"LocalAddress":"192.168.1.5","LocalPort":52341,"RemoteAddress":"93.184.216.34","RemotePort":443,"State":5,"OwningProcess":5678}
        ]"#;
        let got = parse_powershell_json(stdout);
        assert_eq!(got.len(), 2);
        assert_eq!(got[0].state, "LISTEN");
        assert_eq!(got[0].local_port, 22);
        assert_eq!(got[1].state, "ESTABLISHED");
        assert_eq!(got[1].local_addr, "192.168.1.5");
        assert_eq!(got[1].peer_port, 443);
    }

    #[test]
    fn parse_single_row_no_array() {
        let stdout = r#"{"LocalAddress":"::","LocalPort":80,"RemoteAddress":"::","RemotePort":0,"State":2,"OwningProcess":1}"#;
        let got = parse_powershell_json(stdout);
        assert_eq!(got.len(), 1);
        assert_eq!(got[0].local_addr, "::");
    }

    #[test]
    fn parse_handles_stringified_ints() {
        // PS sometimes stringifies large or empty integers.
        let stdout = r#"{"LocalAddress":"0.0.0.0","LocalPort":"22","RemoteAddress":"0.0.0.0","RemotePort":"0","State":"2","OwningProcess":"4"}"#;
        let got = parse_powershell_json(stdout);
        assert_eq!(got[0].local_port, 22);
        assert_eq!(got[0].state, "LISTEN");
    }

    #[test]
    fn parse_empty_or_null_or_garbage() {
        assert!(parse_powershell_json("").is_empty());
        assert!(parse_powershell_json("null").is_empty());
        assert!(parse_powershell_json("not json").is_empty());
    }

    #[test]
    fn row_proto_distinguishes_v4_v6() {
        let v4 = Row {
            local_addr: "127.0.0.1".into(),
            ..Default::default()
        };
        let v6 = Row {
            local_addr: "::1".into(),
            ..Default::default()
        };
        assert_eq!(row_proto(&v4), "tcp");
        assert_eq!(row_proto(&v6), "tcp6");
    }
}
