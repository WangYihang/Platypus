// sys-cron-linux — read every standard cron source on the host and
// flatten them into a list of CronJob rows. Pure file reads; no
// shell-out. Five distinct source kinds, each with its own parser:
//
//   /etc/crontab                  crontab format with user field
//   /etc/cron.d/*                 same shape as /etc/crontab
//   /var/spool/cron/{crontabs,}/* per-user crontab (no user field;
//                                 user comes from filename)
//   /etc/cron.{hourly,daily,weekly,monthly}/*  run-parts scripts
//   /etc/anacrontab               anacron format

use extism_pdk::*;
use serde::{Deserialize, Serialize};

#[cfg(target_arch = "wasm32")]
#[host_fn("platypus")]
extern "ExtismHost" {
    fn host_fs_read(path: String) -> Json<Envelope>;
    fn host_fs_listdir(path: String) -> Json<Envelope>;
}

#[derive(Deserialize, Default)]
struct Envelope {
    ok: bool,
    #[serde(default)]
    data: serde_json::Value,
    #[serde(default)]
    error: String,
}

#[derive(Deserialize, Default)]
struct DirEntryJSON {
    name: String,
    #[serde(default)]
    is_dir: bool,
}

// ---- request / response wire shapes ----

#[derive(Deserialize, Default)]
struct ListRequest {
    #[serde(default)]
    include_disabled: bool,
}

#[derive(Serialize, Default)]
struct ListResponse {
    jobs: Vec<CronJob>,
    #[serde(skip_serializing_if = "String::is_empty")]
    error: String,
}

#[derive(Serialize, Default, Debug, PartialEq)]
struct CronJob {
    source: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    user: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    schedule: String,
    command: String,
    kind: String,
    #[serde(rename = "lineNo", skip_serializing_if = "is_zero_u32")]
    line_no: u32,
    enabled: bool,
}

fn is_zero_u32(n: &u32) -> bool {
    *n == 0
}

// ---- entry point ----

#[cfg(target_arch = "wasm32")]
#[plugin_fn]
pub fn list_cron_jobs(req: Json<ListRequest>) -> FnResult<String> {
    let r = req.0;
    let mut jobs = Vec::new();

    // /etc/crontab — system crontab WITH user field.
    if let Some(body) = read_string("/etc/crontab") {
        jobs.extend(parse_system_crontab("/etc/crontab", &body, "system_crontab"));
    }
    // /etc/cron.d/* — same shape.
    if let Some(entries) = list_dir("/etc/cron.d") {
        for e in entries.into_iter().filter(|e| !e.is_dir && !e.name.starts_with('.')) {
            let path = format!("/etc/cron.d/{}", e.name);
            if let Some(body) = read_string(&path) {
                jobs.extend(parse_system_crontab(&path, &body, "cron_d"));
            }
        }
    }
    // /var/spool/cron/crontabs/* — Debian/Ubuntu per-user.
    jobs.extend(read_user_crontabs("/var/spool/cron/crontabs"));
    // /var/spool/cron/* — RHEL/Fedora per-user. Skip the
    // "crontabs" subdir we already handled above.
    jobs.extend(read_user_crontabs("/var/spool/cron"));
    // /etc/cron.{hourly,daily,weekly,monthly}/* — run-parts.
    for cadence in &["hourly", "daily", "weekly", "monthly"] {
        let dir = format!("/etc/cron.{cadence}");
        if let Some(entries) = list_dir(&dir) {
            for e in entries.into_iter().filter(|e| !e.is_dir && !e.name.starts_with('.')) {
                jobs.push(CronJob {
                    source: dir.clone(),
                    user: "root".to_string(),
                    schedule: (*cadence).to_string(),
                    command: format!("{dir}/{}", e.name),
                    kind: "run_parts".to_string(),
                    line_no: 0,
                    enabled: true,
                });
            }
        }
    }
    // /etc/anacrontab — anacron schedules.
    if let Some(body) = read_string("/etc/anacrontab") {
        jobs.extend(parse_anacrontab(&body));
    }

    if !r.include_disabled {
        jobs.retain(|j| j.enabled);
    }

    Ok(serde_json::to_string(&ListResponse {
        jobs,
        error: String::new(),
    })?)
}

#[cfg(target_arch = "wasm32")]
fn read_user_crontabs(dir: &str) -> Vec<CronJob> {
    let mut out = Vec::new();
    let entries = match list_dir(dir) {
        Some(v) => v,
        None => return out,
    };
    for e in entries.into_iter().filter(|e| !e.is_dir && !e.name.starts_with('.')) {
        // Skip the "crontabs" subdir on Debian when we're walking
        // /var/spool/cron itself — the user listing lives one level
        // deeper there. (Debian: /var/spool/cron/crontabs/<user>;
        // RHEL: /var/spool/cron/<user>.)
        if dir == "/var/spool/cron" && e.name == "crontabs" {
            continue;
        }
        let path = format!("{dir}/{}", e.name);
        if let Some(body) = read_string(&path) {
            out.extend(parse_user_crontab(&path, &e.name, &body));
        }
    }
    out
}

// ---- parsers (pure, host-testable) ----

// parse_system_crontab handles the format used by /etc/crontab and
// /etc/cron.d/*: each non-comment line is
//   <minute> <hour> <dom> <mon> <dow> <user> <command>
// or a shorthand `@<keyword> <user> <command>` (where <keyword>
// is reboot/yearly/monthly/weekly/daily/hourly). Comments and
// VAR=value lines are skipped. include_disabled rerouting happens
// in the caller; the parser sets enabled=true for live lines and
// scans commented lines as enabled=false (so disabled-job audits
// can find them).
fn parse_system_crontab(path: &str, body: &str, kind: &str) -> Vec<CronJob> {
    let mut out = Vec::new();
    for (i, raw) in body.lines().enumerate() {
        let (text, enabled) = strip_comment_marker(raw);
        let trimmed = text.trim();
        if trimmed.is_empty() || is_var_assignment(trimmed) {
            continue;
        }
        let (schedule, after) = match split_schedule(trimmed) {
            Some(p) => p,
            None => continue,
        };
        // Next token is user, rest is command.
        let after = after.trim_start();
        let space = match after.find(char::is_whitespace) {
            Some(i) => i,
            None => continue,
        };
        let user = after[..space].to_string();
        let command = after[space..].trim().to_string();
        if command.is_empty() {
            continue;
        }
        out.push(CronJob {
            source: path.to_string(),
            user,
            schedule,
            command,
            kind: kind.to_string(),
            line_no: (i + 1) as u32,
            enabled,
        });
    }
    out
}

// parse_user_crontab is the same as parse_system_crontab minus the
// user field — the user comes from the filename.
fn parse_user_crontab(path: &str, user: &str, body: &str) -> Vec<CronJob> {
    let mut out = Vec::new();
    for (i, raw) in body.lines().enumerate() {
        let (text, enabled) = strip_comment_marker(raw);
        let trimmed = text.trim();
        if trimmed.is_empty() || is_var_assignment(trimmed) {
            continue;
        }
        let (schedule, after) = match split_schedule(trimmed) {
            Some(p) => p,
            None => continue,
        };
        let command = after.trim().to_string();
        if command.is_empty() {
            continue;
        }
        out.push(CronJob {
            source: path.to_string(),
            user: user.to_string(),
            schedule,
            command,
            kind: "crontab".to_string(),
            line_no: (i + 1) as u32,
            enabled,
        });
    }
    out
}

// parse_anacrontab handles /etc/anacrontab. Format:
//   <period_days> <delay_minutes> <job_id> <command>
// plus VAR=value assignments + comments. Periods can be the
// symbolic "@daily"/"@weekly"/"@monthly".
fn parse_anacrontab(body: &str) -> Vec<CronJob> {
    let mut out = Vec::new();
    for (i, raw) in body.lines().enumerate() {
        let (text, enabled) = strip_comment_marker(raw);
        let trimmed = text.trim();
        if trimmed.is_empty() || is_var_assignment(trimmed) {
            continue;
        }
        // Anacron lines have at least 4 tokens.
        let parts: Vec<&str> = trimmed.splitn(4, char::is_whitespace).collect();
        if parts.len() < 4 {
            continue;
        }
        let schedule = format!("{} (delay {}min)", parts[0], parts[1]);
        let job_id = parts[2].to_string();
        let command = parts[3].trim().to_string();
        if command.is_empty() {
            continue;
        }
        out.push(CronJob {
            source: "/etc/anacrontab".to_string(),
            user: "root".to_string(),
            schedule,
            command: format!("[{job_id}] {command}"),
            kind: "anacron".to_string(),
            line_no: (i + 1) as u32,
            enabled,
        });
    }
    out
}

// strip_comment_marker peels a leading `#` off the line if present
// and reports whether the line was a live entry. Trailing inline
// comments (after the command) are preserved — cron interprets a
// `#` mid-command literally, so peeling them would corrupt the
// command. Returns (text-without-leading-#, enabled).
fn strip_comment_marker(raw: &str) -> (&str, bool) {
    let trimmed_start = raw.trim_start();
    if let Some(rest) = trimmed_start.strip_prefix('#') {
        // Disabled — return the rest, less the leading whitespace.
        // Heuristic: if the rest doesn't look like a job
        // (no schedule), parse_* will skip it anyway.
        return (rest, false);
    }
    (raw, true)
}

fn is_var_assignment(line: &str) -> bool {
    // VAR=value lines have an `=` before any whitespace AND no
    // leading digit / `@` / `*` (which would mark a schedule).
    let bytes = line.as_bytes();
    if bytes.is_empty() {
        return false;
    }
    let first = bytes[0];
    if first == b'@' || first == b'*' || first.is_ascii_digit() {
        return false;
    }
    let eq = match line.find('=') {
        Some(i) => i,
        None => return false,
    };
    let ws = line.find(char::is_whitespace).unwrap_or(usize::MAX);
    eq < ws
}

// split_schedule peels the schedule off the front of a crontab
// line. Returns (schedule, rest) on success. Handles:
//   - "@reboot ..." / "@daily ..." / "@yearly ..." etc.
//   - "M H DOM MON DOW ..." (5 whitespace-separated fields)
fn split_schedule(line: &str) -> Option<(String, &str)> {
    let trimmed = line.trim_start();
    if trimmed.starts_with('@') {
        // Shorthand: one token, then rest.
        let space = trimmed.find(char::is_whitespace)?;
        return Some((trimmed[..space].to_string(), &trimmed[space..]));
    }
    // Five fields separated by whitespace. Walk char-by-char so the
    // schedule string preserves its original spacing.
    let mut field = 0;
    let mut prev_was_ws = true;
    let mut transitions = 0;
    let bytes = trimmed.as_bytes();
    let mut split_at = None;
    for (i, &b) in bytes.iter().enumerate() {
        let is_ws = b == b' ' || b == b'\t';
        if is_ws && !prev_was_ws {
            transitions += 1;
            field += 1;
            if field == 5 {
                split_at = Some(i);
                break;
            }
        }
        prev_was_ws = is_ws;
    }
    let split_at = split_at?;
    if transitions < 5 {
        return None;
    }
    Some((trimmed[..split_at].to_string(), &trimmed[split_at..]))
}

// ---- host helpers ----

#[cfg(target_arch = "wasm32")]
fn read_string(path: &str) -> Option<String> {
    let env: Envelope = unsafe { host_fs_read(path.to_string()).ok()?.0 };
    if !env.ok {
        return None;
    }
    env.data.as_str().map(|s| s.to_string())
}

#[cfg(target_arch = "wasm32")]
fn list_dir(path: &str) -> Option<Vec<DirEntryJSON>> {
    let env: Envelope = unsafe { host_fs_listdir(path.to_string()).ok()?.0 };
    if !env.ok {
        return None;
    }
    serde_json::from_value(env.data).ok()
}

// ============================================================
// Pure-function unit tests
// ============================================================
#[cfg(test)]
mod tests {
    use super::*;

    // ---- split_schedule ----

    #[test]
    fn split_five_field_schedule() {
        let (sched, rest) = split_schedule("0 5 * * * root /usr/bin/foo").unwrap();
        assert_eq!(sched, "0 5 * * *");
        assert_eq!(rest.trim_start(), "root /usr/bin/foo");
    }

    #[test]
    fn split_shorthand_reboot() {
        let (sched, rest) = split_schedule("@reboot /usr/bin/foo").unwrap();
        assert_eq!(sched, "@reboot");
        assert_eq!(rest.trim_start(), "/usr/bin/foo");
    }

    #[test]
    fn split_shorthand_daily() {
        let (sched, rest) = split_schedule("@daily root /usr/bin/foo").unwrap();
        assert_eq!(sched, "@daily");
        assert!(rest.contains("root"));
    }

    #[test]
    fn split_too_few_fields() {
        assert!(split_schedule("0 5 * *").is_none());
    }

    #[test]
    fn split_compound_schedule_preserves_spacing() {
        // Crontab fields can contain commas, slashes, ranges.
        let (sched, _) = split_schedule("*/15 0,12 * * 1-5 root job").unwrap();
        assert_eq!(sched, "*/15 0,12 * * 1-5");
    }

    // ---- is_var_assignment ----

    #[test]
    fn var_assignment_caught() {
        assert!(is_var_assignment("PATH=/usr/bin"));
        assert!(is_var_assignment("MAILTO=ops@example.com"));
        assert!(is_var_assignment("FOO=bar=baz"));
    }

    #[test]
    fn schedule_not_var_assignment() {
        assert!(!is_var_assignment("0 5 * * * root foo"));
        assert!(!is_var_assignment("@reboot /bin/foo"));
        assert!(!is_var_assignment("*/5 * * * * foo"));
    }

    // ---- strip_comment_marker ----

    #[test]
    fn strip_comment_disabled() {
        let (text, enabled) = strip_comment_marker("# 0 5 * * * root foo");
        assert!(!enabled);
        assert_eq!(text.trim(), "0 5 * * * root foo");
    }

    #[test]
    fn strip_comment_indented() {
        let (text, enabled) = strip_comment_marker("   # 0 5 * * * root foo");
        assert!(!enabled);
        assert_eq!(text.trim(), "0 5 * * * root foo");
    }

    #[test]
    fn strip_no_comment() {
        let (text, enabled) = strip_comment_marker("0 5 * * * root foo");
        assert!(enabled);
        assert_eq!(text, "0 5 * * * root foo");
    }

    // ---- parse_system_crontab ----

    #[test]
    fn parse_etc_crontab_basic() {
        let body = "\
# /etc/crontab
SHELL=/bin/sh
PATH=/usr/local/sbin:/usr/local/bin:/sbin:/bin:/usr/sbin:/usr/bin

17 *	* * *	root    cd / && run-parts --report /etc/cron.hourly
25 6	* * *	root	test -x /usr/sbin/anacron || /etc/cron.daily-deb run
";
        let jobs = parse_system_crontab("/etc/crontab", body, "system_crontab");
        assert_eq!(jobs.len(), 2);
        assert_eq!(jobs[0].user, "root");
        assert_eq!(jobs[0].schedule, "17 *	* * *");
        assert!(jobs[0].command.contains("run-parts"));
        assert_eq!(jobs[0].kind, "system_crontab");
        assert!(jobs[0].enabled);
    }

    #[test]
    fn parse_disabled_lines_marked() {
        let body = "\
# 0 5 * * * root /usr/bin/disabled-job
0 6 * * * root /usr/bin/active-job
";
        let jobs = parse_system_crontab("/etc/crontab", body, "system_crontab");
        assert_eq!(jobs.len(), 2);
        let disabled = jobs.iter().find(|j| !j.enabled).unwrap();
        assert!(disabled.command.contains("disabled-job"));
        let active = jobs.iter().find(|j| j.enabled).unwrap();
        assert!(active.command.contains("active-job"));
    }

    #[test]
    fn parse_skips_var_lines_and_blanks() {
        let body = "\
SHELL=/bin/sh

PATH=/usr/bin
0 5 * * * root /usr/bin/foo
";
        let jobs = parse_system_crontab("/etc/crontab", body, "system_crontab");
        assert_eq!(jobs.len(), 1);
    }

    #[test]
    fn parse_shorthand_in_system_crontab() {
        let body = "@reboot root /usr/sbin/onreboot.sh\n";
        let jobs = parse_system_crontab("/etc/crontab", body, "system_crontab");
        assert_eq!(jobs.len(), 1);
        assert_eq!(jobs[0].schedule, "@reboot");
        assert_eq!(jobs[0].user, "root");
        assert!(jobs[0].command.ends_with("onreboot.sh"));
    }

    // ---- parse_user_crontab ----

    #[test]
    fn parse_user_crontab_basic() {
        let body = "\
# alice's cron
0 8 * * * /home/alice/bin/morning.sh
@daily /home/alice/bin/cleanup.sh
";
        let jobs = parse_user_crontab(
            "/var/spool/cron/crontabs/alice",
            "alice",
            body,
        );
        assert_eq!(jobs.len(), 2);
        for j in &jobs {
            assert_eq!(j.user, "alice");
            assert_eq!(j.kind, "crontab");
        }
        assert_eq!(jobs[1].schedule, "@daily");
    }

    // ---- parse_anacrontab ----

    #[test]
    fn parse_anacrontab_basic() {
        let body = "\
SHELL=/bin/sh

1	5	cron.daily	run-parts --report /etc/cron.daily
7	25	cron.weekly	run-parts --report /etc/cron.weekly
";
        let jobs = parse_anacrontab(body);
        assert_eq!(jobs.len(), 2);
        assert_eq!(jobs[0].schedule, "1 (delay 5min)");
        assert!(jobs[0].command.starts_with("[cron.daily]"));
        assert_eq!(jobs[0].user, "root");
        assert_eq!(jobs[0].kind, "anacron");
    }

    #[test]
    fn parse_anacrontab_skips_short_lines() {
        let body = "1 5 cron.daily\n";
        // Only 3 tokens — no command. Should be skipped.
        assert!(parse_anacrontab(body).is_empty());
    }
}
