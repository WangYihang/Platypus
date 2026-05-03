# Streaming plugin ABI — design

> **Status:** design draft. No code under `internal/agent/plugin/`
> implements this yet; tracked under iterations C8–C10. This document
> is the contract the future PR will be written against.

## Why

The current plugin ABI is request/response: one `PluginCallRequest`
goes in, one `PluginCallResponse` comes out. That fits SysInfo,
ProcessList, Mkdir — short, bounded RPCs. It does NOT fit:

- **PTY sessions** (`STREAM_TYPE_PROCESS_OPEN`): bidirectional byte
  stream from operator's terminal to a child process on the agent
  host. Lives for minutes-to-hours; reads/writes are interleaved at
  byte granularity.
- **File streaming** (`STREAM_TYPE_FILE_READ` / `_WRITE` / `_SCAN` /
  `_ARCHIVE`): server pulls / pushes / walks files on the agent.
  Multi-MB chunks, often many of them per call.
- **Tunnel proxy** (`STREAM_TYPE_TUNNEL_PULL`): full TCP byte stream
  proxied through the agent. Behaves like a socket.

Plugin-ifying these without a streaming ABI would require either
buffering full transfers in memory (impractical for 10 GB archive
walks) or splitting one logical operation into many request/response
pairs (latency death for an interactive PTY).

## Requirements

A streaming plugin ABI must:

1. Let the plugin **read** unbounded input (operator's keystrokes,
   incoming TCP, source file bytes) without buffering it all.
2. Let the plugin **write** unbounded output (PTY echo, file bytes,
   socket data).
3. Honor the same **capability** + **resource limit** model as
   one-shot plugins. A streaming plugin still gets a memory cap; a
   per-call invocation deadline becomes a per-stream lifetime
   ceiling (configurable, may be infinite for PTY).
4. Survive operator-initiated cancellation cleanly (Ctrl-C,
   browser tab closed). Wasm-side `host_stream_read` must surface a
   `Cancelled` error, not block forever.
5. Fit alongside the existing one-shot ABI without forcing every
   plugin to choose a side. A single plugin can export both
   `(input) -> output` methods and stream handlers.

## Design

### Manifest declaration

`plugin.yaml` gains a streaming arm next to the existing `rpc:`:

```yaml
rpc:
  - name: scan
    request:  { proto: ScanRequest }
    response: { proto: ScanResponse }
streams:
  - name: shell
    type: bidirectional
    request:  { proto: ProcessOpenRequest }
  - name: file_read
    type: server_to_client
    request:  { proto: FileReadRequest }
```

`type` is one of:

| Value                | Direction                                     | Used by                  |
|----------------------|-----------------------------------------------|--------------------------|
| `bidirectional`      | host ⇄ plugin (interleaved, byte-oriented)    | PTY, tunnel              |
| `server_to_client`   | host → plugin → wire (plugin produces bytes)  | file_read, file_archive  |
| `client_to_server`   | wire → plugin → host (plugin consumes bytes)  | file_write, file_scan    |

### Wire protocol

A new `STREAM_TYPE_PLUGIN_STREAM = 13` is added to `common.proto`.
Header metadata is `PluginStreamRequest{plugin_id, stream_name,
payload bytes}` — `payload` carries the original request proto
(e.g. `ProcessOpenRequest`), so the agent doesn't need a per-stream
oneof arm.

After `StreamAccept`, byte frames flow as
`PluginStreamFrame{kind: DATA|EOF|ERROR, data bytes, error string}`.
The agent forwards these bytes verbatim across the wasm <-> wire
boundary; framing is the agent's responsibility, content is opaque
to the runtime.

### Host functions

Three new host functions, namespaced under `platypus`:

| Host fn               | Capability        | Behaviour                                                               |
|-----------------------|-------------------|-------------------------------------------------------------------------|
| `host_stream_read`    | (per-stream)      | Block for up to N bytes from the inbound side. Returns 0 on EOF.        |
| `host_stream_write`   | (per-stream)      | Send N bytes to the outbound side. May partial-write under backpressure. |
| `host_stream_close`   | (per-stream)      | Signal EOF on the outbound side. Subsequent writes return Cancelled.    |

"per-stream capability" means the operator's grant for the stream
type maps to the host function set:

- `cap.pty`:    grants both read + write + close on bidirectional
  streams. Plus a child-process command allowlist mirroring `exec`.
- `cap.fs.write`: grants write + close on `client_to_server` file
  streams. Plus a path allowlist (similar to `fs.read`).
- `cap.tunnel`: grants both directions on bidirectional streams. Plus
  a remote-address allowlist (host:port pairs).

### Plugin contract (Rust PDK shape)

```rust
#[plugin_stream(name = "shell", type = "bidirectional")]
pub fn shell_stream(req: ProcessOpenRequest, ctx: StreamCtx) -> StreamResult<()> {
    // Spawn the child via host_exec.
    let child = host::exec_pipe(&req.command, &req.args)?;

    // Reader: bytes from the operator's terminal -> child stdin.
    let to_child = ctx.fork(|reader| {
        let mut buf = [0u8; 4096];
        loop {
            let n = reader.read(&mut buf)?;
            if n == 0 { break; }
            child.stdin.write_all(&buf[..n])?;
        }
        Ok(())
    });

    // Writer: child stdout/stderr -> operator's terminal.
    let from_child = ctx.fork(|writer| {
        let mut buf = [0u8; 4096];
        loop {
            let n = child.stdout.read(&mut buf)?;
            if n == 0 { break; }
            writer.write_all(&buf[..n])?;
        }
        Ok(())
    });

    to_child.join()?;
    from_child.join()?;
    Ok(())
}
```

`StreamCtx::fork` schedules a wasm function on a separate fuel
budget so reads + writes can interleave. Single-threaded wasm with
cooperative yields under the hood (extism / wazero's host-fn
boundary is the natural yield point).

### Agent-side bridge

Each `STREAM_TYPE_PLUGIN_STREAM` arrival:

1. Looks up the plugin by `plugin_id` + `stream_name`.
2. Verifies the manifest declares the stream and the operator
   granted the matching capability.
3. Constructs a `pluginStreamCtx` with two byte channels (`incoming`,
   `outgoing`), per-stream deadline, and the granted capability set.
4. Invokes the wasm export — `host_stream_read` and
   `host_stream_write` close over the channels.
5. Bridges the channels to the wire: a goroutine reads
   `PluginStreamFrame` off the wire and pushes bytes onto
   `incoming`; another reads from `outgoing` and writes
   `PluginStreamFrame` to the wire.
6. On wasm return / error / context cancel, drains channels,
   writes a terminal `PluginStreamFrame{kind=EOF}` or
   `kind=ERROR`, and closes the wire stream.

### Resource limits

Plugin authors declare per-stream limits in the manifest:

```yaml
streams:
  - name: shell
    type: bidirectional
    request: { proto: ProcessOpenRequest }
    resources:
      max_lifetime_ms: 0          # 0 = unbounded (PTY default)
      max_bytes_in:  104857600    # 100 MiB ceiling on inbound
      max_bytes_out: 104857600
```

The agent enforces:

- `max_lifetime_ms` via the parent context's deadline.
- `max_bytes_in` / `max_bytes_out` by counting bytes through the
  bridge; exceeding either closes the stream with
  `ERROR{kind="resource_exhausted"}`.

### Cancellation semantics

- Operator closes terminal tab → server cancels the wire stream →
  agent context cancels → `host_stream_read` returns `Cancelled`,
  `host_stream_write` returns `Cancelled`, the wasm function should
  return.
- Plugin returns from the function → agent drains pending writes,
  emits `EOF`, closes the wire stream.
- Network drop → wire stream errors, agent detects, cancels context,
  same as operator cancel from the plugin's POV.

### What does NOT change

- The one-shot ABI (`STREAM_TYPE_RPC` + `PluginCallRequest`) stays
  exactly as it is. Plugins can mix one-shot methods + streams in
  the same manifest.
- The signature / install / capability / catalog flow is identical;
  streaming is purely a runtime + manifest extension.
- The system plugin bootstrap auto-installs streaming plugins the
  same as one-shot ones.

## Open questions

1. **Backpressure granularity.** Should `host_stream_write` return
   the number of bytes accepted (POSIX-style) so the plugin can
   handle partial writes, or always block until full delivery? Start
   with always-full; add partial-write only if a real plugin needs
   it.

2. **Per-stream context propagation.** PTY exec needs the operator's
   credentials forwarded to the child (sudo prompts, etc.). Do
   credentials flow through `ProcessOpenRequest` as today, or via a
   dedicated `host_stream_context()` call? Vote: keep in the request
   proto for symmetry with the wire format.

3. **Fork primitive.** `StreamCtx::fork` would need either a real
   wasm thread (component-model proposal still draft) or
   cooperative fibers managed by the PDK. For MVP, force the plugin
   to write straight-line code that reads then writes alternately —
   workable for non-PTY streams, painful for PTY. Revisit when the
   wasm threads proposal stabilises.

4. **Memory limits per stream.** A single bidirectional stream that
   buffers a 100 MiB inbound burst could exhaust the plugin's
   `max_memory_mb`. Does the agent push back with TCP-style
   windowing, or trust the plugin to drain promptly? Lean toward
   windowing in the bridge (default 1 MiB high-water mark) so a
   slow plugin doesn't OOM.

## Phased rollout (for Iter C8–C10)

1. **C8a: protocol.** Add `STREAM_TYPE_PLUGIN_STREAM` +
   `PluginStreamRequest` / `PluginStreamFrame` to `common.proto` /
   `plugin.proto`. Regenerate. Tests for proto encode/decode only.
2. **C8b: agent-side bridge.** New `internal/agent/plugin/stream/`
   subpackage with the channels + bridge + capability check + limit
   enforcement. Stubbed wasm side; tests use a Go fake plugin.
3. **C8c: host functions.** Wire `host_stream_read` / `_write` /
   `_close` into the existing host_funcs registration path.
4. **C8d: file_read migration.** First real streaming plugin: replace
   the FileRead built-in handler. Includes a Rust example PDK macro
   for the streaming shape.
5. **C9: PTY migration.** Bidirectional case. Probably needs the
   StreamCtx::fork primitive or interim straight-line shape.
6. **C10: tunnel migration.** Same shape as PTY but with a
   net.Dial-flavored capability.

After C10, `internal/agent/serve_link.go` stops dispatching
`STREAM_TYPE_PROCESS_OPEN` / `STREAM_TYPE_TUNNEL_PULL` /
`STREAM_TYPE_FILE_*` to built-in handlers — they all flow through
`STREAM_TYPE_PLUGIN_STREAM` to the migrated system plugins. C11 then
deletes the old dispatch arms.

## Migration status (2026-05-03)

The streaming ABI shipped (slice 1-5 in `wasm_*.go`) and four of
the six legacy stream handlers have a wasm reference plugin proven
to work end-to-end via integration tests:

| Stream type | Plugin | Status |
|---|---|---|
| `PLUGIN_STREAM` (echo demo) | `example/plugins/echo-stream` | Shipping. Pump-mode dispatch. |
| `FILE_READ` | `example/plugins/sys-file-read` | Plugin + e2e test ready. Cutover blocked on system signing key. |
| `FILE_SCAN` | `example/plugins/sys-file-scan` | Plugin + e2e test ready. Cutover blocked on system signing key. |
| `FILE_WRITE` | `example/plugins/sys-file-write` | Plugin + e2e test ready. Cutover blocked on system signing key. |
| `FILE_ARCHIVE` | not started | Needs Rust tar/zip/gzip deps; ~2-4 hours impl. |
| `PROCESS_OPEN` | not started | Needs new `host_process_*` host fn family for streaming exec; ~6-8 hours impl. |
| `TUNNEL_PULL` | not started | Needs new `host_net_dial` capability; security review on which destinations a plugin may dial. ~4-6 hours impl + design. |

The shipped infrastructure (`internal/agent/plugin/wasm_legacy_dispatch.go`,
`host_link_read_frame` / `host_link_write_frame` /
`host_fs_read_range` / `host_fs_write_range`) is reusable for the
remaining three; each just needs the per-protocol Rust plugin and
its proto encoder/decoder.

### Cutover (per-plugin)

The legacy Go handler for each migrated stream type still serves
production traffic until:

1. The plugin's `.wasm` is signed with `PLATYPUS_SYSTEM_KEY` (kept
   out-of-repo) and staged under
   `internal/agent/plugin/system/embedded/<id>/<version>/`.
2. The matching entry is removed from
   `example/plugins/sys-streams/plugin.yaml`'s `streams:` list, and
   sys-streams is rebuilt + re-signed (`make sign-system-plugins`).
3. `internal/agent/<type>_stream.go` and the matching adapter line
   in `cmd/platypus-agent/stream_adapters.go` are deleted.

Until then the legacy handler keeps serving — the wasm replacement
sits in `example/plugins/` as proof the migration mechanically
works, with the integration test under
`internal/agent/plugin/<type>_integration_test.go` as the canonical
end-to-end coverage.

## See also

- [AUTHORS.md](AUTHORS.md) — current one-shot ABI for plugin authors
- [SECURITY.md](SECURITY.md) — capability model the streaming caps
  extend
- `internal/agent/serve_link.go` — current built-in stream dispatch
  to be migrated
