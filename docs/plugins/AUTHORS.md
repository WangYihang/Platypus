# Writing Platypus plugins

This guide is for **plugin authors** — anyone building a new
extension that will be installed onto a Platypus agent. Operator-side
documentation (browse the marketplace, install on a fleet, audit
capability grants) lives in [USERS.md](USERS.md). The trust model
+ sandbox guarantees live in [SECURITY.md](SECURITY.md). Plugins
that take install-time configuration (endpoints, credentials,
allowlists) declare it via the `config:` block — see
[CONFIG_AUTHORING.md](CONFIG_AUTHORING.md).

## TL;DR

A Platypus plugin is a single **WebAssembly module** plus a
`plugin.yaml` manifest, signed with **minisign Ed25519**. The same
artefact runs on every supported agent platform (linux/darwin/windows
× amd64/arm64/arm/mips/...) — no per-target builds.

Pick a language with an [Extism PDK](https://extism.org/docs/concepts/pdk):
Rust, Go (TinyGo), JavaScript (QuickJS), Python (py2wasm), C/C++,
AssemblyScript, Zig. Rust is the recommended first-class option —
smallest binaries, fastest startup, strongest type system. The
reference `example/plugins/echo/` is in Rust.

## 1. Scaffold the project

Easiest: copy `example/plugins/echo/`. The structure for a Rust
plugin is:

```
your-plugin/
  Cargo.toml          # crate-type=cdylib, depends on extism-pdk
  plugin.yaml         # Platypus manifest (id, version, capabilities, …)
  src/lib.rs          # one or more #[plugin_fn] exports
  README.md
```

Required toolchain:

```
rustup target add wasm32-unknown-unknown
go build -o build/platypus-cli ./cmd/platypus-cli   # for sign + verify
```

## 2. Write your code

Each RPC method is a `#[plugin_fn]` function:

```rust
use extism_pdk::*;
use serde::{Deserialize, Serialize};

#[derive(Deserialize)]
pub struct PingRequest { pub greeting: String }

#[derive(Serialize)]
pub struct PingResponse { pub reply: String }

#[plugin_fn]
pub fn ping(req: Json<PingRequest>) -> FnResult<Json<PingResponse>> {
    Ok(Json(PingResponse {
        reply: format!("hello, {}!", req.0.greeting),
    }))
}
```

The function name (`ping`) becomes the **method** the server calls
via `RpcRequest.plugin_call.method`. List every method you export in
your `plugin.yaml` under `rpc:` — methods not declared there are
rejected before reaching the wasm runtime, even if the binary
exports them.

### Calling host functions

The agent exposes a small set of capability-gated host functions in
the `platypus` namespace. Each requires a matching capability in your
manifest (and operator confirmation at install time):

| Host fn                         | Capability  | What it does                                      |
|---------------------------------|-------------|---------------------------------------------------|
| `host_log`                      | `log`       | Structured log line (always granted)              |
| `host_kv_get` / `host_kv_put`   | `kv`        | Per-plugin namespaced scratch on disk             |
| `host_fs_read`                  | `fs.read`   | Read a file under a manifest-allowlisted path     |
| `host_fs_listdir`               | `fs.read`   | List a directory under the same allowlist          |
| `host_fs_stat`                  | `fs.read`   | Stat a path under the same allowlist               |
| `host_exec`                     | `exec`      | Run a command from the manifest's exact-path list  |
| `host_sysinfo`                  | `sysinfo`   | Read-only host snapshot                            |
| `host_http`                     | `net.http`  | HTTP request to a manifest-allowlisted host        |

Bind them from Rust with `extism_pdk::host_fn!`:

```rust
extism_pdk::host_fn!("platypus" "host_log" (level: i32, msg: u64));
extism_pdk::host_fn!("platypus" "host_fs_read" (path: u64) -> u64);
```

The wire convention for input/output offsets follows extism's own
PDK; consult its docs.

### What the sandbox refuses

WebAssembly's linear memory means anything not exposed via a host
function is impossible to do. Out of the box:

- **No filesystem syscalls** beyond `host_fs_*`.
- **No network** beyond `host_http`.
- **No `exec`** beyond the manifest allowlist.
- **No threads** (pure single-threaded execution).
- **No clocks beyond what extism's runtime provides.**

If you need something else, open an issue against `WangYihang/Platypus`
proposing a new host function.

## 3. Write the manifest

`plugin.yaml` is the contract between your plugin and the operator.
Every field that affects security is operator-visible at install time.

```yaml
api_version: 1                              # only 1 is accepted today
id: com.acme.disk-monitor                   # reverse-DNS, globally unique
name: Disk Monitor
version: 1.4.2                              # strict semver MAJOR.MINOR.PATCH
author:
  name: Acme Corp
  email: plugins@acme.example
license: Apache-2.0
homepage: https://github.com/acme/platypus-disk-monitor
description: |
  Watches disk usage on every mounted filesystem and reports
  thresholds back via the Platypus event stream.

runtime:
  type: wasm
  entry: disk_monitor.wasm                  # built artefact filename
  abi: extism/1

rpc:
  - name: scan
    request:  { proto: ScanRequest }
    response: { proto: ScanResponse }
    proto_descriptor: descriptor.binpb      # optional; informational

capabilities:
  fs.read:
    paths: [/proc/mounts, /sys/block]       # exact paths or directory roots
  net.http:
    hosts: [stats.acme.example]             # bare hostnames; no scheme/port

resources:
  max_memory_mb: 32                         # wazero memory cap
  max_invocation_ms: 5000                   # per-call ceiling

signature:
  algo: minisign-ed25519                    # only algorithm in MVP
  key_id: <16-hex-chars>                    # your publisher key id
  sig_file: disk_monitor.wasm.minisig
```

**Capability declarations are an upper bound.** The operator
installing your plugin ticks each capability separately; if they
deny `net.http`, host_http calls return `capability_denied` even
though your manifest requests it. Make every capability worth its
cost.

## 4. Generate a publisher key

If you don't already have one:

```
platypus-cli plugin keygen \
    --out-secret  ~/.platypus/keys/me.secret \
    --out-public  ~/.platypus/keys/me.pub
```

The public key id (printed by the command) goes into your manifest's
`signature.key_id`. The secret stays on your build machine — never
checked in, never published.

For trust to flow, every agent that should accept your plugin must
have your public key under `~/.platypus/agent/plugins/publishers/`.
Operators do this when they enroll your publisher; for system plugins
shipped inside the agent build, it's
`internal/agent/plugin/system/embedded/publisher.pub`.

## 5. Build + sign + ship

```
# Build to WebAssembly
cargo build --release --target wasm32-unknown-unknown

# Sign the artefact
platypus-cli plugin sign \
    --key  ~/.platypus/keys/me.secret \
    --wasm target/wasm32-unknown-unknown/release/disk_monitor.wasm

# Validate the manifest before publishing
platypus-cli plugin validate-manifest plugin.yaml
```

You now have three files ready to ship:

```
plugin.yaml
disk_monitor.wasm
disk_monitor.wasm.minisig
```

Distribution paths:

- **Direct install** (operator uploads to a single agent): operator
  bundles the three files in the desktop UI's Install dialog
  (forthcoming) or POSTs them base64-encoded to the agent's plugin
  REST endpoint. See [USERS.md](USERS.md).

- **Marketplace** (forthcoming): publish to the
  `WangYihang/platypus-plugins` git index repo via PR; operators
  browse + install through the desktop UI.

- **System plugin** (you're a Platypus maintainer): drop the three
  files under
  `internal/agent/plugin/system/embedded/<plugin_id>/<version>/`
  in the platypus-agent source tree. Every agent built from that
  source auto-installs the plugin on boot.

## 6. Iterate

After install, the agent surfaces your plugin's stdout/stderr-style
log lines (everything you wrote with `host_log`) via the
`GET .../plugins/:id/logs` REST endpoint. Use it during development
instead of guessing what your plugin is doing inside the sandbox.

Versions are strictly compared: an in-place upgrade requires bumping
`version` in the manifest. The runtime's hot-load path tears down the
prior wasm instance and brings the new one up without a server-side
reconnect.

## See also

- [USERS.md](USERS.md) — installing + managing plugins as an operator
- [SECURITY.md](SECURITY.md) — the trust model + sandbox guarantees
- [CROSS_PLATFORM.md](CROSS_PLATFORM.md) — per-OS plugin matrix +
  exemplar references for multi-backend dispatch (M5a `sys-pkg-linux`),
  shell-out + column parsing (M4a `sys-services-darwin`),
  `host_fs_read` + structured-text parsing (M3a `sys-net-linux`),
  and PowerShell injection-safe string builders (M4b `sys-services-windows`)
- [GO_PLUGINS.md](GO_PLUGINS.md) — TinyGo SDK + Rust ↔ Go parity matrix
- `example/plugins/echo/` — minimal complete reference
- [Extism docs](https://extism.org/docs/) — PDK references for every
  supported language
