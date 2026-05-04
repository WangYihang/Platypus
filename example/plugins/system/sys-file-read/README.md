# sys-file-read

System plugin replacing the legacy Go `HandleFileReadStream`. Claims
`STREAM_TYPE_FILE_READ` via the `wasm:read` host_handler marker so
the agent's `DispatchLegacyWasmStream` routes incoming streams here.

The wire format is byte-for-byte identical to the previous Go
implementation (length-prefixed `FileReadResponse` + `FileChunk`
frames) so server-side readers don't need to change.

## Cutover sequence (release-engineering)

The sys-streams system bundle currently claims `STREAM_TYPE_FILE_READ`
with a `host_handler: agent.file_read` marker. That claim wins over
this plugin's because sys-streams' bundle has been signed and shipped
inside the agent binary. Cutting over requires:

1. `cargo build --release --target wasm32-unknown-unknown`
2. `platypus-cli plugin sign --key $PLATYPUS_SYSTEM_KEY --wasm target/wasm32-unknown-unknown/release/sys_file_read.wasm`
3. Stage the signed bundle:
   ```
   internal/agent/plugin/system/embedded/com.platypus.sys-file-read/1.0.0/
     plugin.yaml
     sys_file_read.wasm
     sys_file_read.wasm.minisig
   ```
4. Edit `example/plugins/sys-streams/plugin.yaml` to remove the
   `file_read` entry from `streams:`.
5. Rebuild + re-sign the sys-streams .wasm so the embedded bundle
   under `internal/agent/plugin/system/embedded/com.platypus.sys-streams/1.0.0/`
   reflects the trimmed claim list. (`make sign-system-plugins` does
   the resign step in bulk.)
6. Delete `internal/agent/file_read_stream.go` + the file_read
   `streamAdapter` line in `cmd/platypus-agent/stream_adapters.go`.
7. Run the test suite: `go test ./internal/agent/...`. The integration
   test in `internal/agent/plugin/file_read_integration_test.go` is
   the canonical end-to-end coverage.

`PLATYPUS_SYSTEM_KEY` is the dev / release publisher signing secret
kept out-of-repo. Without access to it the bundle stays in this
example/ tree and the legacy Go handler keeps serving traffic.

## Capability requirements

`fs.read` with `paths: ["/"]` — same posture the legacy in-process
handler had implicitly. The agent enforces this on every
`host_fs_read_range` call, not at install time.
