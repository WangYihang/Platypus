package dispatcher

import (
	"fmt"
	"strings"

	"github.com/WangYihang/Platypus/lib/context"
	"github.com/WangYihang/Platypus/lib/util/log"
	"github.com/fatih/color"
)

func (dispatcher Dispatcher) Jump(args []string) {
	if len(args) != 1 {
		log.Error("Arguments error, use `Help Jump` to get more information")
		dispatcher.JumpHelp([]string{})
		return
	}

	// Search via Hash
	var target *context.TCPClient
	for _, server := range context.Ctx.Servers {
		for _, client := range (*server).GetAllTCPClients() {
			if strings.HasPrefix(client.Hash, strings.ToLower(args[0])) {
				target = client
			}
		}
	}

	// Searching via Hash failed, search via Alias
	if target == nil {
		for _, server := range context.Ctx.Servers {
			for _, client := range (*server).GetAllTCPClients() {
				if strings.HasPrefix(client.Alias, strings.ToLower(args[0])) {
					target = client
				}
			}
		}
	}

	if target != nil {
		// TODO: lock, websocket race condition when jumping
		context.Ctx.Current = target
		log.Success("The current interactive shell is set to: %s", context.Ctx.Current.FullDesc())
		// Update prompt
		// BUG:
		// The prompt will set only at the `Jump` command once.
		// If we jump to a client before the os & user is detected
		// So the prompt will be:
		// (Unknown) 127.0.0.1:43802 [unknown] »
		ReadLineInstance.SetPrompt(color.CyanString(context.Ctx.Current.GetPrompt()))
	} else {
		log.Error("No such node")
	}
}

func (dispatcher Dispatcher) JumpHelp(args []string) {
	fmt.Println("Usage of Jump")
	fmt.Println("\tJump [HASH | NAME]")
	fmt.Println("\tHASH\tThe hash of a node which you want to interact with.")
	fmt.Println("\tNAME\tThe name of a node which you want to interact with. The name can be set via `Rename` command.")
}

func (dispatcher Dispatcher) JumpDesc(args []string) {
	fmt.Println("Jump")
	fmt.Println("\tJump to a node, waiting for interactiving with it.")
}
