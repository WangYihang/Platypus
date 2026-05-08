# Writing Platypus plugins in Go

Plugins targeting the Platypus extism runtime can be written in Go
(via [TinyGo](https://tinygo.org)) as well as Rust.  Both languages
produce wasm modules that drop into the same agent install pipeline,
share the same `platypus` host-fn namespace, and use the same JSON
envelope wire format — the agent has no idea which source language
produced any given `.wasm` it loads.

Reference implementation:

- `example/plugins/system-go/hello/` — minimal smoke-test plugin
  (~30 LoC).  Reads stdin via `pdk.Input()`, calls `HostLog`, writes
  output via `pdk.OutputString()`.

The Go SDK lives at `sdk/go/platypus-plugin/` and wraps host
functions on top of `github.com/extism/go-pdk`'s `Memory`
primitives.  Coverage today: `HostLog`, `HostUname`,
`HostFSRead` / `HostFSReadString` / `HostFSListDir`, `KVGet` /
`KVPut`.  Add more bindings as you write Go plugins; they're 1:1
with the Rust extern declarations under
`example/plugins/system/<plugin>/src/lib.rs`.

Note: the canonical system-plugin set is the Rust crates under
`example/plugins/system/`. Go is supported as a secondary path for
operators who prefer it for marketplace plugins; we no longer ship
Go ports of the system plugins themselves.

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

TinyGo wasm modules are heftier than Rust. The hello smoke plugin
weighs in at ~260 KiB with no dependencies pulled in; anything
that touches `encoding/json`, `strconv`, or reflect runs closer to
1 MiB. Acceptable for low-volume operator plugins; marketplace
plugins that ship over slow uplinks generally prefer Rust.

## Capabilities — same surface, same enforcement

| Capability  | SDK fn(s)                                    | Manifest field                       |
|-------------|----------------------------------------------|--------------------------------------|
| `log`       | `HostLog`, `LogInfof`/`LogWarnf`/...         | implicit (every plugin)              |
| `sysinfo`   | `HostUname`                                  | `capabilities.sysinfo: true`         |
| `kv`        | `KVGet`, `KVPut`                             | `capabilities.kv: true`              |
| `fs.read`   | `HostFSRead`, `HostFSReadString`, `HostFSListDir` | `capabilities.fs.read.paths: [...]` |

Other capabilities (`fs.write`, `exec`, `net.http`, `net.dial`,
`process`) are implemented host-side and exposed to Rust today; add
the matching Go binding in `sdk/go/platypus-plugin/host.go` (one
`wasmimport` + a wrapper, mirroring the Rust extern block) when you
need them.

The manifest schema accepts globs (`*`, `**`, `?`) in path/command
allowlist entries — see `docs/plugins/SECURITY.md` for the syntax;
applies to both Rust and Go plugins.
