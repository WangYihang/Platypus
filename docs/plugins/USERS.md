# Installing + managing Platypus plugins

This guide is for **operators** — Platypus admins who install plugins
onto their fleet's agents. Plugin authoring is in
[AUTHORS.md](AUTHORS.md); the security model is in
[SECURITY.md](SECURITY.md).

## What plugins do

A plugin extends one specific agent's RPC surface. Once installed on
agent A, the server can call the plugin's exported methods on A;
calls to agent B that doesn't have it return `plugin_not_installed`.

Each plugin runs in a WebAssembly sandbox, can only do what its
manifest declared (and what you confirmed at install time), and
counts every call against a fuel + memory budget defined by the
plugin author. A misbehaving plugin can't break out — worst case it
returns errors or burns its own per-call deadline.

## Trusting a publisher

Before installing any plugin from publisher X, you must drop X's
public key under each agent's
`~/.platypus/agent/plugins/publishers/<keyid>.pub`. Plugin signatures
are verified against this directory at install time and on every
agent reboot; an unsigned-or-untrusted plugin is **quarantined**, not
loaded.

Public keys ship through whatever channel you trust — author's
website, Slack message, signed email. They're 32 bytes of Ed25519
under one comment line; impossible to confuse with a malicious
forgery as long as the channel itself wasn't compromised.

## Installing on one agent

Two paths — REST (suitable for scripting) and Desktop UI
(forthcoming, click-through).

### REST

The endpoint is
`POST /api/v1/projects/:pid/agents/:agent_id/plugins`. Body shape
(all three artefact bytes are base64):

```json
{
  "plugin_id": "com.acme.disk-monitor",
  "version": "1.4.2",
  "publisher_pubkey": "untrusted comment: ...\n<base64>\n",
  "manifest_b64": "<base64 of plugin.yaml>",
  "wasm_b64": "<base64 of disk_monitor.wasm>",
  "signature_b64": "<base64 of disk_monitor.wasm.minisig>",
  "granted_capabilities": ["fs.read", "net.http"]
}
```

The server pushes the bytes through a `STREAM_TYPE_PLUGIN_MGMT`
stream to the agent. The agent walks the install pipeline and
streams progress back; the response body is the full progression:

```json
{
  "status": "installed",
  "plugin_id": "com.acme.disk-monitor",
  "version": "1.4.2",
  "progress": [
    { "phase": "PHASE_RECEIVE" },
    { "phase": "PHASE_VERIFY_SHA", "bytes_done": 81920, "bytes_total": 81920 },
    { "phase": "PHASE_VERIFY_SIG" },
    { "phase": "PHASE_EXTRACT" },
    { "phase": "PHASE_LOAD" },
    { "phase": "PHASE_INSTALLED" }
  ]
}
```

`status` is one of `installed` / `failed` / `in_progress`. Failed
installs include a terminal frame with `error_code` +
`error_message` (e.g. `signature_mismatch`, `manifest_invalid`,
`capability_overgrant`).

### Operator-side curl helper

```sh
curl -X POST https://platypus.example/api/v1/projects/proj-abc/agents/agent-123/plugins \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d @<(jq -n \
    --arg pid "com.acme.disk-monitor" \
    --arg ver "1.4.2" \
    --arg pub "$(cat me.pub)" \
    --arg man "$(base64 -w0 plugin.yaml)" \
    --arg wasm "$(base64 -w0 disk_monitor.wasm)" \
    --arg sig "$(base64 -w0 disk_monitor.wasm.minisig)" \
    '{ plugin_id: $pid, version: $ver, publisher_pubkey: $pub,
       manifest_b64: $man, wasm_b64: $wasm, signature_b64: $sig,
       granted_capabilities: ["fs.read", "net.http"] }')
```

### Capability confirmation

`granted_capabilities` is the **enforced** capability set, not the
manifest's request. A plugin manifest that asks for
`["fs.read", "net.http", "exec"]` but is installed with
`granted_capabilities: ["fs.read"]` will:

- Successfully install (no overgrant — granted ⊆ requested).
- Have `host_fs_*` calls succeed (within the path allowlist).
- Have `host_http` and `host_exec` return `capability_denied`.

This is the correct posture when you trust an author's intentions
but want belt-and-braces: grant the minimum the plugin actually
needs to work; widen later if reports of broken functionality come
in.

The agent rejects an install whose `granted_capabilities` includes
an entry the manifest doesn't request (`capability_overgrant`) — you
can't grant more than the plugin asked for, only less.

## Listing what's installed

```
GET /api/v1/projects/:pid/agents/:agent_id/plugins
```

Returns

```json
{
  "plugins": [
    {
      "id": "com.acme.disk-monitor",
      "name": "Disk Monitor",
      "version": "1.4.2",
      "author": "Acme Corp",
      "enabled": true,
      "granted_capabilities": ["fs.read", "net.http"],
      "install_unix": 1714737600,
      "publisher_key_id": "ABCDEF1234567890"
    }
  ]
}
```

System plugins (those baked into the agent build) appear in the same
list but are tagged differently in the desktop UI; they refuse to
uninstall.

## Disable / re-enable

```
PATCH /api/v1/projects/:pid/agents/:agent_id/plugins/com.acme.disk-monitor
Body: { "enabled": false }
```

A disabled plugin stays loaded (cheap) but every `Invoke` call
returns `plugin_disabled` without entering the wasm runtime. Use
this to triage a runaway plugin without uninstalling it (so you
keep its log buffer + state for forensics).

## Uninstall

```
DELETE /api/v1/projects/:pid/agents/:agent_id/plugins/com.acme.disk-monitor
Body (optional): { "purge_state": true }
```

Removes the plugin from the catalog, closes its wasm runtime, and
deletes its install directory. `purge_state: true` also wipes the
plugin's `state/` directory; default is to preserve state across
uninstall+reinstall (subject to the documented Phase 2 limitation —
state currently lives inside the install dir so it's removed either
way; `purge_state` is wired through for forward compatibility).

System plugins return HTTP 400 + `plugin: cannot uninstall system
plugin` — the bundled bootstrap would reinstall them on the next
agent boot anyway.

## Reading plugin logs

```
GET /api/v1/projects/:pid/agents/:agent_id/plugins/com.acme.disk-monitor/logs?tail=100
```

Returns the most recent N entries from the plugin's in-memory log
ring (every `host_log` call from the plugin lands here). `tail=0` or
unset returns everything currently buffered (capped agent-side).

```json
{
  "entries": [
    {
      "unix_nano": 1714737612345678901,
      "level": "info",
      "message": "scanning /proc/mounts",
      "correlation_id": "rpc-abc-1"
    }
  ]
}
```

`correlation_id` matches the inbound RPC that produced the log line,
which lets you trace a single operator action across the activity
log + per-plugin log buffer.

## Auditing

Every plugin install / uninstall / enable / invoke is recorded in the
agent's structured log with `plugin_id`, `granted_capabilities`,
`fuel_used`, `mem_peak_bytes`, `correlation_id`, and the operator
who triggered the action (`actor`). The server's activity log mirrors
the install / uninstall / enable rows for fleet-wide forensics.

## Troubleshooting

| Symptom                                          | Likely cause                                                                                       |
|--------------------------------------------------|----------------------------------------------------------------------------------------------------|
| `signature_mismatch` on install                  | Publisher key on the agent doesn't match the .minisig — re-distribute the .pub file               |
| `capability_overgrant` on install                | `granted_capabilities` includes something the manifest doesn't request — narrow the list           |
| `manifest_id_mismatch` on install                | The id in `plugin.yaml` doesn't match the install request's `plugin_id` field                      |
| `plugin_not_installed` on call                   | Plugin isn't on this agent — install it first                                                     |
| `plugin_disabled` on call                        | Someone PATCH'd `enabled: false`                                                                   |
| `method_not_declared` on call                    | The method isn't in the manifest's `rpc:` list — add it + bump version + reinstall                 |
| `capability_denied: fs.read` from a plugin       | Either the operator denied `fs.read` at install OR the path isn't under the manifest's allowlist  |

## See also

- [AUTHORS.md](AUTHORS.md) — building plugins yourself
- [SECURITY.md](SECURITY.md) — the trust model + sandbox guarantees
