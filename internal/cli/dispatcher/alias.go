package dispatcher

import (
	"fmt"
	"strings"

	"github.com/WangYihang/Platypus/internal/context"
	"github.com/WangYihang/Platypus/internal/utils/log"
	"github.com/fatih/color"
)

func (dispatcher commandDispatcher) Alias(args []string) {
	if len(args) != 1 {
		log.Error("Arguments error, use `Help Alias` to get more information")
		dispatcher.AliasHelp([]string{})
		return
	}

	// Ensure the interactive session is set
	if context.Ctx.Current == nil && context.Ctx.CurrentTermite == nil {
		log.Error("Interactive session is not set, please use `Jump` command to set the interactive Interact")
		return
	}

	if context.Ctx.Current != nil {
		// Alias session
		log.Info("Renaming session: %s", context.Ctx.Current.FullDesc())
		context.Ctx.Current.Alias = strings.TrimSpace(args[0])
		readLineInstance.SetPrompt(color.CyanString(context.Ctx.Current.GetPrompt()))
		return
	}

	if context.Ctx.CurrentTermite != nil {
		// Alias session
		log.Info("Renaming session: %s", context.Ctx.CurrentTermite.FullDesc())
		context.Ctx.CurrentTermite.Alias = strings.TrimSpace(args[0])
		readLineInstance.SetPrompt(color.CyanString(context.Ctx.CurrentTermite.GetPrompt()))
		return
	}

}

func (dispatcher commandDispatcher) AliasHelp(args []string) {
	fmt.Println("Usage of Alias")
	fmt.Println("\tAlias")
}

func (dispatcher commandDispatcher) AliasDesc(args []string) {
	fmt.Println("Alias")
	fmt.Println("\tAlias the current session with a human-readable name.")
}
