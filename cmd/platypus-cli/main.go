// Command platypus-cli is the developer-side Platypus tool. It hosts
// helpers that plugin authors and operators run on their own
// workstation — keygen, sign, verify, manifest validation, and other
// build-time utilities. Distinct from platypus-server / platypus-agent
// so production deployments don't ship developer commands.
//
// Subcommand entry points live in sibling files under this directory
// (plugin_keygen.go, plugin_sign.go, ...). Add new subcommands by
// dropping a new field on `cli.Plugin` and writing the matching file.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/alecthomas/kong"

	"github.com/WangYihang/Platypus/pkg/version"
)

// cli is the top-level kong root. Each top-level subcommand groups
// related operations (today: `plugin`); leaves are the actual
// commands kong routes to.
var cli struct {
	Version kong.VersionFlag `help:"Print platypus-cli version and exit."`
	Plugin  pluginCmd        `cmd:"" help:"Plugin author tooling: keygen, sign, verify, validate."`
}

func main() {
	kctx := kong.Parse(&cli,
		kong.Name("platypus-cli"),
		kong.Description("Platypus developer tooling. Plugin authoring + build-time helpers."),
		kong.Vars{"version": version.Version},
		kong.UsageOnError(),
	)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := kctx.Run(&runContext{Context: ctx}); err != nil {
		fmt.Fprintf(os.Stderr, "platypus-cli: %v\n", err)
		os.Exit(1)
	}
}

// runContext is the single argument every subcommand's Run method
// accepts. Holds the cancellable context so a long-running command
// (like building or verifying a large .wasm) is interruptable with
// Ctrl-C.
type runContext struct {
	context.Context
}
