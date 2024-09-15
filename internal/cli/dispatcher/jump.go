package dispatcher

import (
	"fmt"

	"github.com/WangYihang/Platypus/internal/context"
	"github.com/WangYihang/Platypus/internal/utils/log"
	"github.com/fatih/color"
)

func (dispatcher commandDispatcher) Jump(args []string) {
	if len(args) != 1 {
		log.Error("Arguments error, use `Help Jump` to get more information")
		dispatcher.JumpHelp([]string{})
		return
	}

	clue := args[0]

	// TCPClient
	// Search via Hash
	var target *context.TCPClient = context.Ctx.FindTCPClientByHash(clue)

	// Searching via Hash failed, search via Alias
	if target == nil {
		target = context.Ctx.FindTCPClientByAlias(clue)
	}

	if target != nil {
		// TODO: lock, websocket race condition when jumping
		context.Ctx.CurrentTermite = nil
		context.Ctx.Current = target
		log.Success("The current interactive shell is set to: %s", context.Ctx.Current.FullDesc())
		// Update prompt
		// BUG:
		// The prompt will set only at the `Jump` command once.
		// If we jump to a client before the os & user is detected
		// So the prompt will be:
		// (Unknown) 127.0.0.1:43802 [unknown] »
		readLineInstance.SetPrompt(color.CyanString(context.Ctx.Current.GetPrompt()))
		return
	}

	// TermiteClient
	var targetTermite *context.TermiteClient = context.Ctx.FindTermiteClientByHash(clue)

	if targetTermite == nil {
		targetTermite = context.Ctx.FindTermiteClientByAlias(clue)
	}

	if targetTermite != nil {
		context.Ctx.Current = nil
		context.Ctx.CurrentTermite = targetTermite
		log.Success("The current termite interactive shell is set to: %s", context.Ctx.CurrentTermite.FullDesc())
		readLineInstance.SetPrompt(color.CyanString(context.Ctx.CurrentTermite.GetPrompt()))
		return
	}

	log.Error("No such node: %s", clue)
}

func (dispatcher commandDispatcher) JumpHelp(args []string) {
	fmt.Println("Usage of Jump")
	fmt.Println("\tJump [HASH | NAME]")
	fmt.Println("\tHASH\tThe hash of a node which you want to interact with.")
	fmt.Println("\tNAME\tThe name of a node which you want to interact with. The name can be set via `Rename` command.")
}

func (dispatcher commandDispatcher) JumpDesc(args []string) {
	fmt.Println("Jump")
	fmt.Println("\tJump to a node, waiting for interactiving with it.")
}
