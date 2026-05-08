# Echo plugin

The reference Platypus plugin: takes a string, returns the same
string. Useful as a smoke test that the runtime + signing + install
pipeline are wired up end-to-end on a target host.

This is the canonical "read me first" example for plugin authoring —
the smallest thing that compiles, signs, installs, and runs.

## Prerequisites

```
rustup target add wasm32-unknown-unknown
go build -o ./build/platypus-cli ./cmd/platypus-cli  # from repo root
```

## Build

```
cargo build --release --target wasm32-unknown-unknown
```

The .wasm lands at `target/wasm32-unknown-unknown/release/echo.wasm`.

## Sign

You'll need a publisher key the target agent will trust. Generate one
once:

```
platypus-cli plugin keygen \
    --out-secret  ~/.platypus/keys/me.secret \
    --out-public  ~/.platypus/keys/me.pub
```

Distribute the **public** key to every agent that should accept your
plugins by dropping it under
`~/.platypus/agent/plugins/publishers/<keyid>.pub` on each one.

Then sign the build:

```
platypus-cli plugin sign \
    --key  ~/.platypus/keys/me.secret \
    --wasm target/wasm32-unknown-unknown/release/echo.wasm
```

This writes `target/wasm32-unknown-unknown/release/echo.wasm.minisig`
next to the .wasm.

## Install on an agent

Two paths:

1. **Inline** (operator clicks Install in the desktop UI / curls the
   REST endpoint with the manifest, wasm, and signature base64-encoded
   in the JSON body). See `docs/plugins/USERS.md` (forthcoming).

2. **System bundle** (you're a Platypus maintainer adding a plugin to
   the agent build): copy `plugin.yaml`, `echo.wasm`, and
   `echo.wasm.minisig` into
   `internal/agent/plugin/system/embedded/com.platypus.example-echo/1.0.0/`,
   make sure `internal/agent/plugin/system/embedded/publisher.pub`
   matches the signing key, and rebuild the agent.

## Verify

After install, invoke from the server side:

```
curl -X POST .../api/v1/projects/<pid>/agents/<agent-id>/rpc \
    -H 'Authorization: Bearer <token>' \
    -d '{"plugin_call":{"plugin_id":"com.platypus.example-echo","method":"echo","payload":"aGVsbG8="}}'
```

Should return the base64-encoded `"hello"` payload back.
