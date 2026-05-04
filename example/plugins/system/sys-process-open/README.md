# sys-process-open

System plugin replacing the legacy Go `HandleProcessStream`. Claims
`STREAM_TYPE_PROCESS_OPEN` via the `wasm:open` host_handler marker.

The plugin is intentionally thin: it parses the `ProcessOpenRequest`,
applies policy (capability check on the command via the manifest's
`process.commands` allowlist; an operator-supplied replacement could
add per-fleet allowlists, audit hooks, etc.), then hands off to the
host's `host_process_spawn` + `host_process_relay` family. The host
runs the actual bidirectional pump between the operator's wire and
the child PTY — that infrastructure code is unchanged from the
legacy handler.

The split is deliberate:
- **Spawn policy** (which command, which args, capability check) is
  in user-replaceable wasm.
- **Byte pumping** (server-wire ↔ child PTY/pipes, single-goroutine
  per direction, exit reaping) stays in Go because wasm is
  single-threaded and the pump needs concurrent reads/writes.

## PTY support

Both PTY and non-PTY modes work. The wasm plugin sets `pty: true` in
the spawn spec when the request has it; `host_process_spawn` then
calls `pty.StartWithSize` instead of `cmd.Start`. The bidirectional
pump in `host_process_relay` handles both — `relayPTY` for PTY,
`relayPipes` for non-PTY.

Window resize frames (`ProcessFrame.resize`) are forwarded through
the host's relay pump; the wasm doesn't see them.

## Cutover sequence

Same shape as the other sys-* plugins. See
`example/plugins/sys-file-read/README.md` for the canonical
version. Briefly:

1. Build + sign the .wasm with the system signing key.
2. Stage under `internal/agent/plugin/system/embedded/com.platypus.sys-process-open/1.0.0/`.
3. Remove the `process_open` entry from `example/plugins/sys-streams/plugin.yaml`'s `streams:` list, rebuild + re-sign sys-streams.
4. Delete `internal/agent/process_stream.go` + the `process_open`
   adapter line in `cmd/platypus-agent/stream_adapters.go`.
5. `go test ./internal/agent/...` — `process_open_integration_test.go`
   is the canonical end-to-end coverage (happy path + allowlist
   denial path).

## Capability requirements

`process` with `commands: ["*"]` for the system bundle (matches the
legacy handler's implicit any-command authority). Third-party
replacements should narrow this to a literal list — the install
dialog flags `*` prominently as the unrestricted-spawn marker.
