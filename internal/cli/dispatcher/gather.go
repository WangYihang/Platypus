package dispatcher

import (
	"fmt"

	"github.com/WangYihang/Platypus/internal/context"
	"github.com/WangYihang/Platypus/internal/utils/log"
	"github.com/fatih/color"
)

func (dispatcher commandDispatcher) Gather(args []string) {
	if len(args) > 1 {
		log.Error("Arguments error, use `Help Gather` to get more information")
		dispatcher.GatherHelp([]string{})
		return
	}

	if len(args) == 0 {
		if context.Ctx.Current == nil && context.Ctx.CurrentTermite == nil {
			log.Error("Interactive session is not set, please use `Jump` command to set the interactive Interact")
			return
		}

		if context.Ctx.Current != nil {
			current := context.Ctx.Current
			current.GatherClientInfo(current.GetHashFormat())
			readLineInstance.SetPrompt(color.CyanString(current.GetPrompt()))
			return
		}

		if context.Ctx.CurrentTermite != nil {
			current := context.Ctx.CurrentTermite
			current.GatherClientInfo(current.GetHashFormat())
			readLineInstance.SetPrompt(color.CyanString(current.GetPrompt()))
			return
		}
	} else {
		clue := args[0]
		// Client information
		targetClient := context.Ctx.FindTCPClientByHash(clue)
		if targetClient != nil {
			targetClient.GatherClientInfo(targetClient.GetHashFormat())
			readLineInstance.SetPrompt(color.CyanString(targetClient.GetPrompt()))
			return
		}

		// Client information
		targetTermiteClient := context.Ctx.FindTermiteClientByHash(clue)
		if targetTermiteClient != nil {
			targetTermiteClient.GatherClientInfo(targetTermiteClient.GetHashFormat())
			readLineInstance.SetPrompt(color.CyanString(targetTermiteClient.GetPrompt()))
			return
		}

		// Server information
		targetServer := context.Ctx.FindServerByHash(clue)
		if targetServer != nil {
			for _, client := range targetServer.Clients {
				client.GatherClientInfo(client.GetHashFormat())
			}
			return
		}
		log.Error("No such node")
	}
}

func (dispatcher commandDispatcher) GatherHelp(args []string) {
	fmt.Println("Usage of Gather")
	fmt.Println("\tGather [HASH]")
	fmt.Println("\tHASH\tThe hash of an node, node can be both a server or a client")
}

func (dispatcher commandDispatcher) GatherDesc(args []string) {
	fmt.Println("Gather")
	fmt.Println("\tGather information from the current client or the client with hash provided")
}
