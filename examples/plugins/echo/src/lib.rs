// Reference Platypus plugin written in Rust against the Extism PDK.
//
// What it does: takes a string, returns the same string. Useful as a
// smoke test that the runtime + signing + install pipeline are wired
// up end-to-end on a target host. Not a stand-in for a real plugin —
// it doesn't claim any capabilities, doesn't persist state, doesn't
// call any host functions.
//
// To build:
//
//   rustup target add wasm32-unknown-unknown
//   cargo build --release --target wasm32-unknown-unknown
//
// Output lands at target/wasm32-unknown-unknown/release/echo.wasm.
// Sign with:
//
//   platypus-cli plugin sign \
//     --key path/to/your/secret.platypus \
//     --wasm target/wasm32-unknown-unknown/release/echo.wasm
//
// Then bundle the .wasm + .minisig + plugin.yaml into a tarball / push
// via the agent REST API / drop into a system-plugin embedded/ tree,
// depending on the deployment shape.

use extism_pdk::*;

#[plugin_fn]
pub fn echo(input: String) -> FnResult<String> {
    Ok(input)
}
