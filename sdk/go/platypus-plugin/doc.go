// Package platypus is the Go-language plugin SDK for Platypus.
//
// It exposes:
//   - Host-function bindings (host_log, host_kv_*, host_fs_*,
//     host_exec, host_http, host_uname, host_net_*, host_process_*,
//     host_stream_*) as plain Go functions whose signatures mirror
//     the corresponding Rust extism-pdk extern declarations under
//     example/plugins/system/<plugin>/src/lib.rs;
//   - JSON envelope helpers — every host fn call in Platypus
//     round-trips through {"ok": bool, "data": <T>, "error": "..."}
//     (see internal/agent/plugin/host_funcs.go);
//   - Capability declarations + manifest pointers for plugin
//     authors looking for boilerplate.
//
// Build target: TinyGo with the wasi (wasip1) target. Full Go is
// not supported — Go's wasm32 emit produces ~5 MiB output with no
// extism import compatibility, while TinyGo produces ~150-300 KiB
// modules that drop straight into the Platypus runtime.
//
// Example minimal plugin:
//
//	package main
//
//	import (
//	    "github.com/extism/go-pdk"
//	    plugin "github.com/WangYihang/Platypus/sdk/go/platypus-plugin"
//	)
//
//	//export hello
//	func hello() int32 {
//	    name := string(pdk.Input())
//	    plugin.HostLog(plugin.LogInfo, "greeting "+name)
//	    pdk.OutputString("Hello, " + name + "!")
//	    return 0
//	}
//
//	func main() {} // required by TinyGo's wasi target
//
// Build:
//
//	tinygo build -target wasi -o plugin.wasm .
package platypus
