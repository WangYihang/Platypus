# sys-tunnel-pull

System plugin replacing the legacy Go `HandleTunnelPullStream`.
Claims `STREAM_TYPE_TUNNEL_PULL` via the `wasm:pull` host_handler
marker.

The plugin is thin by design: it parses the `TunnelPullRequest`,
applies dial policy (capability check on the target via the
manifest's `net.dial.targets` allowlist; an operator-supplied
replacement could narrow further or audit-log every dial), then hands
off to the host's `host_net_dial` + `host_net_relay` family. The host
does the actual TCP dial + bidirectional byte splice between the
operator's wire and the dialed conn.

The split is the same shape sys-process-open uses:

- **Dial policy** (which target, with what timeout, capability
  allowlist enforcement) lives in user-replaceable wasm.
- **Byte splicing** (raw `io.Copy` both ways, close-on-EOF) stays in
  Go because wasm is single-threaded and the splice needs concurrent
  reads/writes on the wire and the dialed conn.

## Security note

`net.dial` is the most powerful capability in the manifest set. With
`targets: ["*"]` (the system bundle's grant) the plugin has effective
SSRF authority over the agent's network. Third-party replacements
should narrow `targets` to a literal list — the install dialog flags
`*` as a high-risk wildcard, similar to how `process.commands: ["*"]`
is flagged.

## Cutover sequence

Same shape as the other sys-* plugins. See
`example/plugins/sys-file-read/README.md` for the canonical version.
Briefly:

1. Build + sign the .wasm with the system signing key.
2. Stage under `internal/agent/plugin/system/embedded/com.platypus.sys-tunnel-pull/1.0.0/`.
3. Remove the `tunnel_pull` entry from `example/plugins/sys-streams/plugin.yaml`'s `streams:` list, rebuild + re-sign sys-streams.
4. Delete `internal/agent/tunnel_pull_stream.go` + the `tunnel_pull`
   adapter line in `cmd/platypus-agent/stream_adapters.go`.
5. `go test ./internal/agent/...` — `tunnel_pull_integration_test.go`
   is the canonical end-to-end coverage (echo round-trip + allowlist
   denial path).

## Capability requirements

`net.dial` with `targets: ["*"]` for the system bundle (matches the
legacy handler's implicit any-target authority).
