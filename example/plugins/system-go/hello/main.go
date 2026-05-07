// hello-go is the G0 smoke-test plugin: proves the Platypus extism
// plugin contract is language-portable by implementing the simplest
// possible RPC in Go (TinyGo wasi target).  The wire is identical
// to a Rust extism plugin — same JSON envelope shape, same
// `platypus` host-fn namespace, same .minisig signing, same
// installation pipeline.
//
// Beyond smoke-testing the install pipeline this plugin doubles as
// the canonical install-time-config example, referenced from
// docs/plugins/CONFIG_AUTHORING.md. The two configurable knobs are
// `greeting` (string prefix) and `shout` (bool, uppercase output)
// — small enough to read in one screenful but covering both
// primitive types most config schemas reach for.
//
// Build:  tinygo build -target wasi -o hello.wasm .
//
// Stage:  go run ./hack/stage_system_plugins  (from repo root)
package main

import (
	"errors"
	"strings"

	"github.com/extism/go-pdk"

	platypus "github.com/WangYihang/Platypus/sdk/go/platypus-plugin"
)

// helloConfig mirrors plugin.yaml's config.schema. JSON tags match
// schema property names; missing fields fall back to Go zero values
// which we treat as "use the manifest defaults" inline below.
type helloConfig struct {
	Greeting string `json:"greeting"`
	Shout    bool   `json:"shout"`
}

// hello takes the input bytes (a name, raw — no envelope), reads
// the operator's resolved plugin config (or falls back to defaults),
// and emits "<greeting>, <name> from Go!" — possibly uppercased
// when shout is on.
//
//export hello
func hello() int32 {
	name := string(pdk.Input())
	if name == "" {
		name = "world"
	}

	cfg := helloConfig{Greeting: "Hello"} // mirror manifest default
	if err := platypus.ConfigInto(&cfg); err != nil && !errors.Is(err, platypus.ErrNoConfig) {
		platypus.LogInfof("hello-go: config decode failed (%v); using defaults", err)
	}
	if cfg.Greeting == "" {
		cfg.Greeting = "Hello"
	}

	platypus.LogInfof("hello-go invoked with name=%s greeting=%q shout=%v",
		name, cfg.Greeting, cfg.Shout)

	out := cfg.Greeting + ", " + name + " from Go!"
	if cfg.Shout {
		out = strings.ToUpper(out)
	}
	pdk.OutputString(out)
	return 0
}

// main is required by TinyGo's wasi target even when the module is
// purely export-driven.  Empty body is fine; the runtime never
// invokes it on a plugin module.
func main() {}
