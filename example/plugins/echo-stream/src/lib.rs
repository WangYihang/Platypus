// Reference wasm-streaming plugin: echoes bytes from inbound to
// outbound, then closes. Smoke test for the
// STREAM_TYPE_PLUGIN_STREAM dispatch path.
//
// The agent sets pctx.activeStream before invoking `echo`; calls to
// host_stream_read pull one chunk off the wire, host_stream_write
// pushes one chunk back, host_stream_close signals end-of-output.
// host_stream_read returns base64-encoded bytes inside a JSON
// envelope (binary-safe transport across the wasm/JSON boundary);
// host_stream_write takes raw bytes.

use base64::{engine::general_purpose::STANDARD, Engine as _};
use extism_pdk::*;
use serde::Deserialize;

#[host_fn("platypus")]
extern "ExtismHost" {
    fn host_stream_read() -> Json<Envelope>;
    fn host_stream_write(data: Vec<u8>) -> Json<Envelope>;
    fn host_stream_close() -> Json<Envelope>;
}

#[derive(Deserialize, Default)]
struct Envelope {
    ok: bool,
    #[serde(default)]
    data: serde_json::Value,
    #[serde(default)]
    error: String,
}

#[plugin_fn]
pub fn echo(_: ()) -> FnResult<()> {
    loop {
        let env: Envelope = unsafe { host_stream_read()?.0 };
        if !env.ok {
            return Err(WithReturnCode::new(
                Error::msg(format!("host_stream_read: {}", env.error)),
                1,
            ));
        }
        // host_stream_read encodes binary chunks as base64 strings so
        // the JSON envelope stays UTF-8 clean. An empty string is the
        // EOF sentinel — the wire side observed KIND_EOF or peer
        // close, no more bytes will arrive.
        let b64 = env.data.as_str().unwrap_or("");
        if b64.is_empty() {
            break;
        }
        let chunk = STANDARD
            .decode(b64)
            .map_err(|e| WithReturnCode::new(Error::msg(format!("base64 decode: {e}")), 2))?;
        let env: Envelope = unsafe { host_stream_write(chunk)?.0 };
        if !env.ok {
            return Err(WithReturnCode::new(
                Error::msg(format!("host_stream_write: {}", env.error)),
                3,
            ));
        }
    }
    let _: Envelope = unsafe { host_stream_close()?.0 };
    Ok(())
}
