# Changelog

Notable, breaking, or operator-facing changes.  Grouped by sprint
phase tag (e.g. M0, I3a) so commits are easy to locate.

## Unreleased

### Breaking changes

- **M0** Plugin id `com.platypus.sys-procs` renamed to
  `com.platypus.sys-procs-linux`.  The Go variant
  `com.platypus.sys-procs-go` renamed to
  `com.platypus.sys-procs-linux-go`.  The plugin's RPC and
  capabilities are unchanged; only the id moved to align with the
  per-OS naming convention now used across `sys-disk-linux`,
  `sys-systemd-linux`, `sys-journald-linux`.

  Operators with `baseline_plugin_ids` referencing the old id
  must update their host config / install tokens to the new id;
  an unmigrated reference will surface as `plugin_not_installed`
  on the next reconcile.  No compat shim is installed.

  Sibling plugins `sys-procs-darwin` and `sys-procs-windows` ship
  in the M1a / M1b commits later in Sprint 2.

- **I3a** Plugin id `com.platypus.sys-tunnel-pull` (and its Go
  counterpart `com.platypus.sys-tunnel-pull-go`) deleted; the TCP
  forwarding role is now owned by `com.platypus.sys-tunnel-tcp`
  with the same wire stream type (`STREAM_TYPE_TUNNEL_PULL`).
  Operators using the old id in their baseline must update.

### New plugins (Sprint 1)

- `com.platypus.sys-systemd-linux` — three RPCs over `systemctl`
- `com.platypus.sys-journald-linux` — `query` over `journalctl -o json`
- `com.platypus.sys-tunnel-tcp` — replaces `sys-tunnel-pull`
- `com.platypus.sys-disk-linux` — filesystem usage via `df -B1 -T -P`

### New REST endpoints (Sprint 1, K-phase)

- `POST /api/v1/projects/:pid/agents/bulk/plugin_call`
- `POST /api/v1/projects/:pid/agents/bulk/exec`
- `POST /api/v1/projects/:pid/agents/bulk/sys_info`
