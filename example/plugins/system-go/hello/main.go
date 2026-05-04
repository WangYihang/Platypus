// hello-go is the G0 smoke-test plugin: proves the Platypus extism
// plugin contract is language-portable by implementing the simplest
// possible RPC in Go (TinyGo wasi target).  The wire is identical
// to a Rust extism plugin — same JSON envelope shape, same
// `platypus` host-fn namespace, same .minisig signing, same
// installation pipeline.
//
// Build:  tinygo build -target wasi -o hello.wasm .
//
// Stage:  go run ./hack/stage_system_plugins  (from repo root)
package main

import (
	"github.com/extism/go-pdk"

	platypus "github.com/WangYihang/Platypus/sdk/go/platypus-plugin"
)

// hello takes the input bytes (a name, raw — no envelope), logs the
// invocation through host_log, and emits "Hello, <name>!" as the
// output. Caller marshals/unmarshals on the agent side.
//
//export hello
func hello() int32 {
	name := string(pdk.Input())
	if name == "" {
		name = "world"
	}
	platypus.LogInfof("hello-go invoked with name=%s", name)
	pdk.OutputString("Hello, " + name + " from Go!")
	return 0
}

// main is required by TinyGo's wasi target even when the module is
// purely export-driven.  Empty body is fine; the runtime never
// invokes it on a plugin module.
func main() {}
