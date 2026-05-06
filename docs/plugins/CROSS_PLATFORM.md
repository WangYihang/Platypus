# Cross-platform plugin matrix

Sprint 2 (M-phase) closed the cross-platform parity gap for
host-introspection RPCs. Every operator UI surface that worked
on Linux now has a per-OS sibling.

## Naming convention

OS-specific plugins use a `-<os>` suffix on the plugin id:

```
com.platypus.sys-procs-linux
com.platypus.sys-procs-darwin
com.platypus.sys-procs-windows
```

Multi-platform plugins (whose Rust source compiles to a single
.wasm that runs anywhere via `host_uname` branching) keep the
unsuffixed id:

```
com.platypus.sys-info        (linux + darwin + windows)
com.platypus.sys-files-read  (multi-platform)
com.platypus.sys-files-write (multi-platform)
com.platypus.sys-process     (multi-platform PTY)
com.platypus.sys-security    (multi-platform host hardening)
com.platypus.sys-config-audit (multi-platform secret-scanning)
```

The reconciler (`internal/api/plugin_sync.go:platformMatches`)
filters per-OS plugins against the agent's reported runtime.GOOS
on every reconnect. Operators add OS-specific ids to their
`baseline_plugin_ids`; the reconciler installs only the matching
ones per host.

## RPC parity matrix

Each row is one operator-visible RPC. Cells show which plugin
provides it on each OS, and the underlying tool / file the
plugin reads from.

| RPC | Linux | macOS | Windows |
|---|---|---|---|
| **Process list** (`process_list`) | `sys-procs-linux` /proc | `sys-procs-darwin` `/bin/ps -axwwo` | `sys-procs-windows` Get-CimInstance Win32_Process |
| **Filesystem usage** (`list_filesystems`) | `sys-disk-linux` `df -B1 -T -P` | `sys-disk-darwin` `df -k -P` | `sys-disk-windows` Get-PSDrive |
| **Network listeners** (`list_listeners`) | `sys-net-linux` /proc/net/tcp | `sys-net-darwin` `netstat -anv -p tcp` | `sys-net-windows` Get-NetTCPConnection |
| **Network connections** (`list_connections`) | `sys-net-linux` /proc/net/tcp | `sys-net-darwin` `netstat -anv -p tcp` | `sys-net-windows` Get-NetTCPConnection |
| **Service list / control** (`list_units` / `unit_action`) | `sys-systemd-linux` `systemctl` | `sys-services-darwin` `/bin/launchctl` | `sys-services-windows` Get-Service / Start-Service / Stop-Service / Restart-Service / Suspend-Service / Resume-Service |
| **Package list** (`list_installed`) | `sys-pkg-linux` (apt/dnf/yum/zypper/pacman auto-detect) | `sys-pkg-darwin` `brew list --versions` | `sys-pkg-windows` Get-Package |
| **Package upgrade list** (`list_upgradable`) | `sys-pkg-linux` (per-backend) | `sys-pkg-darwin` `brew outdated --json=v2` | `sys-pkg-windows` `winget upgrade` (winget-only; missing → error="winget_not_installed") |
| **Logs** (`query`) | `sys-journald-linux` `journalctl -o json` | (deferred — `log show --predicate` is verbose; v2) | (deferred — Get-WinEvent; v2) |
| **TCP forwarding** (`pull` stream) | `sys-tunnel-tcp` (multi-platform via host_net_dial / host_net_relay) | same | same |

## Bulk RPC interaction

The bulk-RPC endpoints (Sprint 1's K-phase) don't auto-route by
OS. An operator running

```
POST /api/v1/projects/:pid/agents/bulk/exec
{"command": "systemctl status"}
```

against a fleet that mixes Linux + macOS will see the darwin
agents return per-row `exit_code != 0` with "command not found"
in stderr. The bulk endpoint reports these as per-row failures
correctly; the operator UI surfaces them.

For OS-aware fleet operations, prefer the typed
`/bulk/plugin_call` endpoint with the per-OS plugin id:

```
POST /api/v1/projects/:pid/agents/bulk/plugin_call
{
  "agent_ids": ["...windows-host..."],
  "plugin_id": "com.platypus.sys-services-windows",
  "method": "list_units",
  "payload": {}
}
```

A future operator UI may auto-pick the per-OS plugin from a
unified "list services on all hosts" button by partitioning the
agent_ids by their host.OS column.

## Test coverage caveat

CI mostly runs Linux. Per-OS integration tests skip on the
wrong runtime.GOOS (existing pattern in
`sys_systemd_linux_integration_test.go`). The Rust parser
unit tests run on the host build target and cover the bulk of
the format-handling logic; coverage of the actual subprocess
round-trip on darwin / windows depends on operators running
`go test ./internal/agent/plugin/` on those platforms or a
future GitHub Actions matrix.

The `Manifest` integration tests work everywhere (they only
read the staged plugin.yaml + .wasm and verify parsing) — so
each per-OS plugin still has a guaranteed-Linux-CI safety net.

## Adding a new cross-platform plugin

1. Pick whether the plugin is OS-specific or multi-platform.
   Multi-platform is preferred when `host_uname` branching
   keeps the source readable; OS-specific is preferred when
   the implementations diverge sharply (different tool,
   different output format).
2. For OS-specific:
   - Use `-<os>` suffix on the plugin id.
   - Set `runtime.os_targets: [<os>]` in plugin.yaml.
   - Manifest's `capabilities.exec.commands` may include
     windows-style backslash paths; the validator's
     `isAbsCrossPlatform` accepts them on any host (M1b).
3. Mirror `sys-pkg-linux` for any multi-backend dispatch
   pattern (probe via `command -v`, dispatch into per-backend
   parser, surface the detected backend in every response).
4. Mirror `sys-services-windows`'s injection-safe script
   builder pattern when constructing PowerShell strings:
   validate the user-supplied name against a closed allowlist
   of safe characters before string interpolation.

See `docs/plugins/AUTHORS.md` for the broader plugin-authoring
guide. M5a (`sys-pkg-linux/src/lib.rs`) is the exemplar for
multi-backend dispatch; M4a (`sys-services-darwin/src/lib.rs`)
is the exemplar for `host_exec` shell-out + tab-delimited
column parser; M3a (`sys-net-linux/src/lib.rs`) is the
exemplar for `host_fs_read` + structured-text parsing.
