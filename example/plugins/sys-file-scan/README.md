# sys-file-scan

System plugin replacing the legacy Go `HandleFileScanStream`. Claims
`STREAM_TYPE_FILE_SCAN` via the `wasm:scan` host_handler marker.

The plugin walks the requested paths using `host_fs_listdir` +
`host_fs_stat` (iterative, stack-based — wasm has no stack-overflow
protection beyond the linear memory cap), then emits a single
`FileScanResponse` frame matching the legacy handler's output.

## Cutover sequence

Same shape as sys-file-read's cutover (see that plugin's README).
Briefly:

1. Build + sign the .wasm with the system key.
2. Stage under `internal/agent/plugin/system/embedded/com.platypus.sys-file-scan/1.0.0/`.
3. Remove the `file_scan` entry from `example/plugins/sys-streams/plugin.yaml`'s `streams:` list, rebuild + re-sign sys-streams.
4. Delete `internal/agent/file_scan_stream.go` + the file_scan adapter line in `cmd/platypus-agent/stream_adapters.go`.
5. `go test ./internal/agent/...` — the integration test in
   `internal/agent/plugin/file_scan_integration_test.go` is the
   canonical end-to-end coverage.

## Capability requirements

`fs.read` with `paths: ["/"]` — matches the legacy in-process posture.
