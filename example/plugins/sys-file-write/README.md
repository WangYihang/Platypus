# sys-file-write

System plugin replacing the legacy Go `HandleFileWriteStream`. Claims
`STREAM_TYPE_FILE_WRITE` via the `wasm:write` host_handler marker.

The plugin is the first in the bundle to use the bidirectional
legacy-wasm bridge: it consumes incoming `FileChunk` frames via
`host_link_read_frame`, writes each chunk to disk via
`host_fs_write_range`, and emits the same `FileWriteResponse` ack +
`FileWriteResult` trailer the legacy handler did. Wire format is
byte-for-byte identical so server-side uploaders don't change.

## Cutover sequence

Same shape as the other sys-file-* plugins (see `sys-file-read/README.md`
for the canonical version). Briefly:

1. Build + sign the .wasm with the system signing key.
2. Stage under `internal/agent/plugin/system/embedded/com.platypus.sys-file-write/1.0.0/`.
3. Remove the `file_write` entry from `example/plugins/sys-streams/plugin.yaml`'s `streams:` list, rebuild + re-sign sys-streams.
4. Delete `internal/agent/file_write_stream.go` + the file_write
   adapter line in `cmd/platypus-agent/stream_adapters.go`.
5. `go test ./internal/agent/...` — `file_write_integration_test.go`
   is the canonical e2e coverage.

## Capability requirements

`fs.write` with `paths: ["/"]` — same posture as the legacy
in-process handler.

## Append vs truncate

This plugin currently treats `append=true` as "start writing at
offset 0 without truncating", letting the OS extend the file via
seek-then-write. The legacy handler used `O_APPEND` which atomically
seeks to end-of-file on every write — different semantics for
concurrent writers. For the typical "single uploader streams a fresh
file" case both produce identical results; for concurrent appenders
add a dedicated `host_fs_write_append(path, data)` host fn.
