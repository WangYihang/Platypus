// sys-tasks-windows — Windows Scheduled Tasks inventory.
//
// Shells PowerShell:
//   Get-ScheduledTask + Get-ScheduledTaskInfo → ConvertTo-Json.
//
// Trigger objects on a ScheduledTask have wildly varying shapes
// (DailyTrigger / TimeTrigger / LogonTrigger / BootTrigger / …),
// so we ask PowerShell to render them with .ToString() into a
// single human-readable line per trigger, and ship that. Operators
// who want the raw COM Trigger object can drop down to Get-
// ScheduledTask directly via sys-process exec; this plugin
// optimises for the common "what's scheduled and when" view.

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

// ---- request / response wire shapes ----

#[derive(Deserialize, Default)]
struct ListRequest {
    #[serde(default)]
    include_disabled: bool,
    #[serde(default)]
    filter: String,
    #[serde(default)]
    path_prefix: String,
    #[serde(default)]
    offset: u32,
    #[serde(default)]
    limit: u32,
}

#[derive(Serialize, Default)]
struct ListResponse {
    tasks: Vec<ScheduledTask>,
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
const HARD_LIMIT: u32 = 2_000;

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

#[derive(Serialize, Deserialize, Default, Debug, PartialEq)]
#[serde(default)]
struct ScheduledTask {
    #[serde(rename = "taskName")]
    task_name: String,
    #[serde(rename = "taskPath")]
    task_path: String,
    state: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    author: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    description: String,
    #[serde(skip_serializing_if = "Vec::is_empty")]
    triggers: Vec<String>,
    #[serde(skip_serializing_if = "Vec::is_empty")]
    actions: Vec<TaskAction>,
    #[serde(rename = "lastRunUnix", skip_serializing_if = "is_zero_i64")]
    last_run_unix: i64,
    #[serde(rename = "lastResult", skip_serializing_if = "is_zero_i32")]
    last_result: i32,
    #[serde(rename = "nextRunUnix", skip_serializing_if = "is_zero_i64")]
    next_run_unix: i64,
    #[serde(rename = "runAsUser", skip_serializing_if = "String::is_empty")]
    run_as_user: String,
}

#[derive(Serialize, Deserialize, Default, Debug, PartialEq)]
struct TaskAction {
    #[serde(default)]
    execute: String,
    #[serde(default)]
    arguments: String,
    #[serde(default, rename = "workingDir")]
    working_dir: String,
}

fn is_zero_i64(n: &i64) -> bool {
    *n == 0
}
fn is_zero_i32(n: &i32) -> bool {
    *n == 0
}

// ---- entry point ----

const PS_SCRIPT: &str = r#"[Console]::OutputEncoding = [System.Text.Encoding]::UTF8;
$out = Get-ScheduledTask | ForEach-Object {
    $task = $_
    $info = $null
    try { $info = Get-ScheduledTaskInfo -TaskPath $task.TaskPath -TaskName $task.TaskName -ErrorAction Stop } catch {}
    $triggers = @()
    foreach ($t in $task.Triggers) {
        try { $triggers += $t.ToString() } catch { $triggers += $t.GetType().Name }
    }
    $actions = @()
    foreach ($a in $task.Actions) {
        $exec = ''
        $args = ''
        $cwd = ''
        if ($a.PSObject.Properties.Match('Execute').Count -gt 0)         { $exec = [string]$a.Execute }
        if ($a.PSObject.Properties.Match('Arguments').Count -gt 0)       { $args = [string]$a.Arguments }
        if ($a.PSObject.Properties.Match('WorkingDirectory').Count -gt 0){ $cwd  = [string]$a.WorkingDirectory }
        if ([string]::IsNullOrEmpty($exec)) { $exec = '<' + $a.GetType().Name + '>' }
        $actions += [pscustomobject]@{ execute = $exec; arguments = $args; workingDir = $cwd }
    }
    $lastRun = 0; $lastResult = 0; $nextRun = 0
    if ($info -ne $null) {
        if ($info.LastRunTime -ne $null -and $info.LastRunTime.Year -gt 1700) {
            $lastRun = [int][double]::Parse(($info.LastRunTime.ToUniversalTime().Subtract([datetime]'1970-01-01').TotalSeconds.ToString()))
        }
        if ($info.LastTaskResult -ne $null) { $lastResult = [int]$info.LastTaskResult }
        if ($info.NextRunTime -ne $null -and $info.NextRunTime.Year -gt 1700) {
            $nextRun = [int][double]::Parse(($info.NextRunTime.ToUniversalTime().Subtract([datetime]'1970-01-01').TotalSeconds.ToString()))
        }
    }
    [pscustomobject]@{
        taskName    = $task.TaskName
        taskPath    = $task.TaskPath
        state       = [string]$task.State
        author      = [string]$task.Author
        description = [string]$task.Description
        triggers    = $triggers
        actions     = $actions
        lastRunUnix = $lastRun
        lastResult  = $lastResult
        nextRunUnix = $nextRun
        runAsUser   = if ($task.Principal -ne $null) { [string]$task.Principal.UserId } else { '' }
    }
}
$out | ConvertTo-Json -Compress -Depth 5"#;

#[cfg(target_arch = "wasm32")]
#[plugin_fn]
pub fn list_tasks(req: Json<ListRequest>) -> FnResult<String> {
    let r = req.0;
    let exec_resp = match run_powershell(PS_SCRIPT, 50_000) {
        Ok(v) => v,
        Err(e) => {
            return Ok(serde_json::to_string(&ListResponse {
                tasks: Vec::new(),
                error: e,
                ..Default::default()
            })?)
        }
    };
    if exec_resp.exit_code != 0 {
        return Ok(serde_json::to_string(&ListResponse {
            tasks: Vec::new(),
            error: format!(
                "powershell exit {}: {}",
                exec_resp.exit_code,
                exec_resp.stderr.trim()
            ),
            ..Default::default()
        })?);
    }
    let parsed = parse_powershell_output(&exec_resp.stdout);
    let filtered = filter_tasks(parsed, &r);
    let (sliced, total, has_more) = paginate(filtered, r.offset, r.limit);
    Ok(serde_json::to_string(&ListResponse {
        tasks: sliced,
        error: String::new(),
        total_count: total,
        has_more,
    })?)
}

// ---- pure parsers ----

// parse_powershell_output handles the two shapes ConvertTo-Json
// emits: a single object (when there's one task) and an array (when
// there are many). Empty / null output yields an empty Vec.
fn parse_powershell_output(stdout: &str) -> Vec<ScheduledTask> {
    let trimmed = stdout.trim();
    if trimmed.is_empty() || trimmed == "null" {
        return Vec::new();
    }
    // Try array first — that's the >1-task case (the common one).
    if let Ok(v) = serde_json::from_str::<Vec<ScheduledTask>>(trimmed) {
        return v;
    }
    // Single object fallback.
    if let Ok(t) = serde_json::from_str::<ScheduledTask>(trimmed) {
        return vec![t];
    }
    Vec::new()
}

// filter_tasks applies the request's include_disabled / filter /
// path_prefix knobs.
fn filter_tasks(tasks: Vec<ScheduledTask>, req: &ListRequest) -> Vec<ScheduledTask> {
    let filter_lower = req.filter.to_ascii_lowercase();
    let prefix_lower = req.path_prefix.to_ascii_lowercase();
    tasks
        .into_iter()
        .filter(|t| {
            if !req.include_disabled && t.state.eq_ignore_ascii_case("Disabled") {
                return false;
            }
            if !filter_lower.is_empty()
                && !t.task_name.to_ascii_lowercase().contains(&filter_lower)
            {
                return false;
            }
            if !prefix_lower.is_empty()
                && !t.task_path.to_ascii_lowercase().starts_with(&prefix_lower)
            {
                return false;
            }
            true
        })
        .collect()
}

// ---- exec helper ----

#[cfg(target_arch = "wasm32")]
fn run_powershell(script: &str, timeout_ms: u32) -> Result<ExecResponse, String> {
    let req = ExecRequest {
        command: "C:\\Windows\\System32\\WindowsPowerShell\\v1.0\\powershell.exe"
            .to_string(),
        args: vec![
            "-NoProfile".to_string(),
            "-NonInteractive".to_string(),
            "-OutputFormat".to_string(),
            "Text".to_string(),
            "-Command".to_string(),
            script.to_string(),
        ],
        timeout_ms,
    };
    let body = serde_json::to_string(&req).map_err(|e| format!("encode_exec_req: {e}"))?;
    let env: Envelope = unsafe {
        host_exec(body)
            .map_err(|e| format!("host_exec: {e}"))?
            .0
    };
    if !env.ok {
        return Err(env.error);
    }
    serde_json::from_value(env.data).map_err(|e| format!("decode_exec_resp: {e}"))
}

#[cfg(not(target_arch = "wasm32"))]
#[allow(dead_code)]
fn run_powershell(_script: &str, _timeout_ms: u32) -> Result<ExecResponse, String> {
    Err("not available on host build".to_string())
}

// ============================================================
// Pure-function unit tests
// ============================================================
#[cfg(test)]
mod tests {
    use super::*;

    fn sample_task(name: &str, path: &str, state: &str) -> ScheduledTask {
        ScheduledTask {
            task_name: name.to_string(),
            task_path: path.to_string(),
            state: state.to_string(),
            ..Default::default()
        }
    }

    // ---- parse_powershell_output ----

    #[test]
    fn parse_array_output() {
        let json = r#"[
            {"taskName":"A","taskPath":"\\","state":"Ready","triggers":[],"actions":[]},
            {"taskName":"B","taskPath":"\\Microsoft\\","state":"Disabled","triggers":[],"actions":[]}
        ]"#;
        let tasks = parse_powershell_output(json);
        assert_eq!(tasks.len(), 2);
        assert_eq!(tasks[0].task_name, "A");
        assert_eq!(tasks[1].state, "Disabled");
    }

    #[test]
    fn parse_single_object_output() {
        let json = r#"{"taskName":"Only","taskPath":"\\","state":"Ready"}"#;
        let tasks = parse_powershell_output(json);
        assert_eq!(tasks.len(), 1);
        assert_eq!(tasks[0].task_name, "Only");
    }

    #[test]
    fn parse_empty_output() {
        assert!(parse_powershell_output("").is_empty());
        assert!(parse_powershell_output("null").is_empty());
        assert!(parse_powershell_output("   \n").is_empty());
    }

    #[test]
    fn parse_with_actions_and_triggers() {
        let json = r#"[
            {"taskName":"Backup","taskPath":"\\","state":"Ready",
             "triggers":["Daily at 03:00","AtStartup"],
             "actions":[{"execute":"powershell.exe","arguments":"-File c:\\backup.ps1","workingDir":"c:\\"}],
             "lastRunUnix": 1700000000, "nextRunUnix": 1700086400}
        ]"#;
        let tasks = parse_powershell_output(json);
        assert_eq!(tasks.len(), 1);
        assert_eq!(tasks[0].triggers.len(), 2);
        assert_eq!(tasks[0].actions.len(), 1);
        assert_eq!(tasks[0].actions[0].execute, "powershell.exe");
        assert_eq!(tasks[0].last_run_unix, 1700000000);
        assert_eq!(tasks[0].next_run_unix, 1700086400);
    }

    // ---- filter_tasks ----

    #[test]
    fn filter_skips_disabled_by_default() {
        let tasks = vec![
            sample_task("A", "\\", "Ready"),
            sample_task("B", "\\", "Disabled"),
        ];
        let req = ListRequest::default();
        let out = filter_tasks(tasks, &req);
        assert_eq!(out.len(), 1);
        assert_eq!(out[0].task_name, "A");
    }

    #[test]
    fn filter_includes_disabled_when_requested() {
        let tasks = vec![
            sample_task("A", "\\", "Ready"),
            sample_task("B", "\\", "Disabled"),
        ];
        let req = ListRequest {
            include_disabled: true,
            ..Default::default()
        };
        assert_eq!(filter_tasks(tasks, &req).len(), 2);
    }

    #[test]
    fn filter_by_name_substring() {
        let tasks = vec![
            sample_task("Backup", "\\", "Ready"),
            sample_task("UpdateOrchestrator", "\\", "Ready"),
            sample_task("BackupVerify", "\\", "Ready"),
        ];
        let req = ListRequest {
            filter: "backup".to_string(),
            ..Default::default()
        };
        let out = filter_tasks(tasks, &req);
        assert_eq!(out.len(), 2);
        assert!(out.iter().all(|t| t.task_name.to_lowercase().contains("backup")));
    }

    #[test]
    fn filter_by_path_prefix() {
        let tasks = vec![
            sample_task("A", "\\Microsoft\\Windows\\Defender\\", "Ready"),
            sample_task("B", "\\Custom\\", "Ready"),
        ];
        let req = ListRequest {
            path_prefix: "\\microsoft".to_string(),
            ..Default::default()
        };
        let out = filter_tasks(tasks, &req);
        assert_eq!(out.len(), 1);
        assert_eq!(out[0].task_name, "A");
    }
}
