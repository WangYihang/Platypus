# sys-file-archive

System plugin replacing the legacy Go `HandleFileArchiveStream`.
Claims `STREAM_TYPE_FILE_ARCHIVE` via the `wasm:archive` host_handler
marker.

The plugin walks the requested paths via `host_fs_listdir` +
`host_fs_stat` + `host_fs_read_range`, packs them into TAR (POSIX
ustar, hand-rolled to skip the `tar` crate dep) or gzip-wrapped TAR
via `flate2`, and emits the same `FileArchiveResponse` ack +
`FileChunk` frames the legacy handler did.

## Format support

| Format | Status |
|---|---|
| `ARCHIVE_FORMAT_TAR` | Supported |
| `ARCHIVE_FORMAT_TAR_GZ` | Supported (flate2, level 1-9, default 6) |
| `ARCHIVE_FORMAT_ZIP` | **Not supported** — returns a clear error |

The legacy Go handler had ZIP support inherited from
`archive/zip`. The wasm replacement omits ZIP because the Rust
`zip` crate adds ~150 KiB to the wasm after lto/strip — judged
disproportionate to the value, since every modern OS (Windows
10+, macOS, Linux) reads `.tar.gz` natively. Operators on Windows
fleets that genuinely need ZIP today can either install a separate
plugin or fall back to the legacy handler until the cutover.

## Cutover sequence

Same shape as the other sys-file-* plugins. See
`sys-file-read/README.md` for the canonical version. Briefly:

1. Build + sign the .wasm with the system signing key.
2. Stage under `internal/agent/plugin/system/embedded/com.platypus.sys-file-archive/1.0.0/`.
3. Remove the `file_archive` entry from `example/plugins/sys-streams/plugin.yaml`'s `streams:` list, rebuild + re-sign sys-streams.
4. Delete `internal/agent/file_archive_stream.go` + the file_archive
   adapter line in `cmd/platypus-agent/stream_adapters.go`.
5. `go test ./internal/agent/...` — the integration tests in
   `internal/agent/plugin/file_archive_integration_test.go` are the
   canonical end-to-end coverage (TAR + TAR_GZ + ZIP-error-path).

## Capability requirements

`fs.read` with `paths: ["/"]` — same posture the legacy in-process
handler had implicitly.

## Memory budget

Default `max_memory_mb: 64` in the manifest. TAR_GZ archiving
buffers up to one 256-KiB FileChunk in flight + flate2's deflate
state (~32 KiB of dictionary) + walking-stack vectors. 64 MiB has
plenty of headroom for typical fleet workloads.
