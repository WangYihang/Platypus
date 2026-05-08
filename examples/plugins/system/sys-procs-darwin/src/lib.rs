// sys-procs-darwin — macOS process list via `/bin/ps -axwwo …`.
//
// BSD ps emits whitespace-aligned columns whose order is dictated by
// the `-o` arg. We pin the order so the parser doesn't have to reflect
// on the header:
//
//   PID PPID USER %CPU %MEM RSS STAT COMM ARGS…
//
// Every field except ARGS is a single token. ARGS is everything from
// the start of column 9 to end-of-line. STAT can include trailing
// flags ("Ss", "R+", "S<s") — they're a single token.
//
// rss_bytes: BSD ps reports RSS in 1024-byte blocks; multiply.
//
// The wasm side serialises directly into v2pb.ProcessListResponse's
// protojson shape (camelCase keys, snake_case suffixes via
// #[serde(rename = ...)]) so the agent-side bridge can
// protojson.Unmarshal straight through — same wire deal as
// sys-procs-linux.

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
struct ProcessListRequest {
    #[serde(default)]
    top_n: u32,
    #[serde(default)]
    sort_by: String,
}

#[derive(Serialize, Default)]
pub struct ProcessInfo {
    pub pid: u32,
    pub ppid: u32,
    pub user: String,
    pub name: String,
    pub cmdline: String,
    pub status: String,
    #[serde(rename = "cpuPercent")]
    pub cpu_percent: f64,
    #[serde(rename = "memPercent")]
    pub mem_percent: f64,
    #[serde(rename = "rssBytes")]
    pub rss_bytes: u64,
}

#[derive(Serialize, Default)]
struct ProcessListResponse {
    processes: Vec<ProcessInfo>,
    #[serde(rename = "totalCount", skip_serializing_if = "is_zero_u32")]
    total_count: u32,
    #[serde(skip_serializing_if = "String::is_empty")]
    error: String,
}

fn is_zero_u32(n: &u32) -> bool {
    *n == 0
}

const PROC_LIST_CAP: u32 = 500;

#[cfg(target_arch = "wasm32")]
#[plugin_fn]
pub fn process_list(req: Json<ProcessListRequest>) -> FnResult<String> {
    let r = req.0;

    let exec_resp = match run_ps(7_000) {
        Ok(v) => v,
        Err(e) => {
            return Ok(serde_json::to_string(&ProcessListResponse {
                processes: Vec::new(),
                total_count: 0,
                error: e,
            })?)
        }
    };
    if exec_resp.exit_code != 0 {
        return Ok(serde_json::to_string(&ProcessListResponse {
            processes: Vec::new(),
            total_count: 0,
            error: format!(
                "ps exit {}: {}",
                exec_resp.exit_code,
                exec_resp.stderr.trim()
            ),
        })?);
    }

    let mut procs = parse_ps_output(&exec_resp.stdout);
    let total = procs.len() as u32;

    let sort_by = r.sort_by.as_str();
    procs.sort_by(|a, b| match sort_by {
        "mem" | "rss" => b.rss_bytes.cmp(&a.rss_bytes),
        "pid" => a.pid.cmp(&b.pid),
        "cpu" => b
            .cpu_percent
            .partial_cmp(&a.cpu_percent)
            .unwrap_or(std::cmp::Ordering::Equal),
        // Default falls through to RSS (deterministic; %CPU on a
        // single-shot ps reading is mostly noise anyway).
        _ => b.rss_bytes.cmp(&a.rss_bytes),
    });

    let mut top_n = r.top_n;
    if top_n == 0 || top_n > PROC_LIST_CAP {
        top_n = PROC_LIST_CAP;
    }
    if procs.len() > top_n as usize {
        procs.truncate(top_n as usize);
    }

    Ok(serde_json::to_string(&ProcessListResponse {
        processes: procs,
        total_count: total,
        error: String::new(),
    })?)
}

// parse_ps_output handles `/bin/ps -axwwo pid,ppid,user,%cpu,%mem,rss,stat,comm,args`.
// Drops the header row. Splits by whitespace; first 8 tokens are
// fixed-column, everything after is `args` joined with single spaces.
//
// Rows with fewer than 9 tokens (rare; can happen for kernel
// processes whose args are empty) get an empty cmdline. Rows with
// fewer than 8 tokens are dropped.
pub fn parse_ps_output(stdout: &str) -> Vec<ProcessInfo> {
    let mut out = Vec::new();
    let mut lines = stdout.lines();

    // Drop the header. ps writes fixed labels: "PID PPID USER ...".
    // If the first line doesn't look like a header (starts with a
    // numeric pid), the caller probably ran us without -o; treat
    // the whole input as data so callers running tests with
    // header-less fixtures still get something.
    let first = lines.next();
    let trimmed_first = first.map(str::trim_start).unwrap_or_default();
    let header_present = trimmed_first
        .split_whitespace()
        .next()
        .map_or(false, |t| t.eq_ignore_ascii_case("PID"));
    if !header_present {
        if let Some(line) = first {
            if let Some(p) = parse_ps_row(line) {
                out.push(p);
            }
        }
    }

    for line in lines {
        if let Some(p) = parse_ps_row(line) {
            out.push(p);
        }
    }
    out
}

fn parse_ps_row(line: &str) -> Option<ProcessInfo> {
    let trimmed = line.trim();
    if trimmed.is_empty() {
        return None;
    }
    let mut iter = trimmed.split_whitespace();
    let pid: u32 = iter.next()?.parse().ok()?;
    let ppid: u32 = iter.next()?.parse().ok()?;
    let user = iter.next()?.to_string();
    let cpu_percent: f64 = iter.next()?.parse().unwrap_or(0.0);
    let mem_percent: f64 = iter.next()?.parse().unwrap_or(0.0);
    let rss_kb: u64 = iter.next()?.parse().unwrap_or(0);
    let status = iter.next()?.to_string();
    let name = iter.next()?.to_string();
    // `args` is the remainder. .collect() preserves the trailing
    // tail. May be empty (kernel procs) — that's fine.
    let cmdline = iter.collect::<Vec<_>>().join(" ");

    Some(ProcessInfo {
        pid,
        ppid,
        user,
        name,
        cmdline,
        status,
        cpu_percent,
        mem_percent,
        rss_bytes: rss_kb * 1024,
    })
}

#[cfg(target_arch = "wasm32")]
fn run_ps(timeout_ms: u32) -> Result<ExecResponse, String> {
    let args: Vec<String> = vec![
        "-axwwo".to_string(),
        "pid,ppid,user,%cpu,%mem,rss,stat,comm,args".to_string(),
    ];
    let req = ExecRequest {
        command: "/bin/ps".to_string(),
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
fn run_ps(_timeout_ms: u32) -> Result<ExecResponse, String> {
    Err("not available on host build".to_string())
}

// ============================================================
// tests (host-build only — pure parser)
// ============================================================

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parse_basic_three_rows() {
        let stdout = "\
  PID  PPID USER             %CPU %MEM    RSS STAT   COMM             ARGS
    1     0 root              0.1  0.5   1234 Ss     launchd          /sbin/launchd
  123    45 alice             1.5  2.0  10240 R+     bash             -bash
  500     1 _coreaudiod       0.0  0.1    900 S      coreaudiod       /usr/sbin/coreaudiod
";
        let got = parse_ps_output(stdout);
        assert_eq!(got.len(), 3);
        assert_eq!(got[0].pid, 1);
        assert_eq!(got[0].ppid, 0);
        assert_eq!(got[0].user, "root");
        assert_eq!(got[0].name, "launchd");
        assert_eq!(got[0].cmdline, "/sbin/launchd");
        assert_eq!(got[0].status, "Ss");
        assert_eq!(got[0].rss_bytes, 1234 * 1024);
        assert_eq!(got[1].pid, 123);
        assert_eq!(got[1].cpu_percent, 1.5);
        assert_eq!(got[1].cmdline, "-bash");
    }

    #[test]
    fn parse_handles_args_with_spaces() {
        let stdout = "\
  PID  PPID USER %CPU %MEM RSS STAT COMM ARGS
  789    1 root  0.0  0.1 100 S    foo  /usr/bin/foo --flag --bar=quux baz
";
        let got = parse_ps_output(stdout);
        assert_eq!(got.len(), 1);
        assert_eq!(got[0].cmdline, "/usr/bin/foo --flag --bar=quux baz");
    }

    #[test]
    fn parse_handles_kernel_proc_with_empty_args() {
        // ps reports kernel processes with COMM but no ARGS column;
        // trailing whitespace becomes the empty string.
        let stdout = "\
  PID  PPID USER %CPU %MEM RSS STAT COMM ARGS
   42     0 root  0.0  0.0   0 S<   kthreadd
";
        let got = parse_ps_output(stdout);
        assert_eq!(got.len(), 1);
        assert_eq!(got[0].name, "kthreadd");
        assert_eq!(got[0].cmdline, "");
        assert_eq!(got[0].status, "S<");
    }

    #[test]
    fn parse_skips_header_only_input() {
        let stdout = "  PID  PPID USER %CPU %MEM RSS STAT COMM ARGS\n";
        assert!(parse_ps_output(stdout).is_empty());
    }

    #[test]
    fn parse_handles_no_header_input() {
        // A test fixture might omit the header row. The parser
        // should still pick up the data row.
        let stdout = "  10     1 alice  0.0  0.0  100 S bash -bash\n";
        let got = parse_ps_output(stdout);
        assert_eq!(got.len(), 1);
        assert_eq!(got[0].pid, 10);
    }

    #[test]
    fn parse_skips_short_rows() {
        // Fewer than 8 columns — drop, don't panic.
        let stdout = "  PID PPID USER %CPU %MEM RSS STAT COMM ARGS\n  1 2 alice\n";
        assert!(parse_ps_output(stdout).is_empty());
    }

    #[test]
    fn parse_skips_non_numeric_pid() {
        let stdout = "  PID PPID USER %CPU %MEM RSS STAT COMM ARGS\n  garbage 1 alice 0.0 0.0 1 S sh sh\n";
        assert!(parse_ps_output(stdout).is_empty());
    }

    #[test]
    fn parse_returns_empty_for_blank_input() {
        assert!(parse_ps_output("").is_empty());
        assert!(parse_ps_output("   \n   \n").is_empty());
    }

    #[test]
    fn parse_rss_kb_to_bytes() {
        let stdout =
            "  PID PPID USER %CPU %MEM    RSS STAT COMM ARGS\n  1 0 r 0.0 0.0 4096 S a a\n";
        let got = parse_ps_output(stdout);
        assert_eq!(got[0].rss_bytes, 4096 * 1024);
    }
}
