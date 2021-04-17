package dispatcher

import (
	"fmt"
	"strings"

	"github.com/WangYihang/Platypus/lib/context"
	"github.com/WangYihang/Platypus/lib/util/log"
	"github.com/fatih/color"
)

func (dispatcher Dispatcher) Alias(args []string) {
	if len(args) != 1 {
		log.Error("Arguments error, use `Help Alias` to get more information")
		dispatcher.AliasHelp([]string{})
		return
	}

	// Ensure the interactive session is set
	if context.Ctx.Current == nil {
		log.Error("Interactive session is not set, please use `Jump` command to set the interactive Interact")
		return
	}

	// Alias session
	log.Info("Renaming session: %s", context.Ctx.Current.FullDesc())
	context.Ctx.Current.Alias = strings.TrimSpace(args[0])

	// Update prompt
	var user string
	if context.Ctx.Current.User == "" {
		user = "unknown"
	} else {
		user = context.Ctx.Current.User
	}
	ReadLineInstance.SetPrompt(color.CyanString(
		"[%s] (%s) %s [%s] » ",
		context.Ctx.Current.Alias,
		context.Ctx.Current.OS.String(),
		context.Ctx.Current.GetConnString(),
		user,
	))
}

func (dispatcher Dispatcher) AliasHelp(args []string) {
	fmt.Println("Usage of Alias")
	fmt.Println("\tAlias")
}

func (dispatcher Dispatcher) AliasDesc(args []string) {
	fmt.Println("Alias")
	fmt.Println("\tAlias the current session with a human-readable name.")
}
