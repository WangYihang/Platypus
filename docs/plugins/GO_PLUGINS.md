# Writing Platypus plugins in Go

Plugins targeting the Platypus extism runtime can be written in Go
(via [TinyGo](https://tinygo.org)) as well as Rust.  Both languages
produce wasm modules that drop into the same agent install pipeline,
share the same `platypus` host-fn namespace, and use the same JSON
envelope wire format — the agent has no idea which source language
produced any given `.wasm` it loads.

Reference implementations:

- `example/plugins/system-go/hello/` — minimal smoke-test plugin
  (~30 LoC).  Reads stdin via `pdk.Input()`, calls `HostLog`, writes
  output via `pdk.OutputString()`.
- `example/plugins/system-go/sys-info/` — TinyGo port of the Rust
  sys-info plugin.  Same wire output (`v2pb.SysInfoResponse`-shaped
  protojson), same /proc + /etc parsing strategy, ~270 LoC.

The Go SDK lives at `sdk/go/platypus-plugin/` and wraps host
functions on top of `github.com/extism/go-pdk`'s `Memory`
primitives.  Coverage today: `HostLog`, `HostUname`,
`HostFSRead` / `HostFSReadString` / `HostFSListDir`, `KVGet` /
`KVPut`.  Add more bindings as you port plugins; they're 1:1 with
the Rust extern declarations under
`example/plugins/system/<plugin>/src/lib.rs`.

## Toolchain

```
# install TinyGo (linux/amd64):
wget https://github.com/tinygo-org/tinygo/releases/download/v0.39.0/tinygo_0.39.0_amd64.deb
sudo dpkg -i tinygo_0.39.0_amd64.deb
```

The Makefile's `stage-system-plugins` target invokes both `cargo build`
(for Rust plugins) and `tinygo build -target wasi` (for Go
plugins).  Missing TinyGo is non-fatal: the Rust set rebuilds
unconditionally; system-go/ is skipped with a warning.

## Plugin skeleton

```go
//go:build wasip1

package main

import (
    "github.com/extism/go-pdk"
    platypus "github.com/WangYihang/Platypus/sdk/go/platypus-plugin"
)

//export my_method
func myMethod() int32 {
    input := pdk.Input()
    platypus.LogInfof("invoked with %d bytes", len(input))
    pdk.OutputString("hello from Go")
    return 0
}

func main() {} // required by TinyGo's wasi target
```

```yaml
# plugin.yaml
api_version: 1
id: com.example.my-plugin
name: MyPlugin
version: 1.0.0
runtime:
  type: wasm
  entry: my_plugin.wasm
  abi: extism/1
  lang: go        # informational only
rpc:
  - name: my_method
    request:  { proto: bytes }
    response: { proto: bytes }
resources:
  max_memory_mb: 16
  max_invocation_ms: 5000
signature:
  algo: minisign-ed25519
  key_id: REPLACE_WITH_YOUR_KEY_ID
  sig_file: my_plugin.wasm.minisig
```

## Calling host functions

Each `host_*` fn in the agent (`internal/agent/plugin/host_*.go`)
is mirrored as a Go fn in the SDK.  Capabilities apply identically
to both languages — declare `capabilities.fs.read.paths` etc. in
`plugin.yaml` exactly as you would for a Rust plugin; the agent
enforces the allowlist on the wasm side regardless of source
language.

Wire format gotcha: `host_uname` takes no parameters; some host fns
take one i64 (memory pointer); a few like `host_kv_put` take two.
The SDK's bindings do this dance for you — when you add a new one,
match the arity declared in
`internal/agent/plugin/host_funcs.go:buildHostFunctions`.

## Binary size

TinyGo wasm modules are heftier than Rust:

| Plugin           | Rust    | TinyGo | Notes                                  |
|------------------|---------|--------|----------------------------------------|
| hello (smoke)    | -       | 260 KiB | no dependencies pulled in              |
| sys-info         | 205 KiB | 1.0 MiB | encoding/json + strconv + reflect      |

The cost comes from TinyGo's runtime, `encoding/json`, and
`strconv` reflection.  Acceptable for system plugins (well under
the manifest's `max_memory_mb` runtime budget); marketplace plugins
that ship over slow uplinks may prefer Rust.

## Adding more system-plugin Go ports

Pattern from G1 (`sys-info-go`):

1. Read the Rust source at `example/plugins/system/<name>/src/lib.rs`.
2. Translate to Go under `example/plugins/system-go/<name>/main.go`,
   keeping function shapes 1:1 so the parity matrix is greppable.
3. Add any host-fn bindings the plugin needs to
   `sdk/go/platypus-plugin/host.go` (one wasmimport + a Go
   wrapper, mirroring the Rust extern block).
4. Drop a `plugin.yaml` next to `main.go`.  Plugin id is
   `<original>-go` (e.g. `com.platypus.sys-procs-linux-go`).
5. `tinygo build -target wasi -o <entry>.wasm .`
6. `go run ./hack/stage_system_plugins` from repo root.
7. Add an integration test under
   `internal/agent/plugin/<name>_go_integration_test.go` modeled
   after `sys_info_go_integration_test.go`.

Status of the system-plugin port matrix (May 2026):

| Plugin            | Rust   | Go (G phase) |
|-------------------|--------|--------------|
| sys-info          | 2.0.0  | 1.0.0 (G1)   |
| sys-procs-linux   | 2.0.0  | 1.0.0 (G2; renamed M0) |
| sys-security      | 2.0.0  | pending (G3) |
| sys-config-audit  | 2.0.0  | pending (G4) |
| sys-tunnel-pull   | 1.0.0  | pending (G5) |
| sys-process       | 1.0.0  | pending (G6) |
| sys-files-write   | 1.0.0  | pending (G7) |
| sys-files-read    | 1.0.1  | pending (G8) |

Each pending port is mechanical (~30 min once familiar with the
SDK).  The streaming plugins (G5-G8) need the `host_stream_*` SDK
binding as well — pattern from `host_link_write_frame` in the Rust
crates becomes a `HostStreamWrite` wrapper on the Go side.

## Capabilities — same surface, same enforcement

| Capability  | SDK fn(s)                                    | Manifest field                       |
|-------------|----------------------------------------------|--------------------------------------|
| `log`       | `HostLog`, `LogInfof`/`LogWarnf`/...         | implicit (every plugin)              |
| `sysinfo`   | `HostUname`                                  | `capabilities.sysinfo: true`         |
| `kv`        | `KVGet`, `KVPut`                             | `capabilities.kv: true`              |
| `fs.read`   | `HostFSRead`, `HostFSReadString`, `HostFSListDir` | `capabilities.fs.read.paths: [...]` |
| `fs.write`  | (TBD — pending G7)                           | `capabilities.fs.write.paths: [...]` |
| `exec`      | (TBD — pending G6)                           | `capabilities.exec.commands: [...]`  |
| `net.http`  | (TBD — host-side stub)                       | `capabilities.net.http.hosts: [...]` |
| `net.dial`  | (TBD — pending G5)                           | `capabilities.net.dial.targets: [...]` |
| `process`   | (TBD — pending G6)                           | `capabilities.process.commands: [...]` |

The manifest schema accepts globs (`*`, `**`, `?`) in path/command
allowlist entries — see `docs/plugins/SECURITY.md` for the syntax;
applies to both Rust and Go plugins.
