package dispatcher

import (
	"fmt"

	"github.com/WangYihang/Platypus/internal/core"
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
	var target *core.TCPClient = core.Ctx.FindTCPClientByHash(clue)

	// Searching via Hash failed, search via Alias
	if target == nil {
		target = core.Ctx.FindTCPClientByAlias(clue)
	}

	if target != nil {
		// TODO: lock, websocket race condition when jumping
		core.Ctx.CurrentTermite = nil
		core.Ctx.Current = target
		log.Success("The current interactive shell is set to: %s", core.Ctx.Current.FullDesc())
		// Update prompt
		// BUG:
		// The prompt will set only at the `Jump` command once.
		// If we jump to a client before the os & user is detected
		// So the prompt will be:
		// (Unknown) 127.0.0.1:43802 [unknown] »
		readLineInstance.SetPrompt(color.CyanString(core.Ctx.Current.GetPrompt()))
		return
	}

	// TermiteClient
	var targetTermite *core.TermiteClient = core.Ctx.FindTermiteClientByHash(clue)

	if targetTermite == nil {
		targetTermite = core.Ctx.FindTermiteClientByAlias(clue)
	}

	if targetTermite != nil {
		core.Ctx.Current = nil
		core.Ctx.CurrentTermite = targetTermite
		log.Success("The current termite interactive shell is set to: %s", core.Ctx.CurrentTermite.FullDesc())
		readLineInstance.SetPrompt(color.CyanString(core.Ctx.CurrentTermite.GetPrompt()))
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
