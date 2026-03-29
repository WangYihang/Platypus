package dispatcher

import (
	"fmt"

	"github.com/WangYihang/Platypus/internal/core"
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
		if core.Ctx.Current == nil && core.Ctx.CurrentTermite == nil {
			log.Error("Interactive session is not set, please use `Jump` command to set the interactive Interact")
			return
		}

		if core.Ctx.Current != nil {
			current := core.Ctx.Current
			current.GatherClientInfo(current.GetHashFormat())
			readLineInstance.SetPrompt(color.CyanString(current.GetPrompt()))
			return
		}

		if core.Ctx.CurrentTermite != nil {
			current := core.Ctx.CurrentTermite
			current.GatherClientInfo(current.GetHashFormat())
			readLineInstance.SetPrompt(color.CyanString(current.GetPrompt()))
			return
		}
	} else {
		clue := args[0]
		// Client information
		targetClient := core.Ctx.FindTCPClientByHash(clue)
		if targetClient != nil {
			targetClient.GatherClientInfo(targetClient.GetHashFormat())
			readLineInstance.SetPrompt(color.CyanString(targetClient.GetPrompt()))
			return
		}

		// Client information
		targetTermiteClient := core.Ctx.FindTermiteClientByHash(clue)
		if targetTermiteClient != nil {
			targetTermiteClient.GatherClientInfo(targetTermiteClient.GetHashFormat())
			readLineInstance.SetPrompt(color.CyanString(targetTermiteClient.GetPrompt()))
			return
		}

		// Server information
		targetServer := core.Ctx.FindServerByHash(clue)
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
